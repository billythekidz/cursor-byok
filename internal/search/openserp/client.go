// Package openserp manages the local OpenSERP process used by web search.
package openserp

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"cursor/internal/appdata"
	"cursor/internal/logger"
	"cursor/internal/netproxy"
)

const (
	releaseAPIURL        = "https://api.github.com/repos/karust/openserp/releases/latest"
	releaseRepository    = "https://github.com/karust/openserp"
	serverHost           = "127.0.0.1"
	serverReadyTimeout   = 35 * time.Second
	searchRequestTimeout = 75 * time.Second
	maxReleaseSize       = 64 * 1024 * 1024
	maxBinarySize        = 128 * 1024 * 1024
)

var ErrTooManyErrors = errors.New("openserp search stopped after three engine errors")

// FailureClass describes whether a search failure can be retried by the
// interaction bridge without repeating an unproductive provider pass.
type FailureClass string

const (
	FailureRetryableStartup         FailureClass = "retryable_startup"
	FailureRetryableTransport       FailureClass = "retryable_transport"
	FailureTerminalRateLimitOrBlock FailureClass = "terminal_rate_limit_or_block"
	FailureTerminalNoResults        FailureClass = "terminal_no_results"
	FailureTerminalTooManyErrors    FailureClass = "terminal_too_many_errors"
)

// SearchError preserves a bounded failure classification and the engines
// attempted during one Search invocation.
type SearchError struct {
	Class            FailureClass
	Retryable        bool
	AttemptedEngines []string
	Cause            error
}

func (err *SearchError) Error() string {
	if err == nil || err.Cause == nil {
		return string(FailureTerminalNoResults)
	}
	return err.Cause.Error()
}

func (err *SearchError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// FailureDetails returns a normalized failure class for a Search error.
func FailureDetails(err error) (FailureClass, bool, []string) {
	if err == nil {
		return "", false, nil
	}
	var searchErr *SearchError
	if errors.As(err, &searchErr) && searchErr != nil {
		return searchErr.Class, searchErr.Retryable, append([]string(nil), searchErr.AttemptedEngines...)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return FailureRetryableTransport, true, nil
	}
	return FailureRetryableTransport, true, nil
}

func newSearchError(class FailureClass, retryable bool, attemptedEngines []string, cause error) error {
	return &SearchError{
		Class:            class,
		Retryable:        retryable,
		AttemptedEngines: append([]string(nil), attemptedEngines...),
		Cause:            cause,
	}
}

// Result is a normalized result returned by one OpenSERP engine.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Engine  string `json:"engine"`
}

// EngineResults preserves the requested engine slot and the actual source used
// when a fallback was necessary.
type EngineResults struct {
	RequestedEngine string
	SourceEngine    string
	Fallback        bool
	Results         []Result
}

// Client starts OpenSERP on demand and queries the configured engine set.
type Client struct {
	manager *manager
}

type manager struct {
	httpClient *http.Client
	startMu    sync.Mutex
	mu         sync.Mutex
	cmd        *exec.Cmd
	baseURL    string
	ensureFunc func(context.Context) (string, error)
}

type engineResponse struct {
	engine  string
	results []Result
	err     error
}

var managers sync.Map

// NewClient creates a lazy local OpenSERP client.
func NewClient(client *http.Client) *Client {
	if client == nil {
		client = netproxy.NewHTTPClient(searchRequestTimeout)
	}
	managedClient := *client
	if managedClient.Timeout < searchRequestTimeout {
		managedClient.Timeout = searchRequestTimeout
	}
	m := &manager{httpClient: &managedClient}
	managers.Store(m, struct{}{})
	return &Client{manager: m}
}

// Shutdown stops every OpenSERP process started by this application.
func Shutdown() {
	managers.Range(func(key, _ any) bool {
		key.(*manager).stop()
		managers.Delete(key)
		return true
	})
}

