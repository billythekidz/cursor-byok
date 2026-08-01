package mitm

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"cursor/internal/certs"
	"cursor/internal/logger"
	"cursor/internal/netproxy"

	"github.com/elazarl/goproxy"
)

const (
	// HeaderServerUpstreamURL is the header carrying the original upstream address when forwarding to the backend server.
	HeaderServerUpstreamURL = "X-Server-Upstream-URL"
)

// ProxyServer defines the ProxyServer type in this module.
type ProxyServer struct {
	// addr represents the addr field in this declaration.
	addr string
	// baseURL represents the baseURL field in this declaration.
	baseURL string
	// certManager represents the certManager field in this declaration.
	certManager *certs.Manager
	// baseEndpoint represents the baseEndpoint field in this declaration.
	baseEndpoint *url.URL
	// baseMu represents the baseMu field in this declaration.
	baseMu sync.RWMutex

	// upstreamClient represents the upstreamClient field in this declaration.
	upstreamClient *http.Client

	// proxy represents the proxy field in this declaration.
	proxy *goproxy.ProxyHttpServer

	// runMu represents the runMu field in this declaration.
	runMu sync.RWMutex
	// httpServer represents the httpServer field in this declaration.
	httpServer *http.Server
	// serveErrCh represents the serveErrCh field in this declaration.
	serveErrCh chan error
}

// Snapshot defines the Snapshot type in this module.
type Snapshot struct {
	// ListenAddr represents the ListenAddr field in this declaration.
	ListenAddr string `json:"listenAddr"`
	// BaseURL represents the BaseURL field in this declaration.
	BaseURL string `json:"baseUrl"`
	// Running represents the Running field in this declaration.
	Running bool `json:"running"`
}

// hopByHopHeaders represents the hopByHopHeaders state value in this module.
var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Proxy-Connection":    {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

const (
	proxyLogRateLimitWindow  = 30 * time.Second
	proxyLogRateLimitTTL     = 5 * time.Minute
	proxyLogRateLimitMaxKeys = 1024
)

var proxyLogLimiter = newLogLimiter(proxyLogRateLimitWindow)

type logLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	entries map[string]*logLimitEntry
}

type logLimitEntry struct {
	nextAllowed time.Time
	lastSeen    time.Time
	suppressed  int
}

func newLogLimiter(window time.Duration) *logLimiter {
	if window <= 0 {
		window = 30 * time.Second
	}
	return &logLimiter{
		window:  window,
		entries: make(map[string]*logLimitEntry),
	}
}

func (limiter *logLimiter) ShouldLog(key string) (suppressed int, ok bool) {
	if limiter == nil {
		return 0, true
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return 0, true
	}

	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.pruneLocked(now)

	entry := limiter.entries[key]
	if entry == nil {
		limiter.entries[key] = &logLimitEntry{nextAllowed: now.Add(limiter.window), lastSeen: now}
		return 0, true
	}
	entry.lastSeen = now
	if now.Before(entry.nextAllowed) {
		entry.suppressed++
		return 0, false
	}

	suppressed = entry.suppressed
	entry.suppressed = 0
	entry.nextAllowed = now.Add(limiter.window)
	return suppressed, true
}

func (limiter *logLimiter) pruneLocked(now time.Time) {
	if len(limiter.entries) == 0 {
		return
	}
	cutoff := now.Add(-proxyLogRateLimitTTL)
	for key, entry := range limiter.entries {
		if entry == nil || entry.lastSeen.Before(cutoff) {
			delete(limiter.entries, key)
		}
	}
	for len(limiter.entries) >= proxyLogRateLimitMaxKeys {
		oldestKey := ""
		var oldestSeen time.Time
		for key, entry := range limiter.entries {
			if entry == nil {
				oldestKey = key
				break
			}
			if oldestKey == "" || entry.lastSeen.Before(oldestSeen) {
				oldestKey = key
				oldestSeen = entry.lastSeen
			}
		}
		if oldestKey == "" {
			return
		}
		delete(limiter.entries, oldestKey)
	}
}

func logSuppressedProxyMessages(prefix string, suppressed int) {
	if suppressed <= 0 {
		return
	}
	logger.Infof("%s: suppressed %d repeated messages in last %s", prefix, suppressed, proxyLogRateLimitWindow)
}

// NewProxyServer handles logic related to NewProxyServer.
func NewProxyServer(addr, baseURL, _ string, _ string, certManager *certs.Manager) (*ProxyServer, error) {
	u, normalizedBaseURL, err := parseBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	s := &ProxyServer{
		addr:         addr,
		baseURL:      normalizedBaseURL,
		certManager:  certManager,
		baseEndpoint: u,
		upstreamClient: &http.Client{
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          200,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
		},
	}
	s.proxy = s.newGoproxyHandler()
	return s, nil
}

