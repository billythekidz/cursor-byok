package client

import (
	"encoding/json"
	"errors"
)

// LicenseActionRequest defines the LicenseActionRequest type in this module.
type LicenseActionRequest struct {
	// Host represents the Host field in this declaration.
	Host string `json:"host"`
	// Code represents the Code field in this declaration.
	Code string `json:"code"`
	// DeviceID represents the DeviceID field in this declaration.
	DeviceID string `json:"deviceId"`
	// DeviceMeta represents the DeviceMeta field in this declaration.
	DeviceMeta string `json:"deviceMeta"`
}

// LicenseSwitchDeviceRequest defines the LicenseSwitchDeviceRequest type in this module.
type LicenseSwitchDeviceRequest struct {
	// Host represents the Host field in this declaration.
	Host string `json:"host"`
	// Code represents the Code field in this declaration.
	Code string `json:"code"`
	// FromDeviceID represents the FromDeviceID field in this declaration.
	FromDeviceID string `json:"fromDeviceId"`
	// ToDeviceID represents the ToDeviceID field in this declaration.
	ToDeviceID string `json:"toDeviceId"`
	// Remark represents the Remark field in this declaration.
	Remark string `json:"remark"`
}

// LicenseAPIResult defines the LicenseAPIResult type in this module.
type LicenseAPIResult struct {
	// Code represents the Code field in this declaration.
	Code string `json:"code"`
	// Message represents the Message field in this declaration.
	Message string `json:"message"`
	// Data represents the Data field in this declaration.
	Data map[string]any `json:"data,omitempty"`
}

// UsageRecordsRequest defines the UsageRecordsRequest type in this module.
type UsageRecordsRequest struct {
	// Host represents the Host field in this declaration.
	Host string `json:"host"`
	// Code represents the Code field in this declaration.
	Code string `json:"code"`
	// Page represents the Page field in this declaration.
	Page int `json:"page"`
	// PageSize represents the PageSize field in this declaration.
	PageSize int `json:"pageSize"`
	// StartTime represents the StartTime field in this declaration.
	StartTime string `json:"startTime"`
	// EndTime represents the EndTime field in this declaration.
	EndTime string `json:"endTime"`
	// RequestID represents the RequestID field in this declaration.
	RequestID string `json:"requestId"`
	// ConversationID represents the ConversationID field in this declaration.
	ConversationID string `json:"conversationId"`
	// RuntimeModelID represents the RuntimeModelID field in this declaration.
	RuntimeModelID string `json:"runtimeModelId"`
}

// UsageRecord defines the UsageRecord type in this module.
type UsageRecord struct {
	// CreatedAt represents the CreatedAt field in this declaration.
	CreatedAt string `json:"createdAt"`
	// RuntimeModelID represents the RuntimeModelID field in this declaration.
	RuntimeModelID string `json:"runtimeModelId"`
	// RequestID represents the RequestID field in this declaration.
	RequestID string `json:"requestId"`
	// ConversationID represents the ConversationID field in this declaration.
	ConversationID string `json:"conversationId"`
}

// UsageRecordsData defines the UsageRecordsData type in this module.
type UsageRecordsData struct {
	// Items represents the Items field in this declaration.
	Items []UsageRecord `json:"items"`
	// Total represents the Total field in this declaration.
	Total int `json:"total"`
	// Page represents the Page field in this declaration.
	Page int `json:"page"`
	// PageSize represents the PageSize field in this declaration.
	PageSize int `json:"pageSize"`
}

// UsageRecordsResult defines the UsageRecordsResult type in this module.
type UsageRecordsResult struct {
	// Code represents the Code field in this declaration.
	Code string `json:"code"`
	// Message represents the Message field in this declaration.
	Message string `json:"message"`
	// Data represents the Data field in this declaration.
	Data UsageRecordsData `json:"data"`
}

// ActivateLicense handles logic related to ActivateLicense.
func (s *ProxyService) ActivateLicense(LicenseActionRequest) (LicenseAPIResult, error) {
	return LicenseAPIResult{}, errors.New("activation has been removed from the local client")
}

// BindLicenseDevice handles logic related to BindLicenseDevice.
func (s *ProxyService) BindLicenseDevice(LicenseActionRequest) (LicenseAPIResult, error) {
	return LicenseAPIResult{}, errors.New("device binding has been removed from the local client")
}

// SwitchLicenseDevice handles logic related to SwitchLicenseDevice.
func (s *ProxyService) SwitchLicenseDevice(LicenseSwitchDeviceRequest) (LicenseAPIResult, error) {
	return LicenseAPIResult{}, errors.New("device switching has been removed from the local client")
}

// QueryUsageRecords handles logic related to QueryUsageRecords.
func (s *ProxyService) QueryUsageRecords(req UsageRecordsRequest) (UsageRecordsResult, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	return UsageRecordsResult{
		Code:    "UNSUPPORTED",
		Message: "usage records UI has been removed from the local client",
		Data: UsageRecordsData{
			Items:    []UsageRecord{},
			Total:    0,
			Page:     page,
			PageSize: pageSize,
		},
	}, nil
}

// MarshalJSON handles logic related to MarshalJSON.
func (result LicenseAPIResult) MarshalJSON() ([]byte, error) {
	type alias LicenseAPIResult
	output := alias(result)
	if output.Data == nil {
		output.Data = map[string]any{}
	}
	return json.Marshal(output)
}
