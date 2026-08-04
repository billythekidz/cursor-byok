package client

import (
	"errors"
	"fmt"
	goruntime "runtime"

	"cursor/internal/cursor"
)

// ApplyCursorSettings handles logic related to ApplyCursorSettings.
func (s *ProxyService) ApplyCursorSettings() error {
	if s == nil || s.proxy == nil {
		return fmt.Errorf("proxy is not initialized")
	}
	s.caFileMu.Lock()
	caCertPath, err := cursor.EnsureCACertFile(s.caCertPEM, s.caFilePath)
	if err == nil {
		s.caFilePath = caCertPath
	}
	s.caFileMu.Unlock()
	if err != nil {
		return fmt.Errorf("ensure ca cert file: %w", err)
	}

	switch goruntime.GOOS {
	case "windows":
		if err := cursor.EnsureCACertInstalled(s.caCertPEM, caCertPath); err != nil {
			return fmt.Errorf("install ca cert: %w", err)
		}
	case "darwin":
		if err := cursor.EnsureCACertInstalled(s.caCertPEM, caCertPath); err != nil {
			return fmt.Errorf("install ca cert: %w", err)
		}
		if err := cursor.SetSystemNodeExtraCACerts(caCertPath); err != nil {
			return fmt.Errorf("set node extra ca certs: %w", err)
		}
	}

	if err := cursor.WriteUserProxySettings(cursor.ProxyURLFromListenAddr(s.proxy.Snapshot().ListenAddr)); err != nil {
		return err
	}
	s.setCursorSettingsApplied(true)
	return nil
}

// ClearCursorSettings handles logic related to ClearCursorSettings.
func (s *ProxyService) ClearCursorSettings() error {
	var cleanupErr error
	if goruntime.GOOS == "darwin" {
		cleanupErr = errors.Join(cleanupErr, cursor.ClearSystemNodeExtraCACerts())
	}
	if goruntime.GOOS == "darwin" {
		cleanupErr = errors.Join(cleanupErr, cursor.RemoveCACertFromDarwinKeychain(s.caCertPEM))
	}
	cleanupErr = errors.Join(cleanupErr, cursor.ClearUserProxySettings())
	if cleanupErr != nil {
		return cleanupErr
	}
	s.setCursorSettingsApplied(false)
	return nil
}

// GetDeviceID handles logic related to GetDeviceID.
func (s *ProxyService) GetDeviceID() (string, error) {
	return cursor.GetDeviceID()
}