// parseBaseURL handles logic related to parseBaseURL.
func parseBaseURL(baseURL string) (*url.URL, string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, "", errors.New("base URL is empty")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, "", fmt.Errorf("parse base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", fmt.Errorf("unsupported base URL scheme %q", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return nil, "", errors.New("base URL host is empty")
	}

	base := *u
	base.Path = ""
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	base.RawFragment = ""
	normalizedBaseURL := strings.TrimRight(base.String(), "/")
	if normalizedBaseURL == "" {
		return nil, "", errors.New("base URL is empty")
	}
	return &base, normalizedBaseURL, nil
}

// UpdateBaseURL handles logic related to UpdateBaseURL.
func (s *ProxyServer) UpdateBaseURL(baseURL string) error {
	u, normalizedBaseURL, err := parseBaseURL(baseURL)
	if err != nil {
		return err
	}
	s.baseMu.Lock()
	s.baseURL = normalizedBaseURL
	s.baseEndpoint = u
	s.baseMu.Unlock()
	return nil
}

// ListenAndServe handles logic related to ListenAndServe.
func (s *ProxyServer) ListenAndServe() error {
	if err := s.Start(); err != nil {
		return err
	}
	s.runMu.RLock()
	errCh := s.serveErrCh
	s.runMu.RUnlock()
	if errCh == nil {
		return nil
	}
	return <-errCh
}

// Start handles logic related to Start.
func (s *ProxyServer) Start() error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.httpServer != nil {
		return errors.New("proxy is already running")
	}

	httpServer := &http.Server{
		Addr:     s.addr,
		Handler:  s.proxy,
		ErrorLog: stdlog.New(&httpErrorFilterWriter{}, "", stdlog.LstdFlags),
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	s.httpServer = httpServer
	s.serveErrCh = make(chan error, 1)

	go func() {
		var serveErr error
		err := httpServer.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}

		s.runMu.Lock()
		if s.serveErrCh != nil {
			s.serveErrCh <- serveErr
			close(s.serveErrCh)
			s.serveErrCh = nil
		}
		if s.httpServer == httpServer {
			s.httpServer = nil
		}
		s.runMu.Unlock()
	}()

	return nil
}

// Stop handles logic related to Stop.
func (s *ProxyServer) Stop(ctx context.Context) error {
	s.runMu.Lock()
	httpServer := s.httpServer
	if httpServer == nil {
		s.runMu.Unlock()
		return nil
	}
	s.httpServer = nil
	s.runMu.Unlock()
	return httpServer.Shutdown(ctx)
}

// IsRunning handles logic related to IsRunning.
func (s *ProxyServer) IsRunning() bool {
	s.runMu.RLock()
	running := s.httpServer != nil
	s.runMu.RUnlock()
	return running
}

// Snapshot handles logic related to Snapshot.
func (s *ProxyServer) Snapshot() Snapshot {
	baseURL := s.currentBaseURL()
	return Snapshot{
		ListenAddr: s.addr,
		BaseURL:    baseURL,
		Running:    s.IsRunning(),
	}
}

// currentBaseURL handles logic related to currentBaseURL.
func (s *ProxyServer) currentBaseURL() string {
	s.baseMu.RLock()
	baseURL := s.baseURL
	s.baseMu.RUnlock()
	return baseURL
}

// currentBaseEndpoint handles logic related to currentBaseEndpoint.
func (s *ProxyServer) currentBaseEndpoint() *url.URL {
	s.baseMu.RLock()
	endpoint := s.baseEndpoint
	s.baseMu.RUnlock()
	if endpoint == nil {
		return nil
	}
	clone := *endpoint
	return &clone
}

