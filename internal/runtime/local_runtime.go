package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cursor/internal/modelchannel"
)

var (
	// ErrInvalidSystemSetting represents the ErrInvalidSystemSetting state value in this module.
	ErrInvalidSystemSetting = errors.New("invalid system setting")
	// ErrChannelNotAvailable means no model channel is currently available.
	ErrChannelNotAvailable = errors.New("model channel not available")
	// ErrChannelRateLimited means the current model channel is rate limited.
	ErrChannelRateLimited = errors.New("model channel rate limited")
)

const (
	// configurableChannelTimeoutMS represents the configurableChannelTimeoutMS field in this declaration.
	configurableChannelTimeoutMS = int((2 * time.Hour) / time.Millisecond)
	// configurableChannelContextWindowTokens represents the default context window size in this declaration.
	configurableChannelContextWindowTokens = 1_000_000
	// configurableChannelMaxTokens represents the configurableChannelMaxTokens field in this declaration.
	configurableChannelMaxTokens = 65_536
	// configurableChannelThinkingBudgetTokens represents the configurableChannelThinkingBudgetTokens field in this declaration.
	configurableChannelThinkingBudgetTokens = 4_096
	// configurableChannelAnthropicThinkingEffort is the default intensity for Anthropic adaptive thinking.
	configurableChannelAnthropicThinkingEffort = "xhigh"
)

// ModelAdapterConfig defines the ModelAdapterConfig type in this module.
type ModelAdapterConfig struct {
	ID string `json:"id,omitempty"`
	// DisplayName represents the DisplayName field in this declaration.
	DisplayName string `json:"displayName"`
	// Type represents the Type field in this declaration.
	Type string `json:"type"`
	// BaseURL represents the BaseURL field in this declaration.
	BaseURL string `json:"baseURL"`
	// APIKey represents the APIKey field in this declaration.
	APIKey string `json:"apiKey"`
	// TooltipData represents the TooltipData field in this declaration.
	TooltipData string `json:"tooltipData"`
	// ModelID represents the ModelID field in this declaration.
	ModelID string `json:"modelID"`
	// ReasoningEffort represents the ReasoningEffort field in this declaration.
	ReasoningEffort string `json:"reasoningEffort"`
	// OpenAIEndpoint is the API endpoint used by the OpenAI-compatible adapter.
	OpenAIEndpoint string `json:"openAIEndpoint"`
	// OpenAIExtraParamsEnabled indicates whether OpenAI extra request parameters are enabled.
	OpenAIExtraParamsEnabled bool `json:"openAIExtraParamsEnabled"`
	// OpenAIExtraParamsJSON is the JSON object of OpenAI extra request parameters.
	OpenAIExtraParamsJSON string `json:"openAIExtraParamsJSON"`
	// CustomHeadersEnabled indicates whether custom request headers are enabled.
	CustomHeadersEnabled bool `json:"customHeadersEnabled"`
	// CustomHeadersJSON is the JSON object of custom request headers.
	CustomHeadersJSON string `json:"customHeadersJSON"`
	// AnthropicExtraParamsEnabled indicates whether Anthropic extra request parameters are enabled.
	AnthropicExtraParamsEnabled bool `json:"anthropicExtraParamsEnabled"`
	// AnthropicExtraParamsJSON is the JSON object of Anthropic extra request parameters.
	AnthropicExtraParamsJSON string `json:"anthropicExtraParamsJSON"`
	// ContextWindowTokens represents the ContextWindowTokens field in this declaration.
	ContextWindowTokens int `json:"contextWindowTokens"`
	// MaxCompletionTokens represents the MaxCompletionTokens field in this declaration.
	MaxCompletionTokens int `json:"maxCompletionTokens"`
	// AnthropicMaxTokens represents the AnthropicMaxTokens field in this declaration.
	AnthropicMaxTokens int `json:"anthropicMaxTokens"`
	// AnthropicThinkingEffort is the output_config.effort for Anthropic adaptive thinking.
	AnthropicThinkingEffort string `json:"anthropicThinkingEffort,omitempty"`
	// ThinkingBudgetTokens represents the ThinkingBudgetTokens field in this declaration.
	ThinkingBudgetTokens int `json:"thinkingBudgetTokens"`
	// OpenAIEndpointGroupID is the ID of the OpenAI endpoint group the adapter belongs to; manually added adapters leave it empty.
	OpenAIEndpointGroupID string `json:"openAIEndpointGroupID"`
	// Active indicates whether the adapter is injected into Cursor.
	Active bool `json:"active"`
}

