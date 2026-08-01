package client

import (
	"context"

	"cursor/internal/appdata"
	serverconfig "cursor/internal/backend/server/config"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// UserConfig defines the UserConfig type in this module.
type UserConfig = serverconfig.Config

// LoadUserConfig handles logic related to LoadUserConfig.
func (s *ProxyService) LoadUserConfig() (UserConfig, error) {
	if s == nil {
		return serverconfig.DefaultConfig(), nil
	}
	app := application.Get()
	ctx := context.Background()
	if app != nil {
		ctx = app.Context()
	}
	if s.backendHost != nil {
		return s.backendHost.LoadConfig(ctx)
	}
	if s.store == nil {
		return serverconfig.DefaultConfig(), nil
	}
	return s.store.Load(ctx)
}

// SaveUserConfig handles logic related to SaveUserConfig.
func (s *ProxyService) SaveUserConfig(cfg UserConfig) error {
	if s == nil {
		return nil
	}
	app := application.Get()
	ctx := context.Background()
	if app != nil {
		ctx = app.Context()
	}
	var (
		normalized UserConfig
		err        error
	)
	if s.backendHost != nil {
		normalized, err = s.backendHost.SaveConfig(ctx, cfg)
	} else if s.store != nil {
		normalized, err = s.store.Save(ctx, cfg)
	} else {
		return nil
	}
	if err != nil {
		return err
	}
	s.emitUserConfigChanged(normalized)
	return nil
}

func (s *ProxyService) emitUserConfigChanged(cfg UserConfig) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit("user-config:changed", cfg)
}

// resolveUserConfigPath handles logic related to resolveUserConfigPath.
func resolveUserConfigPath() string {
	return appdata.ConfigFilePath()
}

// resolveLogsRootPath handles logic related to resolveLogsRootPath.
func resolveLogsRootPath() string {
	return appdata.LogsRootPath()
}

// ResolveLogsRootPath handles logic related to ResolveLogsRootPath.
func ResolveLogsRootPath() string {
	return resolveLogsRootPath()
}

// ResolveSettingsRootPath handles logic related to ResolveSettingsRootPath.
func ResolveSettingsRootPath() string {
	return appdata.RootDir()
}