// newGoproxyHandler handles logic related to newGoproxyHandler.
func (s *ProxyServer) newGoproxyHandler() *goproxy.ProxyHttpServer {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = false
	proxy.AllowHTTP2 = true
	proxy.Logger = &goproxyLogAdapter{}
	proxy.CertStore = newMITMCertStore()
	proxy.Tr = netproxy.NewTransport(&http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	})
	proxy.ConnectionErrHandler = func(conn io.Writer, ctx *goproxy.ProxyCtx, err error) {
		host := requestHost(ctx)
		remoteAddr := requestRemoteAddr(ctx)
		ua := trimForLog(requestUserAgent(ctx), 160)
		msg := fmt.Sprintf("proxy connect error: host=%s remote=%s ua=%q err=%v", host, remoteAddr, ua, err)
		key := "proxy-connect-error|" + strings.ToLower(strings.TrimSpace(host)) + "|" + errorRateLimitKey(err)
		if suppressed, ok := proxyLogLimiter.ShouldLog(key); ok {
			logSuppressedProxyMessages("proxy connect error", suppressed)
			logger.Errorf("%s", msg)
		}
	}

	var mitmAction *goproxy.ConnectAction
	if s.certManager != nil {
		caCert, err := s.certManager.CATLSCertificate()
		if err != nil {
			logger.Errorf("MITM disabled: invalid CA config err=%v", err)
		} else {
			logMITMCAInfo(caCert)
			baseTLSConfigFromCA := goproxy.TLSConfigFromCA(caCert)
			mitmAction = &goproxy.ConnectAction{
				Action: goproxy.ConnectMitm,
				TLSConfig: func(host string, ctx *goproxy.ProxyCtx) (*tls.Config, error) {
					cfg, err := baseTLSConfigFromCA(host, ctx)
					connectHost := normalizeConnectHost(host)
					remoteAddr := requestRemoteAddr(ctx)
					ua := trimForLog(requestUserAgent(ctx), 160)
					if err != nil {
						logger.Errorf(
							"MITM tls config failed: connect_host=%s remote=%s ua=%q err=%v",
							connectHost,
							remoteAddr,
							ua,
							err,
						)
						return nil, err
					}

					return cfg, nil
				},
			}
		}
	}
	proxy.OnRequest().HandleConnect(goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		if mitmAction == nil || !isWhitelistedRelayHost(host) {
			return goproxy.OkConnect, host
		}
		return mitmAction, host
	}))

	// After MITM decryption: Cursor whitelisted domains are forwarded to the backend server, while other requests are directly proxied upstream by goproxy.
	proxy.OnRequest().DoFunc(func(req *http.Request, _ *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		req.Header.Del(HeaderServerUpstreamURL)

		host := hostFromHTTPRequest(req)
		if isWhitelistedRelayHost(host) {
			if shouldHandleLocalCORSPreflight(req) {
				return req, buildLocalCORSPreflightResponse(req)
			}
			raw := requestURL(req)
			if parsedRaw, rawErr := rawURLForRelay(req); rawErr == nil {
				raw = parsedRaw
			}
			resp, err := s.forwardToServer(req)
			if err != nil {
				logger.Errorf("forwarding failed: %s %s %v", req.Method, raw, err)
				return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusBadGateway, "bad gateway")
			}

			return req, resp
		}

		return req, nil
	})
	return proxy
}

// forwardToServer handles logic related to forwardToServer.
func (s *ProxyServer) forwardToServer(incoming *http.Request) (*http.Response, error) {
	if incoming == nil {
		return nil, errors.New("nil request")
	}

	rawURL, err := rawURLForRelay(incoming)
	if err != nil {
		return nil, err
	}
	endpoint := s.currentBaseEndpoint()
	if endpoint == nil {
		return nil, errors.New("server endpoint is not configured")
	}
	forwardURL := *endpoint
	if incoming.URL != nil {
		forwardURL.Path = incoming.URL.Path
		forwardURL.RawPath = incoming.URL.RawPath
		forwardURL.RawQuery = incoming.URL.RawQuery
	}
	serverReq, err := http.NewRequestWithContext(incoming.Context(), incoming.Method, forwardURL.String(), incoming.Body)
	if err != nil {
		return nil, err
	}
	serverReq.ContentLength = incoming.ContentLength
	copyHeaders(serverReq.Header, incoming.Header)
	serverReq.Header.Set(HeaderServerUpstreamURL, rawURL)
	removeHopByHop(serverReq.Header)

	resp, err := s.upstreamClient.Do(serverReq)
	if err != nil {
		return nil, fmt.Errorf("forward to backend server: %w", err)
	}
	removeHopByHop(resp.Header)
	return resp, nil
}

// requestHost handles logic related to requestHost.
func requestHost(ctx *goproxy.ProxyCtx) string {
	if ctx == nil || ctx.Req == nil {
		return ""
	}
	if strings.TrimSpace(ctx.Req.Host) != "" {
		return ctx.Req.Host
	}
	if ctx.Req.URL != nil {
		return ctx.Req.URL.Host
	}
	return ""
}

// requestRemoteAddr handles logic related to requestRemoteAddr.
func requestRemoteAddr(ctx *goproxy.ProxyCtx) string {
	if ctx == nil || ctx.Req == nil {
		return "-"
	}
	remoteAddr := strings.TrimSpace(ctx.Req.RemoteAddr)
	if remoteAddr == "" {
		return "-"
	}
	return remoteAddr
}

