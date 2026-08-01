package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cursor/internal/cursor"
	"cursor/internal/logger"
	"cursor/internal/mitm"
	"cursor/internal/netproxy"
	localruntime "cursor/internal/runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ProxyState defines the ProxyState type in this module.
type ProxyState struct {
	// ListenAddr keeps the old field for frontend cache compatibility; its actual value equals proxyListenAddr.
	ListenAddr string `json:"listenAddr"`
	// Running keeps the old field for frontend cache compatibility; its actual value equals proxyRunning.
	Running bool `json:"running"`
	// BackendListenAddr represents the embedded backend listen address.
	BackendListenAddr string `json:"backendListenAddr"`
	// BackendRunning represents whether the embedded backend has been started.
	BackendRunning bool `json:"backendRunning"`
	// ProxyListenAddr represents the MITM proxy listen address.
	ProxyListenAddr string `json:"proxyListenAddr"`
	// ProxyRunning represents whether the MITM proxy has been started.
	ProxyRunning bool `json:"proxyRunning"`
	// CursorSettingsApplied represents whether the host proxy settings have been injected.
	CursorSettingsApplied bool `json:"cursorSettingsApplied"`
	// NetProxySource represents the current outbound network proxy source: system/env/direct.
	NetProxySource string `json:"netProxySource"`
	// NetProxyActive represents whether the outbound network proxy is enabled.
	NetProxyActive bool `json:"netProxyActive"`
	// NetProxyUsingSystem represents whether the outbound network proxy comes from the OS proxy.
	NetProxyUsingSystem bool `json:"netProxyUsingSystem"`
	// NetProxyUsingEnv represents whether the outbound network proxy comes from environment variables.
	NetProxyUsingEnv bool `json:"netProxyUsingEnv"`
	// NetProxyHTTP represents the current HTTP proxy address, with credentials removed.
	NetProxyHTTP string `json:"netProxyHttp"`
	// NetProxyHTTPS represents the current HTTPS proxy address, with credentials removed.
	NetProxyHTTPS string `json:"netProxyHttps"`
	// NetProxyPACIgnored represents that a PAC/auto proxy was detected but treated as direct connection this round.
	NetProxyPACIgnored bool `json:"netProxyPacIgnored"`
	// NetProxyDescription represents a summary of the outbound network proxy, with credentials removed.
	NetProxyDescription string `json:"netProxyDescription"`
	// LastError represents the LastError field in this declaration.
	LastError string `json:"lastError"`
}

// StartProxy handles logic related to StartProxy.
func (s *ProxyService) StartProxy() (ProxyState, error) {
	logger.Infof("start service requested config_path=%s logs_root=%s", s.configPath, s.logsRoot)
	fail := func(step string, err error) (ProxyState, error) {
		logger.Errorf("start service failed step=%s err=%v", step, err)
		s.setLastError(err)
		s.emitState()
		return s.GetState(), err
	}
	cfg, err := s.LoadUserConfig()
	if err != nil {
		return fail("load_user_config", err)
	}
	if err := s.ensureBackendHost(); err != nil {
		return fail("ensure_backend_host", err)
	}
	if !s.backendHost.IsRunning() {
		logger.Infof("starting embedded backend listen_addr=%s", s.backendHost.ListenAddr())
		if err := s.backendHost.Start(); err != nil {
			return fail("start_backend", err)
		}
	} else {
		logger.Infof("embedded backend already running listen_addr=%s", s.backendHost.ListenAddr())
	}
	healthCtx, healthCancel := context.WithTimeout(context.Background(), backendReadyTimeout)
	defer healthCancel()
	if err := s.waitForBackend(healthCtx); err != nil {
		return fail("wait_backend_ready", err)
	}
	logger.Infof("embedded backend ready listen_addr=%s", s.backendHost.ListenAddr())
	if err := s.ensureProxy(cfg); err != nil {
		return fail("ensure_proxy", err)
	}

	// Inject account info at startup.
	if err := cursor.InjectCursorUserInfo(localruntime.InjectAccountEmail, localruntime.InjectAuthToken); err != nil {
		logger.Errorf("injectCursorUserInfo failed: %v", err)
		// Do not block startup; only log.
	}

	if s.proxy != nil && !s.proxy.IsRunning() {
		logger.Infof("starting mitm proxy listen_addr=%s", s.proxy.Snapshot().ListenAddr)
		if err := s.proxy.Start(); err != nil {
			return fail("start_mitm_proxy", err)
		}
	}

	if err := s.ApplyCursorSettings(); err != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = s.ClearCursorSettings()
		if s.proxy != nil {
			_ = s.proxy.Stop(stopCtx)
		}
		_ = s.backendHost.Stop(stopCtx)
		startErr := fmt.Errorf("service started, but applying Cursor settings failed: %w", err)
		logger.Errorf("start service failed step=apply_cursor_settings err=%v", startErr)
		s.setLastError(startErr)
		s.emitState()
		return s.GetState(), startErr
	}

	s.setLastError(nil)
	s.emitState()
	state := s.GetState()
	logger.Infof(
		"start service completed backend_listen_addr=%s proxy_listen_addr=%s cursor_settings_applied=%t",
		state.BackendListenAddr,
		state.ProxyListenAddr,
		state.CursorSettingsApplied,
	)
	return state, nil
}