// Search queries the four primary engines. A failed slot is replaced by a
// successful Yandex or Ecosia query. Three total engine errors abort the
// request instead of returning an unreliable result set.
func (client *Client) Search(ctx context.Context, query string) ([]EngineResults, error) {
	if client == nil || client.manager == nil {
		return nil, errors.New("openserp client is not initialized")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search query is required")
	}
	baseURL, err := client.manager.ensureRunning(ctx)
	if err != nil {
		return nil, newSearchError(FailureRetryableStartup, true, nil, fmt.Errorf("start OpenSERP: %w", err))
	}

	primary := []string{"google", "baidu", "duckduckgo", "yandex"}
	responses := make(chan engineResponse, len(primary))
	for _, engine := range primary {
		go func(engine string) {
			results, searchErr := client.searchEngine(ctx, baseURL, engine, query)
			responses <- engineResponse{engine: engine, results: results, err: searchErr}
		}(engine)
	}

	byEngine := make(map[string]engineResponse, len(primary))
	for range primary {
		item := <-responses
		byEngine[item.engine] = item
	}

	errorCount := 0
	for _, engine := range primary {
		if byEngine[engine].err != nil {
			errorCount++
		}
	}
	if errorCount >= 3 {
		return nil, newSearchError(FailureTerminalTooManyErrors, false, primary, fmt.Errorf("%w: %s", ErrTooManyErrors, describeErrors(primary, byEngine)))
	}

	fallbackCandidates := []string{"yandex", "ecosia"}
	fallbackCache := make(map[string]engineResponse, len(fallbackCandidates))
	results := make([]EngineResults, 0, len(primary))
	for _, requestedEngine := range primary {
		item := byEngine[requestedEngine]
		if item.err == nil && len(item.results) > 0 {
			results = append(results, EngineResults{
				RequestedEngine: requestedEngine,
				SourceEngine:    requestedEngine,
				Results:         item.results,
			})
			continue
		}

		replaced := false
		for _, fallbackEngine := range fallbackCandidates {
			if fallbackEngine == requestedEngine {
				continue
			}
			fallback, cached := fallbackCache[fallbackEngine]
			if !cached {
				fallback = byEngine[fallbackEngine]
				if fallback.err == nil && len(fallback.results) > 0 {
					fallbackCache[fallbackEngine] = fallback
				} else {
					fallback.results, fallback.err = client.searchEngine(ctx, baseURL, fallbackEngine, query)
					fallbackCache[fallbackEngine] = fallback
				}
			}
			if fallback.err != nil || len(fallback.results) == 0 {
				if !cached {
					errorCount++
				}
				if errorCount >= 3 {
					return nil, newSearchError(FailureTerminalTooManyErrors, false, primary, fmt.Errorf("%w: %s", ErrTooManyErrors, describeErrors(primary, byEngine)))
				}
				continue
			}
			results = append(results, EngineResults{
				RequestedEngine: requestedEngine,
				SourceEngine:    fallbackEngine,
				Fallback:        true,
				Results:         fallback.results,
			})
			replaced = true
			break
		}
		if !replaced {
			logger.Errorf("OpenSERP returned no replacement for engine=%s", requestedEngine)
		}
	}

	if len(results) == 0 {
		return nil, newSearchError(FailureTerminalNoResults, false, primary, errors.New("OpenSERP returned no parseable results"))
	}
	return results, nil
}