// requestUserAgent handles logic related to requestUserAgent.
func requestUserAgent(ctx *goproxy.ProxyCtx) string {
	if ctx == nil || ctx.Req == nil {
		return "-"
	}
	ua := strings.TrimSpace(ctx.Req.UserAgent())
	if ua == "" {
		return "-"
	}
	return ua
}

// requestURL handles logic related to requestURL.
func requestURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	u := req.URL.String()
	if strings.TrimSpace(u) == "" {
		return "/"
	}
	return u
}

// rawURLForRelay handles logic related to rawURLForRelay.
func rawURLForRelay(r *http.Request) (string, error) {
	if r == nil {
		return "", errors.New("nil request")
	}
	if r.URL != nil && r.URL.IsAbs() {
		return r.URL.String(), nil
	}

	host := strings.TrimSpace(r.Host)
	if host == "" && r.URL != nil {
		host = strings.TrimSpace(r.URL.Host)
	}
	if host == "" {
		return "", errors.New("missing host")
	}

	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	path := "/"
	if r.URL != nil {
		if strings.TrimSpace(r.URL.RequestURI()) != "" {
			path = r.URL.RequestURI()
		}
	}
	return scheme + "://" + host + path, nil
}

// copyHeaders handles logic related to copyHeaders.
func copyHeaders(dst, src http.Header) {
	removeHopByHop(src)
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// removeHopByHop handles logic related to removeHopByHop.
func removeHopByHop(h http.Header) {
	if h == nil {
		return
	}
	connectionVals := h.Values("Connection")
	for _, line := range connectionVals {
		for _, token := range strings.Split(line, ",") {
			name := http.CanonicalHeaderKey(strings.TrimSpace(token))
			if name != "" {
				h.Del(name)
			}
		}
	}
	for k := range hopByHopHeaders {
		h.Del(k)
	}
}

// trimForLog handles logic related to trimForLog.
func trimForLog(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 {
		limit = 160
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

// normalizeConnectHost handles logic related to normalizeConnectHost.
func normalizeConnectHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "-"
	}
	return host
}

// hostFromHTTPRequest handles logic related to hostFromHTTPRequest.
func hostFromHTTPRequest(req *http.Request) string {
	if req == nil {
		return ""
	}
	if strings.TrimSpace(req.Host) != "" {
		return req.Host
	}
	if req.URL != nil {
		return req.URL.Host
	}
	return ""
}

// isWhitelistedRelayHost handles logic related to isWhitelistedRelayHost.
func isWhitelistedRelayHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}
	if strings.HasPrefix(host, "[") {
		host = strings.TrimPrefix(host, "[")
		host = strings.TrimSuffix(host, "]")
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = strings.ToLower(strings.TrimSpace(h))
	} else if strings.Count(host, ":") > 1 {
		// IPv6 without port
		host = strings.Trim(host, "[]")
	}
	if host == "api2.cursor.sh" || host == "api3.cursor.sh" {
		return true
	}
	if strings.HasSuffix(host, ".cursor.sh") {
		return true
	}
	return false
}

// mitmCertStore caches certificates dynamically issued by goproxy for sites, avoiding repeated RSA/x509 issuance for the same host.
type mitmCertStore struct {
	mu    sync.Mutex
	certs map[string]*tls.Certificate
}

func newMITMCertStore() *mitmCertStore {
	return &mitmCertStore{certs: make(map[string]*tls.Certificate)}
}

func (store *mitmCertStore) Fetch(hostname string, gen func() (*tls.Certificate, error)) (*tls.Certificate, error) {
	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if hostname == "" {
		return gen()
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if cert, ok := store.certs[hostname]; ok {
		return cert, nil
	}
	cert, err := gen()
	if err != nil {
		return nil, err
	}
	store.certs[hostname] = cert
	return cert, nil
}

// shouldHandleLocalCORSPreflight handles logic related to shouldHandleLocalCORSPreflight.
func shouldHandleLocalCORSPreflight(req *http.Request) bool {
	if req == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(req.Method), http.MethodOptions) {
		return false
	}
	if strings.TrimSpace(req.Header.Get("Origin")) == "" {
		return false
	}
	if strings.TrimSpace(req.Header.Get("Access-Control-Request-Method")) == "" {
		return false
	}
	return true
}

