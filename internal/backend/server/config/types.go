package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"cursor/internal/modelchannel"
)

const (
	DefaultBackendListenAddr                = "127.0.0.1:18090"
	DefaultProxyListenAddr                  = "127.0.0.1:18080"
	DefaultFrontendBaseURL                  = "http://127.0.0.1"
	DefaultRoutingMode                      = "local"
	DefaultProviderStreamIdleTimeoutSeconds = 240
	MinProviderStreamIdleTimeoutSeconds     = 30
)

type ModelAdapterConfig struct {
	ID                          string `json:"id,omitempty" yaml:"-"`
	DisplayName                 string `json:"displayName" yaml:"displayName"`
	Type                        string `json:"type" yaml:"type"`
	BaseURL                     string `json:"baseURL" yaml:"baseURL"`
	APIKey                      string `json:"apiKey" yaml:"apiKey"`
	TooltipData                 string `json:"tooltipData" yaml:"tooltipData"`
	ModelID                     string `json:"modelID" yaml:"modelID"`
	ReasoningEffort             string `json:"reasoningEffort" yaml:"reasoningEffort"`
	OpenAIEndpoint              string `json:"openAIEndpoint" yaml:"openAIEndpoint"`
	OpenAIExtraParamsEnabled    bool   `json:"openAIExtraParamsEnabled" yaml:"openAIExtraParamsEnabled"`
	OpenAIExtraParamsJSON       string `json:"openAIExtraParamsJSON" yaml:"openAIExtraParamsJSON"`
	CustomHeadersEnabled        bool   `json:"customHeadersEnabled" yaml:"customHeadersEnabled"`
	CustomHeadersJSON           string `json:"customHeadersJSON" yaml:"customHeadersJSON"`
	AnthropicExtraParamsEnabled bool   `json:"anthropicExtraParamsEnabled" yaml:"anthropicExtraParamsEnabled"`
	AnthropicExtraParamsJSON    string `json:"anthropicExtraParamsJSON" yaml:"anthropicExtraParamsJSON"`
	ContextWindowTokens         int    `json:"contextWindowTokens" yaml:"contextWindowTokens"`
	MaxCompletionTokens         int    `json:"maxCompletionTokens" yaml:"maxCompletionTokens"`
	AnthropicMaxTokens          int    `json:"anthropicMaxTokens" yaml:"anthropicMaxTokens"`
	AnthropicThinkingEffort     string `json:"anthropicThinkingEffort,omitempty" yaml:"anthropicThinkingEffort,omitempty"`
	ThinkingBudgetTokens        int    `json:"thinkingBudgetTokens" yaml:"thinkingBudgetTokens"`
	// OpenAIEndpointGroupID identifies the OpenAI endpoint group this adapter belongs to. Adapters added manually leave it empty.
	OpenAIEndpointGroupID string `json:"openAIEndpointGroupID" yaml:"openAIEndpointGroupID"`
	// Active reports whether this adapter is injected into Cursor. At most one model per endpoint group is active.
	Active bool `json:"active" yaml:"active"`
}

type RoutingConfig struct {
	Mode string `json:"mode" yaml:"mode"`
}

type HomeMetricsConfig struct {
	IncludeCacheWriteInHitRate bool `json:"includeCacheWriteInHitRate" yaml:"includeCacheWriteInHitRate"`
}

type Config struct {
	Log                       bool                 `json:"log" yaml:"log"`
	ProviderStreamIdleTimeout int                  `json:"providerStreamIdleTimeout" yaml:"providerStreamIdleTimeout"`
	BackendListenAddr         string               `json:"backendListenAddr" yaml:"backendListenAddr"`
	ProxyListenAddr           string               `json:"proxyListenAddr" yaml:"proxyListenAddr"`
	ModelAdapters             []ModelAdapterConfig `json:"modelAdapters" yaml:"modelAdapters"`
	Routing                   RoutingConfig        `json:"routing" yaml:"routing"`
	HomeMetrics               HomeMetricsConfig    `json:"homeMetrics" yaml:"homeMetrics"`
	LastAgentModelHash        string               `json:"lastAgentModelHash" yaml:"lastAgentModelHash"`
}

func DefaultConfig() Config {
	return Config{
		Log:                       false,
		ProviderStreamIdleTimeout: DefaultProviderStreamIdleTimeoutSeconds,
		BackendListenAddr:         DefaultBackendListenAddr,
		ProxyListenAddr:           DefaultProxyListenAddr,
		ModelAdapters:             []ModelAdapterConfig{},
		Routing: RoutingConfig{
			Mode: DefaultRoutingMode,
		},
	}
}

