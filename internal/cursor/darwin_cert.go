//go:build darwin

package cursor

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os/exec"
	"strings"

	"cursor/internal/logger"
)

const (
	darwinSecurityExe       = "security"
	darwinLoginKeychainName = "login.keychain-db"
)

func getCertSHA1Fingerprint(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to parse certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse certificate: %w", err)
	}
	fingerprint := fmt.Sprintf("%X", sha1.Sum(cert.Raw))
	return fingerprint, nil
}

func isCACertInstalled(certPEM []byte) (bool, error) {
	fingerprint, err := getCertSHA1Fingerprint(certPEM)
	if err != nil {
		return false, fmt.Errorf("failed to get certificate fingerprint: %w", err)
	}

	out, err := exec.Command(darwinSecurityExe, "find-certificate", "-a", "-Z", darwinLoginKeychainName).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to check macOS login keychain: %w: %s", err, strings.TrimSpace(string(out)))
	}
	installed := strings.Contains(strings.ToUpper(string(out)), fingerprint)
	if installed {
		logger.Infof("isCACertInstalled: cert found in macOS login keychain, fingerprint=%s", fingerprint)
	} else {
		logger.Infof("isCACertInstalled: cert not found in macOS login keychain, fingerprint=%s", fingerprint)
	}
	return installed, nil
}

func installCACertToDarwinKeychain(certPEM []byte, certPath string) error {
	fingerprint, err := getCertSHA1Fingerprint(certPEM)
	if err != nil {
		return fmt.Errorf("failed to get certificate fingerprint: %w", err)
	}

	logger.Infof("installCACertToDarwinKeychain: installing cert into login keychain, path=%s fingerprint=%s", certPath, fingerprint)
	out, err := exec.Command(
		darwinSecurityExe,
		"add-trusted-cert",
		"-r", "trustRoot",
		"-p", "ssl",
		"-k", darwinLoginKeychainName,
		certPath,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install CA into macOS login keychain: %w: %s", err, strings.TrimSpace(string(out)))
	}

	installed, err := isCACertInstalled(certPEM)
	if err != nil {
		return fmt.Errorf("failed to verify macOS certificate installation status: %w", err)
	}
	if !installed {
		return fmt.Errorf("certificate import executed but certificate not found in macOS login keychain")
	}

	logger.Infof("installCACertToDarwinKeychain: cert installed successfully, fingerprint=%s", fingerprint)
	return nil
}

// EnsureCACertInstalled ensures the CA certificate is installed into the macOS login keychain.
func EnsureCACertInstalled(certPEM []byte, certPath string) error {
	installed, err := isCACertInstalled(certPEM)
	if err != nil {
		return fmt.Errorf("failed to check macOS certificate installation status: %w", err)
	}
	if installed {
		logger.Infof("ensureCACertInstalled: cert already installed in macOS login keychain, skipping")
		return nil
	}

	logger.Infof("ensureCACertInstalled: cert not installed in macOS login keychain, installing...")
	return installCACertToDarwinKeychain(certPEM, certPath)
}

// RemoveCACertFromDarwinKeychain removes this app's CA from the login keychain.
func RemoveCACertFromDarwinKeychain(certPEM []byte) error {
	fingerprint, err := getCertSHA1Fingerprint(certPEM)
	if err != nil {
		return err
	}
	out, err := exec.Command(darwinSecurityExe, "delete-certificate", "-Z", fingerprint, darwinLoginKeychainName).CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "could not be found") {
		return fmt.Errorf("failed to delete CA from macOS login keychain: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
