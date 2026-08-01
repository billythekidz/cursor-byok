//go:build windows

package cursor

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"cursor/internal/logger"
)

const (
	windowsRootStoreName  = "Root"
	windowsUserStoreFlag  = "-user"
	windowsCertutilExe    = "certutil.exe"
)

// getCertThumbprint gets the SHA1 thumbprint of the certificate, used to uniquely identify it.
func getCertThumbprint(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to parse certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse certificate: %w", err)
	}
	// SHA1 thumbprint; certutil uses this format.
	thumbprint := fmt.Sprintf("%X", sha1.Sum(cert.Raw))
	return thumbprint, nil
}

// hideWindow returns a SysProcAttr that hides the command-line window.
func hideWindow() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow: true,
	}
}

// isCACertInstalled checks whether the CA certificate is already installed in the current user's Windows root certificate store.
func isCACertInstalled(certPEM []byte) (bool, error) {
	thumbprint, err := getCertThumbprint(certPEM)
	if err != nil {
		return false, fmt.Errorf("failed to get certificate fingerprint: %w", err)
	}

	cmd := exec.Command(windowsCertutilExe, windowsUserStoreFlag, "-verifystore", windowsRootStoreName, thumbprint)
	cmd.SysProcAttr = hideWindow()
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// certutil returns a non-zero exit code when it cannot find the certificate.
			logger.Infof("isCACertInstalled: cert not found in system store, thumbprint=%s exitCode=%d", thumbprint, exitErr.ExitCode())
			return false, nil
		}
		return false, fmt.Errorf("failed to execute certutil checking system cert store: %w", err)
	}

	outStr := strings.ToUpper(string(output))
	if strings.Contains(outStr, thumbprint) {
		logger.Infof("isCACertInstalled: cert found in system store, thumbprint=%s", thumbprint)
		return true, nil
	}

	// The certutil output text differs in some Windows locales; it is still treated as not found here.
	logger.Infof("isCACertInstalled: cert not found in certutil output, thumbprint=%s", thumbprint)
	return false, nil
}

// installCACertToWindowsStore installs the CA certificate into the current user's Windows root certificate store.
func installCACertToWindowsStore(certPEM []byte, certPath string) error {
	thumbprint, err := getCertThumbprint(certPEM)
	if err != nil {
		return fmt.Errorf("failed to get certificate fingerprint: %w", err)
	}

	logger.Infof("installCACertToWindowsStore: installing cert into system store, path=%s thumbprint=%s", certPath, thumbprint)

	cmd := exec.Command(windowsCertutilExe, windowsUserStoreFlag, "-addstore", windowsRootStoreName, certPath)
	cmd.SysProcAttr = hideWindow()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to write to current user Windows trust store: %w: %s", err, strings.TrimSpace(string(output)))
	}

	installed, err := isCACertInstalled(certPEM)
	if err != nil {
		return fmt.Errorf("failed to verify system cert installation status: %w", err)
	}
	if !installed {
		return fmt.Errorf("certificate import executed but certificate not found in system trust store")
	}

	logger.Infof("installCACertToWindowsStore: cert installed successfully into system store, thumbprint=%s", thumbprint)
	return nil
}

// RemoveCACertFromWindowsStore removes this app's CA from the current user's Root store.
func RemoveCACertFromWindowsStore(certPEM []byte) error {
	thumbprint, err := getCertThumbprint(certPEM)
	if err != nil {
		return err
	}
	cmd := exec.Command(windowsCertutilExe, windowsUserStoreFlag, "-delstore", windowsRootStoreName, thumbprint)
	cmd.SysProcAttr = hideWindow()
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(output)), "not found") {
		return fmt.Errorf("failed to delete CA from current user Windows store: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// EnsureCACertInstalled ensures the certificate is installed into the Windows system trust store.
func EnsureCACertInstalled(certPEM []byte, certPath string) error {
	installed, err := isCACertInstalled(certPEM)
	if err != nil {
		return fmt.Errorf("failed to check system cert installation status: %w", err)
	}

	if installed {
		logger.Infof("ensureCACertInstalled: cert already installed in system store, skipping")
		return nil
	}

	logger.Infof("ensureCACertInstalled: cert not installed in system store, installing...")
	return installCACertToWindowsStore(certPEM, certPath)
}
