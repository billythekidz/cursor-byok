package modeladapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"cursor/gen/agentv1"
	"cursor/internal/appdata"
	"cursor/internal/backend/codex"
	"cursor/internal/logger"
)

type CodexAdapter struct {
	runtime *codex.ClientManager
}

func NewCodexAdapter(runtime *codex.ClientManager) *CodexAdapter {
	if runtime == nil {
		runtime = codex.NewClientManager(appdata.HistoryRootPath())
	}
	return &CodexAdapter{runtime: runtime}
}

func (adapter *CodexAdapter) Stream(ctx context.Context, req StreamRequest, sink func(ModelEvent) error) error {
	if adapter == nil || adapter.runtime == nil {
		return errors.New("Codex runtime manager is unavailable")
	}
	workspace := strings.TrimSpace(req.WorkspacePath)
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	model := firstNonEmpty(req.ProviderModelID, req.ModelID)
	conversationID := firstNonEmpty(req.ConversationID, req.RequestID)
	rawEffort := firstNonEmpty(req.ReasoningEffort, req.ThinkingEffort)
	effort := normalizeCodexEffort(rawEffort)
	if strings.EqualFold(rawEffort, "max") {
		logger.Infof("Codex reasoning effort max is mapped to xhigh because the app-server schema does not expose max")
	}
	input := latestUserText(req.Messages)
	if input == "" {
		return errors.New("Codex request does not contain a user message")
	}
	if req.Observer != nil {
		_, _ = req.Observer.RecordLLMRequest(req.RequestID, req.RunID, req.ModelCallID, map[string]any{
			"provider":     "codex",
			"model":        model,
			"workspace":    workspace,
			"conversation": conversationID,
			"effort":       effort,
			"input_length": len(input),
		})
	}
	return adapter.runtime.RunTurn(ctx, codex.RunTurnParams{
		ConversationID:        conversationID,
		Workspace:             workspace,
		Model:                 model,
		Effort:                effort,
		Input:                 input,
		BaseInstructions:      "Use the selected workspace as the working directory. Keep the response concise when no code change is required.",
		DeveloperInstructions: "Cursor owns the chat UI. Codex owns shell and file tools for this turn.",
	}, func(notice codex.Notification) error {
		return mapCodexNotification(req, notice, sink)
	})
}

func mapCodexNotification(req StreamRequest, notice codex.Notification, sink func(ModelEvent) error) error {
	var payload struct {
		Delta   string `json:"delta"`
		Message string `json:"message"`
		Turn    struct {
			Status string `json:"status"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(notice.Params, &payload); err != nil {
		return fmt.Errorf("decode Codex notification %s: %w", notice.Method, err)
	}
	event := ModelEvent{
		OccurredAt: time.Now().UTC(),
		Provider:   "codex",
		Model:      firstNonEmpty(req.ProviderModelID, req.ModelID),
	}
	switch notice.Method {
	case "item/agentMessage/delta":
		event.Kind = ModelEventKindTextDelta
		event.Text = payload.Delta
	case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta", "item/plan/delta":
		event.Kind = ModelEventKindThinkingDelta
		event.ThinkingStyle = agentv1.ThinkingStyle_THINKING_STYLE_DEFAULT
		event.Text = payload.Delta
	case "turn/completed":
		if status := strings.ToLower(strings.TrimSpace(payload.Turn.Status)); status == "failed" || status == "error" || status == "interrupted" || status == "cancelled" {
			return fmt.Errorf("Codex turn ended with status %s", status)
		}
		event.Kind = ModelEventKindTurnFinished
		event.FinishReason = firstNonEmpty(payload.Turn.Status, "completed")
	case "turn/failed", "turn/aborted", "turn/cancelled":
		return fmt.Errorf("Codex turn failed: %s", firstNonEmpty(payload.Message, notice.Method))
	case "error":
		return fmt.Errorf("Codex app-server error: %s", strings.TrimSpace(payload.Message))
	default:
		if strings.Contains(notice.Method, "outputDelta") || strings.Contains(notice.Method, "patchUpdated") {
			logger.Infof("Codex tool progress method=%s bytes=%d", notice.Method, len(payload.Delta))
		}
		return nil
	}
	if event.Kind == ModelEventKindTextDelta && strings.TrimSpace(event.Text) == "" {
		return nil
	}
	return sink(event)
}

func latestUserText(messages []Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
			continue
		}
		text := strings.TrimSpace(message.Content)
		if text == "" {
			for _, part := range message.ContentParts {
				if strings.EqualFold(strings.TrimSpace(part.Type), "text") {
					text += part.Text
				}
			}
		}
		return strings.TrimSpace(text)
	}
	return ""
}

func normalizeCodexEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(value))
	case "max":
		return "xhigh"
	default:
		return "medium"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
