package client

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"cursor/internal/appdata"
	backend "cursor/internal/backend"
	serverconfig "cursor/internal/backend/server/config"
	"cursor/internal/certs"
	"cursor/internal/logger"
	"cursor/internal/mitm"
	"cursor/internal/netproxy"
)

const (
	// publicAPITimeout represents the publicAPITimeout state value in this module.
	publicAPITimeout = 15 * time.Second
	// backendReadyTimeout represents the maximum time to wait for the embedded backend to become ready.
	backendReadyTimeout = 15 * time.Second
	// backendHealthCheckInterval represents the interval for polling the backend health check.
	backendHealthCheckInterval = 1 * time.Second
	// backendHealthCheckAttemptTimeout limits the duration of a single health check attempt, preventing one blocking attempt from consuming the whole startup budget.
	backendHealthCheckAttemptTimeout = 1 * time.Second
)

// ProxyService defines the ProxyService type in this module.
type ProxyService struct {
	// proxy represents the proxy field in this declaration.
	proxy *mitm.ProxyServer
	// certManager is used to rebuild the MITM service when the proxy listen address changes.
	certManager *certs.Manager
	// backendHost represents the current embedded backend service.
	backendHost *backend.Host

	// mu represents the mu field in this declaration.
	mu sync.RWMutex
	// lastError represents the lastError field in this declaration.
	lastError string
	// cursorSettingsApplied represents whether host proxy settings injection has been completed.
	cursorSettingsApplied bool

	// configMu represents the configMu field in this declaration.
	configMu sync.Mutex
	// configPath represents the configPath field in this declaration.
	configPath string
	// store represents the unified config storage.
	store *serverconfig.Store
	// caCertPEM represents the caCertPEM field in this declaration.
	caCertPEM []byte

	// caFileMu represents the caFileMu field in this declaration.
	caFileMu sync.Mutex
	// caFilePath represents the caFilePath field in this declaration.
	caFilePath string

	// publicClient represents the publicClient field in this declaration.
	publicClient *http.Client
	// logsRoot represents the logsRoot field in this declaration.
	logsRoot string
	// modelTestMu protects the model speed test cache.
	modelTestMu sync.RWMutex
	// modelTestResults stores the model speed test results within the current process.
	modelTestResults map[string]ModelAdapterTestResult
}

// NewProxyService handles logic related to NewProxyService.
func NewProxyService(proxy *mitm.ProxyServer, certManager *certs.Manager, caCertPEM []byte) *ProxyService {
	if err := appdata.EnsureAssistantHome(); err != nil {
		logger.Errorf("ensure assistant home failed: %v", err)
	}
	copiedCert := make([]byte, len(caCertPEM))
	copy(copiedCert, caCertPEM)

	service := &ProxyService{
		proxy:            proxy,
		certManager:      certManager,
		configPath:       resolveUserConfigPath(),
		logsRoot:         resolveLogsRootPath(),
		caCertPEM:        copiedCert,
		publicClient:     netproxy.NewHTTPClient(publicAPITimeout),
		modelTestResults: make(map[string]ModelAdapterTestResult),
	}
	service.store = serverconfig.NewStore(service.configPath, service.logsRoot)
	host, err := backend.NewHost(service.store)
	if err != nil {
		logger.Errorf("init backend host failed: %v", err)
	} else {
		service.backendHost = host
	}
	return service
}

func (s *ProxyService) ensureBackendHost() error {
	if s == nil {
		return nil
	}
	if s.backendHost != nil {
		return nil
	}
	host, err := backend.NewHost(s.store)
	if err != nil {
		return err
	}
	s.backendHost = host
	return nil
}

func (s *ProxyService) ensureProxy(cfg serverconfig.Config) error {
	if s == nil {
		return nil
	}
	baseURL := ""
	if s.backendHost != nil {
		baseURL = s.backendHost.BaseURL()
	}
	if baseURL == "" {
		baseURL = "http://" + cfg.BackendListenAddr
	}
	listenAddr := cfg.ProxyListenAddr

	if s.proxy != nil {
		snapshot := s.proxy.Snapshot()
		if snapshot.ListenAddr == listenAddr {
			return s.proxy.UpdateBaseURL(baseURL)
		}
		if snapshot.Running {
			return fmt.Errorf("proxy is running, cannot switch from %s to %s, please stop service first", snapshot.ListenAddr, listenAddr)
		}
	}

	proxyServer, err := mitm.NewProxyServer(listenAddr, baseURL, "", "", s.certManager)
	if err != nil {
		return err
	}
	s.proxy = proxyServer
	return nil
}

func (s *ProxyService) waitForBackend(ctx context.Context) error {
	if s == nil || s.backendHost == nil {
		return nil
	}
	ticker := time.NewTicker(backendHealthCheckInterval)
	defer ticker.Stop()
	var lastErr error
	for {
		healthCtx, healthCancel := context.WithTimeout(ctx, backendHealthCheckAttemptTimeout)
		err := s.backendHost.HealthCheck(healthCtx)
		healthCancel()
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("failed waiting for embedded backend readiness: %w", lastErr)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