// RuntimeConfigSnapshot defines the RuntimeConfigSnapshot type in this module.
type RuntimeConfigSnapshot struct {
	// ObservabilityLogEnabled represents the ObservabilityLogEnabled field in this declaration.
	ObservabilityLogEnabled bool
	// ProviderStreamIdleTimeout is the idle timeout in seconds when a provider streaming response has no valid content.
	ProviderStreamIdleTimeout int
	// ModelAdapters represents the ModelAdapters field in this declaration.
	ModelAdapters []ModelAdapterConfig
}

// RuntimeConfigProvider defines the RuntimeConfigProvider type in this module.
type RuntimeConfigProvider func(context.Context) (RuntimeConfigSnapshot, error)

// NormalizeModelAdapterConfigs handles logic related to NormalizeModelAdapterConfigs.
func NormalizeModelAdapterConfigs(input []ModelAdapterConfig) ([]ModelAdapterConfig, error) {
	if len(input) == 0 {
		return []ModelAdapterConfig{}, nil
	}

	normalized := make([]ModelAdapterConfig, 0, len(input))
	seenChannelIDs := make(map[string]struct{}, len(input))
	for _, item := range input {
		baseURL, err := modelchannel.NormalizeBaseURL(item.BaseURL)
		if err != nil {
			return nil, err
		}
		next := ModelAdapterConfig{
			DisplayName:           strings.TrimSpace(item.DisplayName),
			Type:                  normalizeModelAdapterType(item.Type),
			BaseURL:               baseURL,
			APIKey:                strings.TrimSpace(item.APIKey),
			TooltipData:           strings.TrimSpace(item.TooltipData),
			ModelID:               strings.TrimSpace(item.ModelID),
			ReasoningEffort:       normalizeReasoningEffort(item.ReasoningEffort),
			OpenAIEndpoint:        modelchannel.NormalizeOpenAIEndpoint(item.Type, item.OpenAIEndpoint),
			ContextWindowTokens:   normalizeMaxCompletionTokens(item.ContextWindowTokens),
			MaxCompletionTokens:   normalizeMaxCompletionTokens(item.MaxCompletionTokens),
			AnthropicMaxTokens:    normalizeMaxCompletionTokens(item.AnthropicMaxTokens),
			ThinkingBudgetTokens:  normalizeMaxCompletionTokens(item.ThinkingBudgetTokens),
			OpenAIEndpointGroupID: strings.TrimSpace(item.OpenAIEndpointGroupID),
			Active:                item.Active,
		}
		if next.Type == "openai" {
			next.OpenAIExtraParamsEnabled = item.OpenAIExtraParamsEnabled
			next.OpenAIExtraParamsJSON = strings.TrimSpace(item.OpenAIExtraParamsJSON)
		} else if next.Type == "anthropic" {
			next.AnthropicThinkingEffort = normalizeAnthropicThinkingEffort(item.AnthropicThinkingEffort)
			next.AnthropicExtraParamsEnabled = item.AnthropicExtraParamsEnabled
			next.AnthropicExtraParamsJSON = strings.TrimSpace(item.AnthropicExtraParamsJSON)
		}
		next.CustomHeadersEnabled = item.CustomHeadersEnabled
		next.CustomHeadersJSON = strings.TrimSpace(item.CustomHeadersJSON)
		switch {
		case next.DisplayName == "":
			return nil, errors.New("模型适配器 displayName 不能为空")
		case next.Type == "":
			return nil, errors.New("模型适配器 type 仅支持 openai 或 anthropic")
		case next.APIKey == "":
			return nil, errors.New("模型适配器 apiKey 不能为空")
		case next.TooltipData == "":
			return nil, errors.New("模型适配器 tooltipData 不能为空")
		case next.ModelID == "":
			return nil, errors.New("模型适配器 modelID 不能为空")
		case next.Type == "openai" && next.ReasoningEffort == "":
			return nil, errors.New("模型适配器 reasoningEffort 仅支持 low、medium、high、xhigh、max")
		case next.Type == "openai" && next.OpenAIEndpoint == "":
			return nil, errors.New("模型适配器 openAIEndpoint 仅支持 /v1/responses 或 /v1/chat/completions")
		case next.Type == "openai" && next.OpenAIExtraParamsEnabled:
			if err := validateJSONMap(next.OpenAIExtraParamsJSON, "openAIExtraParamsJSON"); err != nil {
				return nil, err
			}
		case next.CustomHeadersEnabled:
			if err := validateHeadersJSON(next.CustomHeadersJSON); err != nil {
				return nil, err
			}
		case next.Type == "anthropic" && next.AnthropicExtraParamsEnabled:
			if err := validateJSONMap(next.AnthropicExtraParamsJSON, "anthropicExtraParamsJSON"); err != nil {
				return nil, err
			}
		case next.Type == "anthropic" && next.AnthropicThinkingEffort == "":
			return nil, errors.New("模型适配器 anthropicThinkingEffort 仅支持 low、medium、high、xhigh、max")
		}
		next.ID = modelchannel.BuildChannelID(next.BaseURL, next.ModelID, next.APIKey, next.DisplayName, next.OpenAIEndpoint)
		if _, exists := seenChannelIDs[next.ID]; exists {
			return nil, errors.New("模型适配器渠道不能重复，请检查 url、modelID、apiKey、displayName、endpoint 组合")
		}
		seenChannelIDs[next.ID] = struct{}{}
		normalized = append(normalized, next)
	}
	return normalized, nil
}