// buildLocalCORSPreflightResponse handles logic related to buildLocalCORSPreflightResponse.
func buildLocalCORSPreflightResponse(req *http.Request) *http.Response {
	allowOrigin := "*"
	if req != nil {
		origin := strings.TrimSpace(req.Header.Get("Origin"))
		if origin != "" {
			allowOrigin = origin
		}
	}

	headers := make(http.Header)
	headers.Set("Access-Control-Allow-Origin", allowOrigin)
	headers.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
	headers.Set("Access-Control-Allow-Headers", "*")
	headers.Set("Access-Control-Allow-Credentials", "true")
	headers.Set("Access-Control-Max-Age", "86400")
	if allowOrigin != "*" {
		headers.Set("Vary", "Origin")
	}

	body := io.NopCloser(strings.NewReader(""))
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Status:     "204 No Content",
		Header:     headers,
		Body:       body,
		Request:    req,
	}
}

// httpErrorFilterWriter defines the httpErrorFilterWriter type in this module.
type httpErrorFilterWriter struct{}

// Write handles logic related to Write.
func (w *httpErrorFilterWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}

	lower := strings.ToLower(msg)
	if strings.Contains(lower, "tls: first record does not look like a tls handshake") {
		logger.Infof("proxy ignore handshake mismatch: %s", msg)
		return len(p), nil
	}
	if strings.Contains(lower, "tls handshake error") {
		logger.Errorf("proxy tls handshake error: %s", msg)
		return len(p), nil
	}

	logger.Infof("proxy http server: %s", msg)
	return len(p), nil
}

// goproxyLogAdapter defines the goproxyLogAdapter type in this module.
type goproxyLogAdapter struct{}

// Printf handles logic related to Printf.
func (l *goproxyLogAdapter) Printf(format string, args ...interface{}) {
	msg := strings.TrimSpace(fmt.Sprintf(format, args...))
	if msg == "" {
		return
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "tls: first record does not look like a tls handshake") {
		// logger.Infof("goproxy ignore handshake mismatch: %s", msg)
		return
	}
	if strings.Contains(lower, "broken pipe") || strings.Contains(lower, "connection reset by peer") {
		// logger.Infof("goproxy transient network error: %s", msg)
		return
	}
	if suppressed, ok := proxyLogLimiter.ShouldLog("goproxy|" + goproxyMessageRateLimitKey(msg)); ok {
		logSuppressedProxyMessages("goproxy", suppressed)
		logger.Infof("goproxy: %s", msg)
	}
}

func errorRateLimitKey(err error) string {
	if err == nil {
		return ""
	}
	return normalizeRateLimitKey(err.Error())
}

func goproxyMessageRateLimitKey(msg string) string {
	return normalizeRateLimitKey(msg)
}

func normalizeRateLimitKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = normalizeAddrPortsForLogKey(value)
	value = normalizeGoproxyRequestIDForLogKey(value)
	return value
}

func normalizeAddrPortsForLogKey(value string) string {
	parts := strings.Fields(value)
	for index, part := range parts {
		parts[index] = normalizeAddrPortToken(part)
	}
	return strings.Join(parts, " ")
}

func normalizeAddrPortToken(token string) string {
	trimmedRight := strings.TrimRight(token, ",;)]}")
	suffix := strings.TrimPrefix(token[len(trimmedRight):], "")
	host, port, err := net.SplitHostPort(trimmedRight)
	if err != nil || host == "" || port == "" {
		return token
	}
	return net.JoinHostPort(host, "*") + suffix
}

func normalizeGoproxyRequestIDForLogKey(value string) string {
	start := strings.Index(value, "[")
	end := strings.Index(value, "]")
	if start < 0 || end <= start {
		return value
	}
	prefix := strings.TrimSpace(value[:start])
	if prefix != "" {
		return value
	}
	return "[*]" + value[end+1:]
}

// logMITMCAInfo handles logic related to logMITMCAInfo.
func logMITMCAInfo(caTLS *tls.Certificate) {
	if caTLS == nil || len(caTLS.Certificate) == 0 {
		logger.Errorf("MITM CA info unavailable: empty certificate chain")
		return
	}

	leaf, err := x509.ParseCertificate(caTLS.Certificate[0])
	if err != nil {
		logger.Errorf("MITM CA parse failed: %v", err)
		return
	}

	sum := sha256.Sum256(leaf.Raw)
	logger.Infof(
		"MITM enabled: sha256=%s subject=%s valid=%s~%s",
		strings.ToUpper(hex.EncodeToString(sum[:])),
		leaf.Subject.String(),
		leaf.NotBefore.Format(time.RFC3339),
		leaf.NotAfter.Format(time.RFC3339),
	)
}