func (client *Client) searchEngine(ctx context.Context, baseURL, engine, query string) ([]Result, error) {
	endpoint := fmt.Sprintf("%s/%s/search?text=%s&limit=5&format=json", baseURL, engine, url.QueryEscape(query))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.manager.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
		return nil, fmt.Errorf("%s http status %d: %s", engine, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope struct {
		Results []Result `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxReleaseSize)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", engine, err)
	}
	if len(envelope.Results) == 0 {
		return nil, fmt.Errorf("%s returned no results", engine)
	}
	if len(envelope.Results) > 5 {
		envelope.Results = envelope.Results[:5]
	}
	return envelope.Results, nil
}

func describeErrors(engines []string, responses map[string]engineResponse) string {
	parts := make([]string, 0, len(engines))
	for _, engine := range engines {
		if err := responses[engine].err; err != nil {
			parts = append(parts, fmt.Sprintf("%s=%v", engine, err))
		}
	}
	return strings.Join(parts, "; ")
}

func (m *manager) ensureRunning(ctx context.Context) (string, error) {
	if m.ensureFunc != nil {
		return m.ensureFunc(ctx)
	}
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	if m.cmd != nil && m.baseURL != "" {
		baseURL := m.baseURL
		m.mu.Unlock()
		if err := waitReady(ctx, m.httpClient, baseURL, 2*time.Second); err == nil {
			return baseURL, nil
		}
		m.stopLocked()
	} else {
		m.mu.Unlock()
	}

	binaryPath, err := resolveBinary(ctx)
	if err != nil {
		return "", err
	}
	listener, err := net.Listen("tcp", serverHost+":0")
	if err != nil {
		return "", fmt.Errorf("reserve OpenSERP port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	baseURL := fmt.Sprintf("http://%s:%d", serverHost, port)
	command := exec.Command(binaryPath, "serve", "--host", serverHost, "--port", fmt.Sprint(port), "--quiet")
	command.Dir = filepath.Dir(binaryPath)
	logFile := openProcessLog()
	if logFile != nil {
		command.Stdout = logFile
		command.Stderr = logFile
	}
	if err := command.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return "", fmt.Errorf("launch %s: %w", binaryPath, err)
	}
	m.mu.Lock()
	m.cmd = command
	m.baseURL = baseURL
	m.mu.Unlock()
	go func() {
		err := command.Wait()
		if logFile != nil {
			_ = logFile.Close()
		}
		m.mu.Lock()
		if m.cmd == command {
			m.cmd = nil
			m.baseURL = ""
		}
		m.mu.Unlock()
		if err != nil {
			logger.Errorf("OpenSERP exited: %v", err)
		}
	}()
	if err := waitReady(ctx, m.httpClient, baseURL, serverReadyTimeout); err != nil {
		m.stop()
		return "", fmt.Errorf("wait for OpenSERP: %w", err)
	}
	logger.Infof("OpenSERP started at %s", baseURL)
	return baseURL, nil
}

func waitReady(ctx context.Context, client *http.Client, baseURL string, timeout time.Duration) error {
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		request, err := http.NewRequestWithContext(readyCtx, http.MethodGet, baseURL+"/ready", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-readyCtx.Done():
			return readyCtx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (m *manager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *manager) stopLocked() {
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
	m.cmd = nil
	m.baseURL = ""
}

func resolveBinary(ctx context.Context) (string, error) {
	if configured := strings.TrimSpace(os.Getenv("OPENSERP_BINARY")); configured != "" {
		if path, err := executablePath(configured); err == nil {
			return path, nil
		}
	}
	for _, candidate := range bundledBinaryCandidates() {
		if path, err := executablePath(candidate); err == nil {
			return path, nil
		}
	}

	cacheDir := filepath.Join(appdata.DataRootPath(), "openserp")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", fmt.Errorf("create OpenSERP cache: %w", err)
	}
	if path, err := materializeEmbeddedBinary(cacheDir); err == nil && path != "" {
		return path, nil
	}
	if path := newestCachedBinary(cacheDir); path != "" {
		return path, nil
	}

	downloaded, downloadErr := downloadLatestRelease(ctx, cacheDir)
	if downloadErr == nil {
		return downloaded, nil
	}
	logger.Errorf("download OpenSERP release failed: %v; trying source build", downloadErr)
	built, buildErr := buildFromSubmodule(ctx, cacheDir)
	if buildErr == nil {
		return built, nil
	}
	return "", fmt.Errorf("download OpenSERP: %v; build from submodule: %w", downloadErr, buildErr)
}

func materializeEmbeddedBinary(cacheDir string) (string, error) {
	if len(embeddedBinary) == 0 {
		return "", nil
	}
	name := fmt.Sprintf("openserp-%s-%s-embedded", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binaryPath := filepath.Join(cacheDir, name)
	if info, err := os.Stat(binaryPath); err == nil && info.Size() == int64(len(embeddedBinary)) {
		return binaryPath, nil
	}
	temporaryPath := binaryPath + ".tmp"
	if err := os.WriteFile(temporaryPath, embeddedBinary, 0o700); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(temporaryPath, 0o700); err != nil {
			_ = os.Remove(temporaryPath)
			return "", err
		}
	}
	if err := os.Rename(temporaryPath, binaryPath); err != nil {
		_ = os.Remove(temporaryPath)
		return "", err
	}
	return binaryPath, nil
}

func executablePath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	return filepath.Abs(path)
}

func bundledBinaryCandidates() []string {
	executable, _ := os.Executable()
	executableDir := filepath.Dir(executable)
	workingDir, _ := os.Getwd()
	name := "openserp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	archName := fmt.Sprintf("openserp-%s", runtime.GOARCH)
	if runtime.GOOS == "windows" {
		archName += ".exe"
	}
	return []string{
		filepath.Join(executableDir, name),
		filepath.Join(executableDir, archName),
		filepath.Join(executableDir, "..", "Resources", name),
		filepath.Join(workingDir, "bin", name),
		filepath.Join(workingDir, "bin", archName),
		filepath.Join(workingDir, "submodules", "openserp", name),
		filepath.Join(workingDir, "submodules", "openserp", archName),
		filepath.Join(executableDir, "submodules", "openserp", name),
		filepath.Join(executableDir, "submodules", "openserp", archName),
	}
}

func newestCachedBinary(cacheDir string) string {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return ""
	}
	namePrefix := fmt.Sprintf("openserp-%s-%s-", runtime.GOOS, runtime.GOARCH)
	candidates := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), namePrefix) {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".tmp") || strings.HasSuffix(entry.Name(), ".download") {
			continue
		}
		if runtime.GOOS == "windows" && !strings.HasSuffix(entry.Name(), ".exe") {
			continue
		}
		candidates = append(candidates, filepath.Join(cacheDir, entry.Name()))
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[len(candidates)-1]
}

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func downloadLatestRelease(ctx context.Context, cacheDir string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPIURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "cursor-byok-openserp-bootstrap")
	client := netproxy.NewHTTPClient(30 * time.Second)
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub release API returned status %d", response.StatusCode)
	}
	var latest release
	if err := json.NewDecoder(io.LimitReader(response.Body, maxReleaseSize)).Decode(&latest); err != nil {
		return "", fmt.Errorf("decode GitHub release metadata: %w", err)
	}
	assetName := fmt.Sprintf("openserp-%s-%s-%s.tgz", runtime.GOOS, runtime.GOARCH, strings.TrimPrefix(latest.TagName, "v"))
	var assetURL string
	for _, asset := range latest.Assets {
		if asset.Name == assetName {
			assetURL = asset.URL
			break
		}
	}
	if assetURL == "" {
		return "", fmt.Errorf("release %s has no asset %s", latest.TagName, assetName)
	}
	assetRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	assetRequest.Header.Set("Accept", "application/octet-stream")
	assetRequest.Header.Set("User-Agent", "cursor-byok-openserp-bootstrap")
	assetResponse, err := client.Do(assetRequest)
	if err != nil {
		return "", err
	}
	defer assetResponse.Body.Close()
	if assetResponse.StatusCode < 200 || assetResponse.StatusCode >= 300 {
		return "", fmt.Errorf("download OpenSERP asset returned status %d", assetResponse.StatusCode)
	}
	archivePath := filepath.Join(cacheDir, assetName+".download")
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(archive, io.LimitReader(assetResponse.Body, maxReleaseSize))
	closeErr := archive.Close()
	if copyErr != nil {
		_ = os.Remove(archivePath)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(archivePath)
		return "", closeErr
	}
	defer os.Remove(archivePath)

	versionedName := fmt.Sprintf("openserp-%s-%s-%s", runtime.GOOS, runtime.GOARCH, strings.TrimPrefix(latest.TagName, "v"))
	if runtime.GOOS == "windows" {
		versionedName += ".exe"
	}
	binaryPath := filepath.Join(cacheDir, versionedName)
	if err := extractBinary(archivePath, binaryPath); err != nil {
		return "", err
	}
	return binaryPath, nil
}

func extractBinary(archivePath, binaryPath string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open OpenSERP archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	temporaryPath := binaryPath + ".tmp"
	_ = os.Remove(temporaryPath)
	found := false
	for {
		header, headerErr := tarReader.Next()
		if errors.Is(headerErr, io.EOF) {
			break
		}
		if headerErr != nil {
			return headerErr
		}
		if header.Typeflag != tar.TypeReg || (filepath.Base(header.Name) != "openserp" && filepath.Base(header.Name) != "openserp.exe") {
			continue
		}
		if header.Size <= 0 || header.Size > maxBinarySize {
			return fmt.Errorf("invalid OpenSERP binary size %d", header.Size)
		}
		output, createErr := os.OpenFile(temporaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
		if createErr != nil {
			return createErr
		}
		_, copyErr := io.CopyN(output, tarReader, header.Size)
		closeErr := output.Close()
		if copyErr != nil {
			_ = os.Remove(temporaryPath)
			return copyErr
		}
		if closeErr != nil {
			_ = os.Remove(temporaryPath)
			return closeErr
		}
		if err := os.Rename(temporaryPath, binaryPath); err != nil {
			_ = os.Remove(temporaryPath)
			return err
		}
		found = true
		break
	}
	if !found {
		return errors.New("OpenSERP archive does not contain an executable")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(binaryPath, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func buildFromSubmodule(ctx context.Context, cacheDir string) (string, error) {
	sourceRoot := findSubmoduleRoot()
	if sourceRoot == "" {
		return "", fmt.Errorf("OpenSERP submodule was not found; see %s", releaseRepository)
	}
	name := fmt.Sprintf("openserp-%s-%s-built", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binaryPath := filepath.Join(cacheDir, name)
	temporaryPath := binaryPath + ".tmp"
	_ = os.Remove(temporaryPath)
	command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-buildvcs=false", "-o", temporaryPath, ".")
	command.Dir = sourceRoot
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(temporaryPath, binaryPath); err != nil {
		_ = os.Remove(temporaryPath)
		return "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(binaryPath, 0o700)
	}
	return binaryPath, nil
}

func findSubmoduleRoot() string {
	executable, _ := os.Executable()
	workingDir, _ := os.Getwd()
	candidates := []string{
		filepath.Join(workingDir, "submodules", "openserp"),
		filepath.Join(filepath.Dir(executable), "submodules", "openserp"),
		filepath.Join(filepath.Dir(executable), "..", "submodules", "openserp"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func openProcessLog() *os.File {
	if err := os.MkdirAll(appdata.LogsRootPath(), 0o700); err != nil {
		return nil
	}
	path := filepath.Join(appdata.LogsRootPath(), "openserp.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		logger.Errorf("open OpenSERP log: %v", err)
		return nil
	}
	return file
}
