package bridge

import (
	"cursor/internal/appdata"
	"cursor/internal/historymetrics"
)

// HomeMetricsSummary defines the historical statistics summary shown on the home page.
type HomeMetricsSummary struct {
	ProviderCallsTotal int      `json:"providerCallsTotal"`
	TurnsTotal         int      `json:"turnsTotal"`
	ValidTurnsTotal    int      `json:"validTurnsTotal"`
	InvalidTurnsTotal  int      `json:"invalidTurnsTotal"`
	RequestTokensTotal int64    `json:"requestTokensTotal"`
	PromptTokensTotal  int64    `json:"promptTokensTotal"`
	CacheReadTokens    int64    `json:"cacheReadTokens"`
	CacheWriteTokens   int64    `json:"cacheWriteTokens"`
	CacheHitRate       *float64 `json:"cacheHitRate"`
}

// MetricsService defines the Wails service for home page statistics.
type MetricsService struct{}

// NewMetricsService creates the home page statistics service.
func NewMetricsService() *MetricsService {
	return &MetricsService{}
}

// GetHomeMetricsSummary returns the full historical statistics summary shown on the home page.
func (service *MetricsService) GetHomeMetricsSummary() (HomeMetricsSummary, error) {
	if err := appdata.EnsureAssistantHome(); err != nil {
		return HomeMetricsSummary{}, err
	}

	summary, err := historymetrics.LoadUsageSummary(appdata.UsageFilePath())
	if err != nil {
		return HomeMetricsSummary{}, err
	}
	return HomeMetricsSummary{
		ProviderCallsTotal: summary.ProviderCallsTotal,
		TurnsTotal:         summary.TurnsTotal,
		ValidTurnsTotal:    summary.ValidTurnsTotal,
		InvalidTurnsTotal:  summary.InvalidTurnsTotal,
		RequestTokensTotal: summary.RequestTokensTotal,
		PromptTokensTotal:  summary.PromptTokensTotal,
		CacheReadTokens:    summary.CacheReadTokens,
		CacheWriteTokens:   summary.CacheWriteTokens,
		CacheHitRate:       summary.CacheHitRate,
	}, nil
}