func validateJSONMap(value string, fieldName string) error {
	text := strings.TrimSpace(value)
	if text == "" {
		return fmt.Errorf("模型适配器 %s 不能为空", fieldName)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return fmt.Errorf("模型适配器 %s 必须是合法 JSON 对象", fieldName)
	}
	if parsed == nil {
		return fmt.Errorf("模型适配器 %s 必须是 JSON 对象", fieldName)
	}
	return nil
}

func validateHeadersJSON(value string) error {
	text := strings.TrimSpace(value)
	if err := validateJSONMap(text, "customHeadersJSON"); err != nil {
		return err
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return errors.New("模型适配器 customHeadersJSON 的值必须是字符串")
	}
	for key := range parsed {
		if strings.TrimSpace(key) == "" {
			return errors.New("模型适配器 customHeadersJSON 的请求头名称不能为空")
		}
	}
	return nil
}

func normalizeReasoningEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "medium":
		return "medium"
	case "low", "high", "xhigh", "max":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeAnthropicThinkingEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "xhigh":
		return "xhigh"
	case "low", "medium", "high", "max":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeMaxCompletionTokens(value int) int {
	if value <= 0 {
		return 0
	}
	return value
}

// normalizeModelAdapterType handles logic related to normalizeModelAdapterType.
func normalizeModelAdapterType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	default:
		return ""
	}
}

// ResolvedChannel represents the currently selected model channel.
type ResolvedChannel struct {
	// ID represents the ID field in this declaration.
	ID string
	// Name represents the Name field in this declaration.
	Name string
	// GroupName represents the GroupName field in this declaration.
	GroupName string
	// Code represents the Code field in this declaration.
	Code string
	// Provider represents the Provider field in this declaration.
	Provider string
	// BaseURL represents the BaseURL field in this declaration.
	BaseURL string
	// APIKey represents the APIKey field in this declaration.
	APIKey string
	// Model represents the Model field in this declaration.
	Model string
	// TimeoutMS represents the TimeoutMS field in this declaration.
	TimeoutMS int
	// ContextWindowTokens represents the ContextWindowTokens field in this declaration.
	ContextWindowTokens int
	// MaxTokens represents the MaxTokens field in this declaration.
	MaxTokens int
	// ReasoningEffort represents the ReasoningEffort field in this declaration.
	ReasoningEffort string
	// OpenAIEndpoint is the API endpoint used by the OpenAI-compatible adapter.
	OpenAIEndpoint string
	// OpenAIExtraParamsEnabled indicates whether OpenAI extra request parameters are enabled.
	OpenAIExtraParamsEnabled bool
	// OpenAIExtraParamsJSON is the JSON object of OpenAI extra request parameters.
	OpenAIExtraParamsJSON string
	// CustomHeadersEnabled indicates whether custom request headers are enabled.
	CustomHeadersEnabled bool
	// CustomHeadersJSON is the JSON object of custom request headers.
	CustomHeadersJSON string
	// AnthropicExtraParamsEnabled indicates whether Anthropic extra request parameters are enabled.
	AnthropicExtraParamsEnabled bool
	// AnthropicExtraParamsJSON is the JSON object of Anthropic extra request parameters.
	AnthropicExtraParamsJSON string
	// AnthropicMaxTokens represents the AnthropicMaxTokens field in this declaration.
	AnthropicMaxTokens int
	// AnthropicThinkingEffort is the output_config.effort for Anthropic adaptive thinking.
	AnthropicThinkingEffort string
	// ThinkingEnabled represents the ThinkingEnabled field in this declaration.
	ThinkingEnabled bool
	// ThinkingBudgetTokens represents the ThinkingBudgetTokens field in this declaration.
	ThinkingBudgetTokens int
}

