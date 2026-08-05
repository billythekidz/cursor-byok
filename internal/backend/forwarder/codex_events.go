package forwarder

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cursor/gen/agentv1"
	modeladapter "cursor/internal/backend/agent/model"
)

// applyCodexProviderEvent projects Codex-owned tool progress onto Cursor's
// existing shell and edit streams. Codex remains the owner of execution; these
// events are display-only and never enter the normal tool execution path.
func (service *Service) applyCodexProviderEvent(stream *ActiveStream, event modeladapter.ModelEvent) error {
	if service == nil || stream == nil {
		return nil
	}
	stream.mu.Lock()
	requestID := stream.RequestID
	modelCallID := stream.CurrentModelCallID
	stream.mu.Unlock()

	switch event.Kind {
	case modeladapter.ModelEventKindProviderShellOutputDelta:
		if event.ProviderShellOutputDelta == nil {
			return nil
		}
		stream.mu.Lock()
		stream.UpdatedAt = time.Now().UTC()
		stream.mu.Unlock()
		return service.broker.Publish(requestID, StreamEvent{
			Message: buildShellOutputDeltaMessage(event.ProviderShellOutputDelta),
		})
	case modeladapter.ModelEventKindProviderFileChangeDelta:
		if strings.TrimSpace(event.ProviderFileChangeDelta) == "" || strings.TrimSpace(event.ProviderItemID) == "" {
			return nil
		}
		stream.mu.Lock()
		stream.UpdatedAt = time.Now().UTC()
		stream.mu.Unlock()
		return service.broker.Publish(requestID, StreamEvent{
			Message: buildToolCallDeltaMessage(event.ProviderItemID, modelCallID, &agentv1.ToolCallDelta{
				Delta: &agentv1.ToolCallDelta_EditToolCallDelta{
					EditToolCallDelta: &agentv1.EditToolCallDelta{
						StreamContentDelta: event.ProviderFileChangeDelta,
					},
				},
			}),
		})
	case modeladapter.ModelEventKindProviderItemStarted:
		toolCall, ok := buildCodexToolCall(event, false)
		if !ok {
			return nil
		}
		stream.mu.Lock()
		stream.UpdatedAt = time.Now().UTC()
		stream.mu.Unlock()
		return service.broker.Publish(requestID, StreamEvent{
			Message: buildToolCallStartedMessage(event.ProviderItemID, modelCallID, toolCall),
		})
	case modeladapter.ModelEventKindProviderItemCompleted:
		toolCall, ok := buildCodexToolCall(event, true)
		if !ok {
			return nil
		}
		stream.mu.Lock()
		stream.UpdatedAt = time.Now().UTC()
		stream.mu.Unlock()
		return service.broker.Publish(requestID, StreamEvent{
			Message: buildToolCallCompletedMessage(event.ProviderItemID, modelCallID, toolCall),
		})
	default:
		return nil
	}
}

type codexItemSnapshot struct {
	Type             string            `json:"type"`
	ID               string            `json:"id"`
	Command          string            `json:"command"`
	CWD              string            `json:"cwd"`
	Status           string            `json:"status"`
	AggregatedOutput string            `json:"aggregatedOutput"`
	ExitCode         *int32            `json:"exitCode"`
	DurationMS       *int64            `json:"durationMs"`
	Changes          []codexFileChange `json:"changes"`
}

type codexFileChange struct {
	Path string          `json:"path"`
	Kind json.RawMessage `json:"kind"`
	Diff string          `json:"diff"`
}

func buildCodexToolCall(event modeladapter.ModelEvent, completed bool) (*agentv1.ToolCall, bool) {
	if strings.TrimSpace(event.ProviderItemID) == "" || len(event.ProviderRawItem) == 0 {
		return nil, false
	}
	var item codexItemSnapshot
	if err := json.Unmarshal(event.ProviderRawItem, &item); err != nil {
		return nil, false
	}
	itemID := firstNonEmpty(event.ProviderItemID, item.ID)
	switch firstNonEmpty(event.ProviderItemType, item.Type) {
	case "commandExecution":
		args := &agentv1.ShellArgs{
			Command:          item.Command,
			WorkingDirectory: item.CWD,
			ToolCallId:       itemID,
		}
		shellCall := &agentv1.ShellToolCall{Args: args}
		if description := strings.TrimSpace(item.Command); description != "" {
			shellCall.Description = &description
		}
		if completed {
			shellCall.Result = buildCodexShellResult(item)
		}
		return &agentv1.ToolCall{Tool: &agentv1.ToolCall_ShellToolCall{ShellToolCall: shellCall}}, true
	case "fileChange":
		path := ""
		if len(item.Changes) > 0 {
			path = item.Changes[0].Path
		}
		args := &agentv1.EditArgs{Path: path}
		if patch := formatCodexItemChanges(item.Changes); patch != "" {
			args.StreamContent = &patch
		}
		editCall := &agentv1.EditToolCall{Args: args}
		if completed {
			editCall.Result = buildCodexEditResult(item)
		}
		return &agentv1.ToolCall{Tool: &agentv1.ToolCall_EditToolCall{EditToolCall: editCall}}, true
	default:
		return nil, false
	}
}

func buildCodexShellResult(item codexItemSnapshot) *agentv1.ShellResult {
	exitCode := int32(0)
	if item.ExitCode != nil {
		exitCode = *item.ExitCode
	}
	executionTime := int32(0)
	if item.DurationMS != nil {
		executionTime = clampInt64ToInt32(*item.DurationMS)
	}
	if strings.EqualFold(strings.TrimSpace(item.Status), "completed") && exitCode == 0 {
		return &agentv1.ShellResult{
			Result: &agentv1.ShellResult_Success{Success: &agentv1.ShellSuccess{
				Command:          item.Command,
				WorkingDirectory: item.CWD,
				ExitCode:         exitCode,
				Stdout:           item.AggregatedOutput,
				ExecutionTime:    executionTime,
			}},
		}
	}
	return &agentv1.ShellResult{
		Result: &agentv1.ShellResult_Failure{Failure: &agentv1.ShellFailure{
			Command:          item.Command,
			WorkingDirectory: item.CWD,
			ExitCode:         exitCode,
			Stdout:           item.AggregatedOutput,
			ExecutionTime:    executionTime,
		}},
	}
}

func buildCodexEditResult(item codexItemSnapshot) *agentv1.EditResult {
	path := ""
	if len(item.Changes) > 0 {
		path = item.Changes[0].Path
	}
	if strings.EqualFold(strings.TrimSpace(item.Status), "completed") {
		diff := formatCodexItemChanges(item.Changes)
		message := "Codex file change completed"
		return &agentv1.EditResult{
			Result: &agentv1.EditResult_Success{Success: &agentv1.EditSuccess{
				Path:       path,
				DiffString: &diff,
				Message:    &message,
			}},
		}
	}
	return &agentv1.EditResult{
		Result: &agentv1.EditResult_Error{Error: &agentv1.EditError{
			Path:  path,
			Error: fmt.Sprintf("Codex file change ended with status %s", firstNonEmpty(item.Status, "unknown")),
		}},
	}
}

func formatCodexItemChanges(changes []codexFileChange) string {
	sections := make([]string, 0, len(changes))
	for _, change := range changes {
		kind := strings.TrimSpace(string(change.Kind))
		if kind == "" {
			kind = "unknown"
		}
		sections = append(sections, strings.TrimSpace(fmt.Sprintf("path: %s\nkind: %s\n%s", change.Path, kind, change.Diff)))
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}
