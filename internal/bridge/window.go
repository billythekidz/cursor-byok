package bridge

import (
	"cursor/internal/buildinfo"
	"cursor/internal/client"
	"cursor/internal/updater"
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"
	"sync"

	"github.com/leaanthony/u"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// modelEditorContext stores the initialization context of the current model editor window.
type modelEditorContext struct {
	Index       int    `json:"index"`
	AdapterJSON string `json:"adapterJSON"`
}

// WindowService defines the WindowService type in this module.
type WindowService struct {
	app               *application.App
	updater           *updater.Manager
	modelConfigWindow *application.WebviewWindow
	modelEditorWindow *application.WebviewWindow
	editorCtx         *modelEditorContext
	mu                sync.RWMutex
}

// NewWindowService handles logic related to NewWindowService.
func NewWindowService() *WindowService {
	return &WindowService{}
}

// SetApp handles logic related to SetApp.
func (s *WindowService) SetApp(app *application.App) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.app = app
}

// SetUpdater associates the update manager so the frontend can trigger update checks manually.
func (s *WindowService) SetUpdater(manager *updater.Manager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updater = manager
}

// GetAppVersion returns the current application version.
func (s *WindowService) GetAppVersion() string {
	return buildinfo.CurrentVersion()
}

// CheckForUpdates triggers a manual update check.
func (s *WindowService) CheckForUpdates() {
	s.mu.RLock()
	manager := s.updater
	s.mu.RUnlock()
	if manager == nil {
		return
	}
	manager.CheckNow(true)
}

// InstallReadyUpdate installs the update that has already been downloaded.
func (s *WindowService) InstallReadyUpdate() error {
	s.mu.RLock()
	manager := s.updater
	s.mu.RUnlock()
	if manager == nil {
		return fmt.Errorf("update manager not initialized")
	}
	return manager.InstallReadyUpdate()
}

// OpenConfigWindow opens the local settings directory.
func (s *WindowService) OpenConfigWindow() {
	_ = os.MkdirAll(client.ResolveSettingsRootPath(), 0o755)
	openDirectory(client.ResolveSettingsRootPath())
}

// OpenModelConfigWindow opens a standalone model config window. If the window already exists, it is focused.
func (s *WindowService) OpenModelConfigWindow() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.app == nil {
		return
	}

	if s.modelConfigWindow != nil {
		s.modelConfigWindow.Show()
		s.modelConfigWindow.Focus()
		return
	}

	win := s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:               "Model Configuration",
		Width:               980,
		Height:              700,
		MinWidth:            820,
		MinHeight:           560,
		DisableResize:       false,
		Frameless:           goruntime.GOOS == "windows",
		URL:                 "/#/model-config",
		Hidden:              false,
		HideOnEscape:        false,
		MinimiseButtonState: application.ButtonEnabled,
		MaximiseButtonState: application.ButtonEnabled,
		CloseButtonState:    application.ButtonEnabled,
		BackgroundColour:    application.RGBA{Red: 25, Green: 25, Blue: 25, Alpha: 255},
		Mac: application.MacWindow{
			Backdrop:      application.MacBackdropLiquidGlass,
			DisableShadow: false,
			TitleBar: application.MacTitleBar{
				AppearsTransparent:   true,
				Hide:                 false,
				HideTitle:            true,
				FullSizeContent:      true,
				UseToolbar:           false,
				HideToolbarSeparator: true,
			},
			WebviewPreferences: application.MacWebviewPreferences{
				FullscreenEnabled:                   u.True,
				TextInteractionEnabled:              u.True,
				AllowsBackForwardNavigationGestures: u.False,
			},
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: false,
		},
	})

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.modelConfigWindow = nil
	})

	s.modelConfigWindow = win
}

// OpenModelEditorWindow opens a standalone model editor window.
// index < 0 means create a new adapter; >= 0 means edit the adapter at that index.
// adapterJSON is the JSON string of the editor's initial data.
func (s *WindowService) OpenModelEditorWindow(index int, adapterJSON string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.app == nil {
		return
	}

	s.editorCtx = &modelEditorContext{
		Index:       index,
		AdapterJSON: adapterJSON,
	}

	if s.modelEditorWindow != nil {
		s.modelEditorWindow.Show()
		s.modelEditorWindow.Focus()
		return
	}

	title := "Add Model Configuration"
	if index >= 0 {
		title = "Edit Model Configuration"
	}

	win := s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:               title,
		Width:               840,
		Height:              680,
		MinWidth:            740,
		MinHeight:           600,
		DisableResize:       false,
		Frameless:           goruntime.GOOS == "windows",
		URL:                 fmt.Sprintf("/#/model-editor?index=%d", index),
		Hidden:              false,
		HideOnEscape:        false,
		MinimiseButtonState: application.ButtonEnabled,
		MaximiseButtonState: application.ButtonEnabled,
		CloseButtonState:    application.ButtonEnabled,
		BackgroundColour:    application.RGBA{Red: 25, Green: 25, Blue: 25, Alpha: 255},
		Mac: application.MacWindow{
			Backdrop:      application.MacBackdropLiquidGlass,
			DisableShadow: false,
			TitleBar: application.MacTitleBar{
				AppearsTransparent:   true,
				Hide:                 false,
				HideTitle:            true,
				FullSizeContent:      true,
				UseToolbar:           false,
				HideToolbarSeparator: true,
			},
			WebviewPreferences: application.MacWebviewPreferences{
				FullscreenEnabled:                   u.False,
				TextInteractionEnabled:              u.True,
				AllowsBackForwardNavigationGestures: u.False,
			},
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: false,
		},
	})

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.modelEditorWindow = nil
		s.editorCtx = nil
	})

	s.modelEditorWindow = win
}

// GetModelEditorContext returns the initialization context of the current editor window.
func (s *WindowService) GetModelEditorContext() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.editorCtx == nil {
		return map[string]any{
			"index":       -1,
			"adapterJSON": "{}",
		}
	}
	return map[string]any{
		"index":       s.editorCtx.Index,
		"adapterJSON": s.editorCtx.AdapterJSON,
	}
}

// OpenHistoryWindow handles logic related to OpenHistoryWindow.
func (s *WindowService) OpenHistoryWindow() {
	_ = os.MkdirAll(client.ResolveLogsRootPath(), 0o755)
	openDirectory(client.ResolveLogsRootPath())
}

// openDirectory handles logic related to openDirectory.
func openDirectory(path string) {
	if path == "" {
		return
	}
	switch goruntime.GOOS {
	case "darwin":
		_ = exec.Command("open", path).Start()
	case "windows":
		_ = exec.Command("explorer", path).Start()
	default:
		_ = exec.Command("xdg-open", path).Start()
	}
}