// ChannelUsageRecordCreatePayload defines the minimal payload for one channel usage record.
type ChannelUsageRecordCreatePayload struct {
	// RequestID represents the RequestID field in this declaration.
	RequestID string
	// ConversationID represents the ConversationID field in this declaration.
	ConversationID string
	// RuntimeModelID represents the RuntimeModelID field in this declaration.
	RuntimeModelID string
}

// ChannelCallRecordCreatePayload defines the minimal payload for one channel call record.
type ChannelCallRecordCreatePayload struct {
	// RequestID represents the RequestID field in this declaration.
	RequestID string
	// ConversationID represents the ConversationID field in this declaration.
	ConversationID string
	// ChannelID represents the ChannelID field in this declaration.
	ChannelID string
	// ChannelName represents the ChannelName field in this declaration.
	ChannelName string
	// GroupName represents the GroupName field in this declaration.
	GroupName string
	// Provider represents the Provider field in this declaration.
	Provider string
	// RuntimeModelID represents the RuntimeModelID field in this declaration.
	RuntimeModelID string
	// ProviderModelID represents the ProviderModelID field in this declaration.
	ProviderModelID string
	// StatusCode represents the StatusCode field in this declaration.
	StatusCode int
	// Success represents the Success field in this declaration.
	Success bool
	// DurationMS represents the DurationMS field in this declaration.
	DurationMS int64
	// ErrorCode represents the ErrorCode field in this declaration.
	ErrorCode string
	// ErrorMessage represents the ErrorMessage field in this declaration.
	ErrorMessage string
}

// FixedChannelService defines the FixedChannelService type in this module.
type FixedChannelService struct {
	// channel represents the channel field in this declaration.
	channel ResolvedChannel
	// configProvider represents the configProvider field in this declaration.
	configProvider RuntimeConfigProvider
}

// NewFixedChannelService handles logic related to NewFixedChannelService.
func NewFixedChannelService(channel ResolvedChannel, logsRoot string) *FixedChannelService {
	_ = logsRoot
	return &FixedChannelService{
		channel: channel,
	}
}

// NewConfigurableChannelService handles logic related to NewConfigurableChannelService.
func NewConfigurableChannelService(provider RuntimeConfigProvider, logsRoot string) *FixedChannelService {
	_ = logsRoot
	return &FixedChannelService{
		configProvider: provider,
	}
}

// SelectChannelForRequestBody handles logic related to SelectChannelForRequestBody.
func (s *FixedChannelService) SelectChannelForRequestBody(_ context.Context, _ []byte) (*ResolvedChannel, error) {
	return s.SelectChannelForModel(context.Background(), "")
}

// SelectChannelForModel handles logic related to SelectChannelForModel.
func (s *FixedChannelService) SelectChannelForModel(ctx context.Context, modelID string) (*ResolvedChannel, error) {
	if s == nil {
		return nil, ErrChannelNotAvailable
	}
	if s.configProvider != nil {
		cfg, err := s.configProvider(ctx)
		if err != nil {
			return nil, err
		}
		adapters, err := NormalizeModelAdapterConfigs(cfg.ModelAdapters)
		if err != nil {
			return nil, err
		}
		matchIndex, ok := modelchannel.ResolveAdapterIndex(
			adapters,
			modelID,
			func(adapter ModelAdapterConfig) string { return adapter.ID },
			func(adapter ModelAdapterConfig) string { return adapter.ModelID },
			func(adapter ModelAdapterConfig) string {
				return modelchannel.BuildLegacyChannelID(adapter.BaseURL, adapter.ModelID, adapter.APIKey, adapter.DisplayName)
			},
		)
		if !ok {
			return nil, ErrChannelNotAvailable
		}
		adapter := adapters[matchIndex]
		resolved := ResolvedChannel{
			ID:                          strings.TrimSpace(adapter.ID),
			Name:                        strings.TrimSpace(adapter.DisplayName),
			GroupName:                   "local",
			Code:                        strings.TrimSpace(adapter.ID),
			Provider:                    strings.TrimSpace(adapter.Type),
			BaseURL:                     strings.TrimSpace(adapter.BaseURL),
			APIKey:                      strings.TrimSpace(adapter.APIKey),
			Model:                       strings.TrimSpace(adapter.ModelID),
			TimeoutMS:                   configurableChannelTimeoutMS,
			ContextWindowTokens:         configurableChannelContextWindowTokens,
			MaxTokens:                   configurableChannelMaxTokens,
			ReasoningEffort:             strings.TrimSpace(adapter.ReasoningEffort),
			OpenAIEndpoint:              strings.TrimSpace(adapter.OpenAIEndpoint),
			OpenAIExtraParamsEnabled:    adapter.OpenAIExtraParamsEnabled,
			OpenAIExtraParamsJSON:       strings.TrimSpace(adapter.OpenAIExtraParamsJSON),
			CustomHeadersEnabled:        adapter.CustomHeadersEnabled,
			CustomHeadersJSON:           strings.TrimSpace(adapter.CustomHeadersJSON),
			AnthropicExtraParamsEnabled: adapter.AnthropicExtraParamsEnabled,
			AnthropicExtraParamsJSON:    strings.TrimSpace(adapter.AnthropicExtraParamsJSON),
			AnthropicMaxTokens:          configurableChannelMaxTokens,
			AnthropicThinkingEffort:     configurableChannelAnthropicThinkingEffort,
			ThinkingEnabled:             true,
			ThinkingBudgetTokens:        configurableChannelThinkingBudgetTokens,
		}
		if adapter.ContextWindowTokens > 0 {
			resolved.ContextWindowTokens = adapter.ContextWindowTokens
		}
		if adapter.MaxCompletionTokens > 0 {
			resolved.MaxTokens = adapter.MaxCompletionTokens
		}
		if adapter.ThinkingBudgetTokens > 0 {
			resolved.ThinkingBudgetTokens = adapter.ThinkingBudgetTokens
		}
		if adapter.AnthropicMaxTokens > 0 {
			resolved.AnthropicMaxTokens = adapter.AnthropicMaxTokens
		}
		if strings.TrimSpace(adapter.AnthropicThinkingEffort) != "" {
			resolved.AnthropicThinkingEffort = strings.TrimSpace(adapter.AnthropicThinkingEffort)
		}
		return &resolved, nil
	}
	if strings.TrimSpace(s.channel.BaseURL) == "" || strings.TrimSpace(s.channel.APIKey) == "" {
		return nil, ErrChannelNotAvailable
	}
	resolved := s.channel
	return &resolved, nil
}

