package client

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"cursor/internal/backend/codex"
	"cursor/internal/logger"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const codexRuntimeUpdatedEvent = "codex-runtime:updated"

type CodexRuntimeStatus = codex.RuntimeStatus
type CodexInstallResult = codex.InstallResult

func (s *ProxyService) GetCodexRuntimeStatus() (CodexRuntimeStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return codex.GetRuntimeStatus(ctx), nil
}

func (s *ProxyService) InstallCodex() (CodexInstallResult, error) {
	if s == nil {
		return CodexInstallResult{Error: "proxy service is unavailable"}, fmt.Errorf("proxy service is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	s.codexSetupMu.Lock()
	if s.codexSetupCancel != nil {
		s.codexSetupMu.Unlock()
		cancel()
		return CodexInstallResult{Error: "Codex setup is already running"}, fmt.Errorf("Codex setup is already running")
	}
	s.codexSetupCancel = cancel
	s.codexSetupMu.Unlock()
	defer func() {
		s.codexSetupMu.Lock()
		s.codexSetupCancel = nil
		s.codexSetupMu.Unlock()
		cancel()
	}()
	result := codex.Install(ctx, func(line string) {
		s.emitCodexRuntimeEvent(map[string]any{"phase": "installing", "output": line})
	})
	s.emitCodexRuntimeEvent(map[string]any{"phase": "install_complete", "success": result.Success, "error": result.Error})
	return result, nil
}

func (s *ProxyService) StartCodexLogin() error {
	return s.startCodexLogin([]string{"login"}, "logging_in")
}

// StartCodexDeviceLogin starts the headless Codex device-auth fallback.
func (s *ProxyService) StartCodexDeviceLogin() error {
	return s.startCodexLogin([]string{"login", "--device-auth"}, "device_logging_in")
}

func (s *ProxyService) startCodexLogin(args []string, phase string) error {
	if s == nil {
		return fmt.Errorf("proxy service is unavailable")
	}
	binaryPath := codex.FindBinary()
	if binaryPath == "" {
		return fmt.Errorf("Codex is not installed")
	}
	s.codexSetupMu.Lock()
	if s.codexLoginCmd != nil {
		s.codexSetupMu.Unlock()
		return fmt.Errorf("Codex login is already running")
	}
	cmd := exec.Command(binaryPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		s.codexSetupMu.Unlock()
		return fmt.Errorf("start Codex login: %w", err)
	}
	s.codexLoginCmd = cmd
	s.codexSetupMu.Unlock()
	s.emitCodexRuntimeEvent(map[string]any{"phase": phase})
	go func() {
		err := cmd.Wait()
		s.codexSetupMu.Lock()
		if s.codexLoginCmd == cmd {
			s.codexLoginCmd = nil
		}
		s.codexSetupMu.Unlock()
		payload := map[string]any{"phase": "login_complete", "success": err == nil}
		if err != nil {
			payload["error"] = "Codex login did not complete successfully"
			logger.Errorf("Codex login exited: %v", err)
		}
		s.emitCodexRuntimeEvent(payload)
	}()
	return nil
}

func (s *ProxyService) CancelCodexSetup() error {
	if s == nil {
		return nil
	}
	s.codexSetupMu.Lock()
	cancel := s.codexSetupCancel
	loginCmd := s.codexLoginCmd
	s.codexSetupCancel = nil
	s.codexLoginCmd = nil
	s.codexSetupMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if loginCmd != nil && loginCmd.Process != nil {
		_ = loginCmd.Process.Kill()
	}
	s.emitCodexRuntimeEvent(map[string]any{"phase": "cancelled"})
	return nil
}

func (s *ProxyService) emitCodexRuntimeEvent(payload map[string]any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(codexRuntimeUpdatedEvent, payload)
	}
}
