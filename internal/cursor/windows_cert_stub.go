//go:build !windows && !darwin

package cursor

import "fmt"

// EnsureCACertInstalled is the stub implementation for non-Windows/macOS platforms.
func EnsureCACertInstalled(_ []byte, certPath string) error {
	return fmt.Errorf("ensureCACertInstalled: platform currently not supported, certPath=%s", certPath)
}

func RemoveCACertFromWindowsStore(_ []byte) error { return nil }