// RecordRunRequestUsage handles logic related to RecordRunRequestUsage.
func (s *FixedChannelService) RecordRunRequestUsage(_ context.Context, payload ChannelUsageRecordCreatePayload) error {
	_ = s
	_ = payload
	return nil
}

// RecordChannelCall handles logic related to RecordChannelCall.
func (s *FixedChannelService) RecordChannelCall(_ context.Context, payload ChannelCallRecordCreatePayload) error {
	_ = s
	_ = payload
	return nil
}

// LocalSystemSettingService defines the LocalSystemSettingService type in this module.
type LocalSystemSettingService struct {
	// provider represents the provider field in this declaration.
	provider RuntimeConfigProvider
}

// NewLocalSystemSettingService handles logic related to NewLocalSystemSettingService.
func NewLocalSystemSettingService(provider RuntimeConfigProvider) *LocalSystemSettingService {
	return &LocalSystemSettingService{provider: provider}
}

// ResolveFrontendBaseURL handles logic related to ResolveFrontendBaseURL.
func (s *LocalSystemSettingService) ResolveFrontendBaseURL(context.Context) (string, error) {
	return "http://127.0.0.1", nil
}

// IsObservabilityLogEnabled handles logic related to IsObservabilityLogEnabled.
func (s *LocalSystemSettingService) IsObservabilityLogEnabled(ctx context.Context) bool {
	cfg, err := s.load(ctx)
	if err != nil {
		return true
	}
	return cfg.ObservabilityLogEnabled
}

// IsAgentRuntimeModelEnabled handles logic related to IsAgentRuntimeModelEnabled.
func (s *LocalSystemSettingService) IsAgentRuntimeModelEnabled(context.Context) bool {
	return true
}

// ResolveCursorServerBridge handles logic related to ResolveCursorServerBridge.
func (s *LocalSystemSettingService) ResolveCursorServerBridge(context.Context) (string, bool) {
	return "", false
}

// LoadRuntimeConfigSnapshot handles logic related to LoadRuntimeConfigSnapshot.
func (s *LocalSystemSettingService) LoadRuntimeConfigSnapshot(ctx context.Context) (RuntimeConfigSnapshot, error) {
	return s.load(ctx)
}

// ResolveModelAdapters handles logic related to ResolveModelAdapters.
func (s *LocalSystemSettingService) ResolveModelAdapters(ctx context.Context) ([]ModelAdapterConfig, error) {
	cfg, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return NormalizeModelAdapterConfigs(cfg.ModelAdapters)
}

// load handles logic related to load.
func (s *LocalSystemSettingService) load(ctx context.Context) (RuntimeConfigSnapshot, error) {
	if s == nil || s.provider == nil {
		return RuntimeConfigSnapshot{}, nil
	}
	return s.provider(ctx)
}
