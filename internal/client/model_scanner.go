package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cursor/internal/modelchannel"
	"cursor/internal/netproxy"
)

const (
	// openAIModelsScanTimeout limits the total duration of a single model list scan.
	openAIModelsScanTimeout = 30 * time.Second
	// openAIModelsScanMaxErrorBodyBytes limits the error response body read size, preventing large responses from being stuffed into the error message.
	openAIModelsScanMaxErrorBodyBytes = 8192
)

// OpenAIModelInfo represents a single model info returned by one OpenAI /v1/models scan.
type OpenAIModelInfo struct {
	ID string `json:"modelID"`
}

// openAIModelsResponse parses the standard OpenAI model list response with compatibility:
// {"object":"list","data":[{"id":"gpt-4o", ...}, ...]}
type openAIModelsResponse struct {
	Object string             `json:"object"`
	Data   []openAIModelEntry `json:"data"`
}

type openAIModelEntry struct {
	ID string `json:"id"`
}

// openAIModelsURL builds the /v1/models address from the user-provided baseURL.
// Handles both cases: if baseURL ends with /v1 (e.g. https://api.openai.com/v1), only /models is appended;
// otherwise /v1/models is appended after trimming the trailing slash.
func openAIModelsURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(normalized), "/v1") {
		return normalized + "/models"
	}
	return normalized + "/v1/models"
}

// ScanOpenAIModels calls the OpenAI standard GET {baseURL}/v1/models to scan the model list.
// Pure HTTP call, does not depend on Wails events.
func (s *ProxyService) ScanOpenAIModels(baseURL string, apiKey string) ([]OpenAIModelInfo, error) {
	normalizedBaseURL, err := modelchannel.NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("model adapter apiKey cannot be empty")
	}

	targetURL := openAIModelsURL(normalizedBaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), openAIModelsScanTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造模型列表请求失败: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	request.Header.Set("Accept", "application/json")

	httpClient := s.publicClient
	if httpClient == nil {
		httpClient = netproxy.NewHTTPClient(openAIModelsScanTimeout)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		limitedBody, readErr := io.ReadAll(io.LimitReader(response.Body, openAIModelsScanMaxErrorBodyBytes))
		bodyText := strings.TrimSpace(string(limitedBody))
		if readErr != nil {
			return nil, fmt.Errorf("扫描模型列表失败 status=%d body_read_error=%v", response.StatusCode, readErr)
		}
		if bodyText == "" {
			return nil, fmt.Errorf("扫描模型列表失败 status=%d", response.StatusCode)
		}
		return nil, fmt.Errorf("扫描模型列表失败 status=%d body=%s", response.StatusCode, bodyText)
	}

	var payload openAIModelsResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 16*1024*1024)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析模型列表响应失败: %w", err)
	}

	models := make([]OpenAIModelInfo, 0, len(payload.Data))
	for _, entry := range payload.Data {
		modelID := strings.TrimSpace(entry.ID)
		if modelID == "" {
			continue
		}
		models = append(models, OpenAIModelInfo{ID: modelID})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("扫描模型列表返回为空，请检查 baseURL 与 apiKey 是否正确")
	}
	return models, nil
}