func NormalizeConfig(input Config) (Config, error) {
	output := DefaultConfig()
	output.Log = input.Log
	output.ProviderStreamIdleTimeout = normalizeProviderStreamIdleTimeout(input.ProviderStreamIdleTimeout)
	backendListenAddr, err := normalizeListenAddr(input.BackendListenAddr, DefaultBackendListenAddr, "backendListenAddr")
	if err != nil {
		return Config{}, err
	}
	proxyListenAddr, err := normalizeListenAddr(input.ProxyListenAddr, DefaultProxyListenAddr, "proxyListenAddr")
	if err != nil {
		return Config{}, err
	}
	output.BackendListenAddr = backendListenAddr
	output.ProxyListenAddr = proxyListenAddr
	output.HomeMetrics.IncludeCacheWriteInHitRate = input.HomeMetrics.IncludeCacheWriteInHitRate
	output.LastAgentModelHash = strings.TrimSpace(input.LastAgentModelHash)
	output.Routing.Mode = normalizeRoutingMode(input.Routing.Mode)
	if output.Routing.Mode == "" {
		output.Routing.Mode = DefaultRoutingMode
	}
	adapters, err := NormalizeModelAdapterConfigs(input.ModelAdapters)
	if err != nil {
		return Config{}, err
	}
	output.ModelAdapters = adapters
	return output, nil
}

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
		nextType := normalizeModelAdapterType(item.Type)
		next := ModelAdapterConfig{
			DisplayName:           strings.TrimSpace(item.DisplayName),
			Type:                  nextType,
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
			return nil, errors.New("model adapter displayName cannot be empty")
		case next.Type == "":
			return nil, errors.New("model adapter type only supports openai or anthropic")
		case next.APIKey == "":
			return nil, errors.New("model adapter apiKey cannot be empty")
		case next.TooltipData == "":
			return nil, errors.New("model adapter tooltipData cannot be empty")
		case next.ModelID == "":
			return nil, errors.New("model adapter modelID cannot be empty")
		case next.Type == "openai" && next.ReasoningEffort == "":
			return nil, errors.New("model adapter reasoningEffort only supports low, medium, high, xhigh, max")
		case next.Type == "openai" && next.OpenAIEndpoint == "":
			return nil, errors.New("model adapter openAIEndpoint only supports /v1/responses, /v1/chat/completions or /custom")
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
			return nil, errors.New("model adapter anthropicThinkingEffort only supports low, medium, high, xhigh, max")
		}
		next.ID = modelchannel.BuildChannelID(next.BaseURL, next.ModelID, next.APIKey, next.DisplayName, next.OpenAIEndpoint)
		if _, exists := seenChannelIDs[next.ID]; exists {
			return nil, errors.New("model adapter channel cannot be duplicated, please check url, modelID, apiKey, displayName, endpoint combination")
		}
		seenChannelIDs[next.ID] = struct{}{}
		normalized = append(normalized, next)
	}
	return normalized, nil
}

func validateJSONMap(value string, fieldName string) error {
	text := strings.TrimSpace(value)
	if text == "" {
		return fmt.Errorf("model adapter %s cannot be empty", fieldName)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return fmt.Errorf("model adapter %s must be a valid JSON object", fieldName)
	}
	if parsed == nil {
		return fmt.Errorf("model adapter %s must be a JSON object", fieldName)
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
		return errors.New("model adapter customHeadersJSON value must be a string")
	}
	for key := range parsed {
		if strings.TrimSpace(key) == "" {
			return errors.New("model adapter customHeadersJSON header name cannot be empty")
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

func normalizeListenAddr(value string, defaultValue string, fieldName string) (string, error) {
	addr := strings.TrimSpace(value)
	if addr == "" {
		addr = defaultValue
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("%s must be in host:port format", fieldName)
	}
	if strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("%s host cannot be empty", fieldName)
	}
	if !isLoopbackHost(host) {
		return "", fmt.Errorf("%s only allows binding loopback addresses (127.0.0.1/::1/localhost)", fieldName)
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		return "", fmt.Errorf("%s port must be between 1-65535", fieldName)
	}
	return net.JoinHostPort(host, strconv.Itoa(parsedPort)), nil
}

func isLoopbackHost(host string) bool {
	value := strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(value, "localhost") {
		return true
	}
	ip := net.ParseIP(value)
	return ip != nil && ip.IsLoopback()
}

func normalizeProviderStreamIdleTimeout(value int) int {
	if value <= 0 {
		return DefaultProviderStreamIdleTimeoutSeconds
	}
	if value < MinProviderStreamIdleTimeoutSeconds {
		return MinProviderStreamIdleTimeoutSeconds
	}
	return value
}

func normalizeMaxCompletionTokens(value int) int {
	if value <= 0 {
		return 0
	}
	return value
}

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

func normalizeRoutingMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "local":
		return "local"
	case "upstream":
		return "upstream"
	default:
		return ""
	}
}
