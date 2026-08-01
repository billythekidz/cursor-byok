package bridge

import (
	serverconfig "cursor/internal/backend/server/config"
	"cursor/internal/certs"
	"cursor/internal/client"
	"cursor/internal/mitm"
	"runtime"
)

// Public DTOs remain in package main for Wails service compatibility.
// ProxyState defines the ProxyState type in this module.
type ProxyState = client.ProxyState

// UserConfig defines the UserConfig type in this module.
type UserConfig = client.UserConfig

// ModelAdapterConfig defines the model config structure used for model speed testing.
type ModelAdapterConfig = serverconfig.ModelAdapterConfig

// ModelAdapterTestResult defines the result of one model speed test.
type ModelAdapterTestResult = client.ModelAdapterTestResult

// ModelAdapterTestResultsPayload defines the speed test results event payload.
type ModelAdapterTestResultsPayload = client.ModelAdapterTestResultsPayload

// OpenAIModelInfo defines a single model info returned by one OpenAI /v1/models scan.
type OpenAIModelInfo = client.OpenAIModelInfo

// LicenseActionRequest defines the LicenseActionRequest type in this module.
type LicenseActionRequest = client.LicenseActionRequest

// LicenseSwitchDeviceRequest defines the LicenseSwitchDeviceRequest type in this module.
type LicenseSwitchDeviceRequest = client.LicenseSwitchDeviceRequest

// LicenseAPIResult defines the LicenseAPIResult type in this module.
type LicenseAPIResult = client.LicenseAPIResult

// UsageRecordsRequest defines the UsageRecordsRequest type in this module.
type UsageRecordsRequest = client.UsageRecordsRequest

// UsageRecord defines the UsageRecord type in this module.
type UsageRecord = client.UsageRecord

// UsageRecordsData defines the UsageRecordsData type in this module.
type UsageRecordsData = client.UsageRecordsData

// UsageRecordsResult defines the UsageRecordsResult type in this module.
type UsageRecordsResult = client.UsageRecordsResult

// ProxyService defines the ProxyService type in this module.
type ProxyService struct {
	// core represents the core field in this declaration.
	core *client.ProxyService
}

// NewProxyService handles logic related to NewProxyService.
func NewProxyService(proxy *mitm.ProxyServer, certManager *certs.Manager, caCertPEM []byte) *ProxyService {
	return &ProxyService{core: client.NewProxyService(proxy, certManager, caCertPEM)}
}

// StartProxy handles logic related to StartProxy.
func (s *ProxyService) StartProxy() (ProxyState, error) {
	return s.core.StartProxy()
}

// StopProxy handles logic related to StopProxy.
func (s *ProxyService) StopProxy() (ProxyState, error) {
	return s.core.StopProxy()
}

// GetState handles logic related to GetState.
func (s *ProxyService) GetState() ProxyState {
	return s.core.GetState()
}

// ClearLastError handles logic related to ClearLastError.
func (s *ProxyService) ClearLastError() ProxyState {
	return s.core.ClearLastError()
}

// SetBaseURL handles logic related to SetBaseURL.
func (s *ProxyService) SetBaseURL(baseURL string) (ProxyState, error) {
	return s.core.SetBaseURL(baseURL)
}

// LoadUserConfig handles logic related to LoadUserConfig.
func (s *ProxyService) LoadUserConfig() (UserConfig, error) {
	return s.core.LoadUserConfig()
}

// SaveUserConfig handles logic related to SaveUserConfig.
func (s *ProxyService) SaveUserConfig(cfg UserConfig) error {
	return s.core.SaveUserConfig(cfg)
}

// TestModelAdapter handles logic related to TestModelAdapter.
func (s *ProxyService) TestModelAdapter(adapter ModelAdapterConfig) (ModelAdapterTestResult, error) {
	return s.core.TestModelAdapter(adapter)
}

// GetModelAdapterTestResults handles logic related to GetModelAdapterTestResults.
func (s *ProxyService) GetModelAdapterTestResults() []ModelAdapterTestResult {
	return s.core.GetModelAdapterTestResults()
}

// ScanOpenAIModels handles logic related to ScanOpenAIModels.
func (s *ProxyService) ScanOpenAIModels(baseURL string, apiKey string) ([]OpenAIModelInfo, error) {
	return s.core.ScanOpenAIModels(baseURL, apiKey)
}

// GetDeviceID handles logic related to GetDeviceID.
func (s *ProxyService) GetDeviceID() (string, error) {
	return s.core.GetDeviceID()
}

// ActivateLicense handles logic related to ActivateLicense.
func (s *ProxyService) ActivateLicense(req LicenseActionRequest) (LicenseAPIResult, error) {
	return s.core.ActivateLicense(req)
}

// BindLicenseDevice handles logic related to BindLicenseDevice.
func (s *ProxyService) BindLicenseDevice(req LicenseActionRequest) (LicenseAPIResult, error) {
	return s.core.BindLicenseDevice(req)
}

// SwitchLicenseDevice handles logic related to SwitchLicenseDevice.
func (s *ProxyService) SwitchLicenseDevice(req LicenseSwitchDeviceRequest) (LicenseAPIResult, error) {
	return s.core.SwitchLicenseDevice(req)
}

// QueryUsageRecords handles logic related to QueryUsageRecords.
func (s *ProxyService) QueryUsageRecords(req UsageRecordsRequest) (UsageRecordsResult, error) {
	return s.core.QueryUsageRecords(req)
}

// ApplyCursorSettings handles logic related to ApplyCursorSettings.
func (s *ProxyService) ApplyCursorSettings() error {
	return s.core.ApplyCursorSettings()
}

// ClearCursorSettings handles logic related to ClearCursorSettings.
func (s *ProxyService) ClearCursorSettings() error {
	return s.core.ClearCursorSettings()
}

// ShutdownForQuit handles logic related to ShutdownForQuit.
func (s *ProxyService) ShutdownForQuit() {
	s.core.ShutdownForQuit()
}

// IsWindows handles logic related to IsWindows.
func (s *ProxyService) IsWindows() bool {
	return runtime.GOOS == "windows"
}