// StopProxy handles logic related to StopProxy.
func (s *ProxyService) StopProxy() (ProxyState, error) {
	logger.Infof("stop service requested")
	fail := func(step string, err error) (ProxyState, error) {
		logger.Errorf("stop service failed step=%s err=%v", step, err)
		s.setLastError(err)
		s.emitState()
		return s.GetState(), err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if s.proxy != nil && s.proxy.IsRunning() {
		logger.Infof("stopping mitm proxy listen_addr=%s", s.proxy.Snapshot().ListenAddr)
		if err := s.proxy.Stop(ctx); err != nil {
			return fail("stop_mitm_proxy", err)
		}
	}

	if err := s.ClearCursorSettings(); err != nil {
		return fail("clear_cursor_settings", err)
	}
	if s.backendHost != nil {
		logger.Infof("stopping embedded backend listen_addr=%s", s.backendHost.ListenAddr())
		if err := s.backendHost.Stop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return fail("stop_backend", err)
		}
	}

	s.setLastError(nil)
	s.emitState()
	state := s.GetState()
	logger.Infof(
		"stop service completed backend_running=%t proxy_running=%t cursor_settings_applied=%t",
		state.BackendRunning,
		state.ProxyRunning,
		state.CursorSettingsApplied,
	)
	return state, nil
}

// GetState handles logic related to GetState.
func (s *ProxyService) GetState() ProxyState {
	var proxySnap mitm.Snapshot
	if s.proxy != nil {
		proxySnap = s.proxy.Snapshot()
	}
	s.mu.RLock()
	lastError := s.lastError
	cursorSettingsApplied := s.cursorSettingsApplied
	s.mu.RUnlock()
	backendListenAddr := ""
	backendRunning := false
	if s.backendHost != nil {
		backendListenAddr = s.backendHost.ListenAddr()
		backendRunning = s.backendHost.IsRunning()
	}
	netProxy := netproxy.CurrentStatus()
	return ProxyState{
		ListenAddr:            proxySnap.ListenAddr,
		Running:               proxySnap.Running,
		BackendListenAddr:     backendListenAddr,
		BackendRunning:        backendRunning,
		ProxyListenAddr:       proxySnap.ListenAddr,
		ProxyRunning:          proxySnap.Running,
		CursorSettingsApplied: cursorSettingsApplied,
		NetProxySource:        netProxy.Source,
		NetProxyActive:        netProxy.Active,
		NetProxyUsingSystem:   netProxy.UsingSystemProxy,
		NetProxyUsingEnv:      netProxy.UsingEnvProxy,
		NetProxyHTTP:          netProxy.HTTPProxy,
		NetProxyHTTPS:         netProxy.HTTPSProxy,
		NetProxyPACIgnored:    netProxy.PACIgnored,
		NetProxyDescription:   netProxy.Description,
		LastError:             lastError,
	}
}

// ClearLastError handles logic related to ClearLastError.
func (s *ProxyService) ClearLastError() ProxyState {
	s.setLastError(nil)
	s.emitState()
	return s.GetState()
}

// SetBaseURL handles logic related to SetBaseURL.
func (s *ProxyService) SetBaseURL(baseURL string) (ProxyState, error) {
	_ = strings.TrimSpace(baseURL)
	err := fmt.Errorf("backend/proxy address is fixed, directly modifying baseURL is no longer supported")
	s.setLastError(err)
	s.emitState()
	return s.GetState(), err
}

// setLastError handles logic related to setLastError.
func (s *ProxyService) setLastError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.lastError = ""
		return
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "unknown error"
	}
	s.lastError = msg
}

// emitState handles logic related to emitState.
func (s *ProxyService) emitState() {
	app := application.Get()
	if app == nil {
		return
	}
	state := s.GetState()
	if state.Running {
		state.LastError = ""
	}
	app.Event.Emit("proxy:state", state)
}

// ShutdownForQuit handles logic related to ShutdownForQuit.
func (s *ProxyService) ShutdownForQuit() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var finalErr error

	if s.proxy != nil {
		if err := s.proxy.Stop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			finalErr = err
		}
	}
	if err := s.ClearCursorSettings(); err != nil {
		finalErr = errors.Join(finalErr, err)
	}
	if s.backendHost != nil {
		if err := s.backendHost.Stop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			finalErr = errors.Join(finalErr, err)
		}
	}
	if finalErr != nil {
		s.setLastError(finalErr)
	}
}

func (s *ProxyService) setCursorSettingsApplied(applied bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursorSettingsApplied = applied
}
