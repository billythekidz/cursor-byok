package appdata

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func ensureAssistantHome() error {
	migrateLegacyAssistantHome()
	if err := ensurePrivateDir(RootDir()); err != nil {
		return fmt.Errorf("create assistant home: %w", err)
	}
	if err := ensurePrivateDir(DataRootPath()); err != nil {
		return fmt.Errorf("create data root: %w", err)
	}
	if err := ensurePrivateDir(HistoryRootPath()); err != nil {
		return fmt.Errorf("create history root: %w", err)
	}
	if err := ensurePrivateDir(RulesRootPath()); err != nil {
		return fmt.Errorf("create rules root: %w", err)
	}
	if err := ensurePrivateDir(LogsRootPath()); err != nil {
		return fmt.Errorf("create logs root: %w", err)
	}
	if err := ensurePrivateFile(ConfigFilePath(), 0o600); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("harden config file: %w", err)
	}
	return nil
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func ensurePrivateFile(path string, mode os.FileMode) error {
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	return nil
}

func EnsureAssistantHome() error {
	return ensureAssistantHome()
}

func migrateLegacyAssistantHome() {
	legacyRoot := legacyRootDir()
	copyLegacyFile(filepath.Join(legacyRoot, "config.yaml"), filepath.Join(RootDir(), "config.yaml"))
	copyLegacyRules(filepath.Join(legacyRoot, "rules"), RulesRootPath())
	_ = os.RemoveAll(legacyRoot)
}

func copyLegacyRules(sourceRoot string, targetRoot string) {
	_ = filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return nil
		}
		targetPath := filepath.Join(targetRoot, rel)
		if info.IsDir() {
			_ = os.MkdirAll(targetPath, info.Mode().Perm())
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		copyLegacyFile(path, targetPath)
		return nil
	})
}

func copyLegacyFile(sourcePath string, targetPath string) {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return
	}
	defer sourceFile.Close()

	info, err := sourceFile.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return
	}
	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return
	}
	defer targetFile.Close()
	_, _ = io.Copy(targetFile, sourceFile)
}
