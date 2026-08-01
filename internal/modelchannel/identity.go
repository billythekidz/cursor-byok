package modelchannel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

const ChannelIDHexLength = 16

const (
	OpenAIEndpointResponses       = "/v1/responses"
	OpenAIEndpointChatCompletions = "/v1/chat/completions"
	OpenAIEndpointCustom          = "/custom"
)

func NormalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("model adapter baseURL cannot be empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("model adapter baseURL is not a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("model adapter baseURL only supports http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("model adapter baseURL is missing hostname")
	}
	parsed.Scheme = strings.ToLower(strings.TrimSpace(parsed.Scheme))
	parsed.Host = strings.ToLower(strings.TrimSpace(parsed.Host))
	normalized := strings.TrimRight(parsed.String(), "/")
	if normalized == "" {
		normalized = parsed.String()
	}
	return normalized, nil
}

// NormalizeOpenAIEndpoint normalizes the OpenAI endpoint path.
// Supports three preset values: /v1/responses, /v1/chat/completions, and /custom (custom path).
// When /custom is selected, the user must fill in the full request URL in the endpoint address field.
func NormalizeOpenAIEndpoint(providerType string, endpoint string) string {
	if strings.TrimSpace(strings.ToLower(providerType)) != "openai" {
		return ""
	}
	normalized := strings.TrimSpace(endpoint)
	switch normalized {
	case "":
		return OpenAIEndpointResponses
	case OpenAIEndpointResponses, OpenAIEndpointChatCompletions, OpenAIEndpointCustom:
		return normalized
	default:
		return ""
	}
}

// OpenAIEndpointShape infers the protocol shape from the last segment of the endpoint path.
// Returns "responses" (Responses API) or "chat/completions" (Chat Completions API).
// This way /v1/chat/completions, /v4/chat/completions, and /chat/completions all take the same protocol branch.
func OpenAIEndpointShape(endpoint string) string {
	lower := strings.ToLower(strings.TrimSpace(endpoint))
	switch {
	case strings.HasSuffix(lower, "/responses"):
		return "responses"
	default:
		return "chat/completions"
	}
}

func BuildLegacyChannelID(baseURL string, modelID string, apiKey string, name string) string {
	return buildChannelID([]string{
		strings.TrimSpace(baseURL),
		strings.TrimSpace(modelID),
		strings.TrimSpace(apiKey),
		strings.TrimSpace(name),
	})
}

func BuildChannelID(baseURL string, modelID string, apiKey string, name string, openAIEndpoint string) string {
	endpoint := strings.TrimSpace(openAIEndpoint)
	if endpoint == "" {
		return BuildLegacyChannelID(baseURL, modelID, apiKey, name)
	}
	return buildChannelID([]string{
		strings.TrimSpace(baseURL),
		strings.TrimSpace(modelID),
		strings.TrimSpace(apiKey),
		strings.TrimSpace(name),
		endpoint,
	})
}

func buildChannelID(parts []string) string {
	payload := strings.Join(parts, "\n")
	hashBytes := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(hashBytes[:])[:ChannelIDHexLength]
}
