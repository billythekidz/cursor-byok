package appdata

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	appDirName       = ".cursor-local-assistant-v2"
	legacyAppDirName = ".cursor-local-assistant"
)

// RootDir returns the application config root directory.
func RootDir() string {
	return appRootDir(appDirName)
}

func legacyRootDir() string {
	return appRootDir(legacyAppDirName)
}

func appRootDir(dirName string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return dirName
	}
	return filepath.Join(homeDir, dirName)
}

// ConfigFilePath returns the unified user config file path.
func ConfigFilePath() string {
	return filepath.Join(RootDir(), "config.yaml")
}

func DataRootPath() string {
	return filepath.Join(RootDir(), "data")
}

func HistoryRootPath() string {
	return filepath.Join(RootDir(), "history")
}

func UsageFilePath() string {
	return filepath.Join(HistoryRootPath(), "usage.json")
}

func AdsRootPath() string {
	return filepath.Join(DataRootPath(), "ads")
}

func CodebaseIndexRootPath() string {
	return filepath.Join(DataRootPath(), "codebase-index")
}

func DocsIndexRootPath() string {
	return filepath.Join(DataRootPath(), "docs-index")
}

func RulesRootPath() string {
	return filepath.Join(RootDir(), "rules")
}

// LogsRootPath returns the unified logs root directory path.
func LogsRootPath() string {
	return filepath.Join(RootDir(), "logs")
}

// CACertFilePath returns the path of the CA file injected into the host.
func CACertFilePath() string {
	return filepath.Join(DataRootPath(), "ca.crt")
}

// CAKeyFilePath returns the path of the CA private key file injected into the host.
func CAKeyFilePath() string {
	return filepath.Join(DataRootPath(), "ca.key")
}

