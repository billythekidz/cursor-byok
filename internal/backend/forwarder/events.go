// events.go builds legacy RunSSE messages that remain compatible with the client.
package forwarder

import (
	"encoding/json"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"cursor/gen/agentv1"
	execbridge "cursor/internal/backend/agent/bridge/exec"
	runtimecore "cursor/internal/backend/agent/core"
)

// buildHeartbeatMessage builds a server heartbeat message.
func buildHeartbeatMessage() *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_Heartbeat{
					Heartbeat: &agentv1.HeartbeatUpdate{},
				},
			},
		},
	}
}

// buildTextDeltaMessage builds a text delta message.
func buildTextDeltaMessage(text string) *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_TextDelta{
					TextDelta: &agentv1.TextDeltaUpdate{Text: text},
				},
			},
		},
	}
}

// buildThinkingDeltaMessage builds a thinking text delta message.
func buildThinkingDeltaMessage(text string, style agentv1.ThinkingStyle) *agentv1.AgentServerMessage {
	styleCopy := style
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_ThinkingDelta{
					ThinkingDelta: &agentv1.ThinkingDeltaUpdate{
						Text:          text,
						ThinkingStyle: &styleCopy,
					},
				},
			},
		},
	}
}

// buildThinkingCompletedMessage builds a thinking-phase-completed message.
func buildThinkingCompletedMessage(durationMS int32) *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_ThinkingCompleted{
					ThinkingCompleted: &agentv1.ThinkingCompletedUpdate{
						ThinkingDurationMs: durationMS,
					},
				},
			},
		},
	}
}

// buildSummaryStartedMessage builds a summary-started message.
func buildSummaryStartedMessage() *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_SummaryStarted{
					SummaryStarted: &agentv1.SummaryStartedUpdate{},
				},
			},
		},
	}
}

// buildSummaryMessage builds a summary delta/completion content message.
func buildSummaryMessage(summary string) *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_Summary{
					Summary: &agentv1.SummaryUpdate{Summary: strings.TrimSpace(summary)},
				},
			},
		},
	}
}

// buildSummaryCompletedMessage builds a summary-completed message.
// field 11 is decoded by the client as GetThoughtAnnotationRequest in the legacy aiserver protocol,
// so request_id must be written into field 1 here to drive the client into querying the "Chat context summarized" annotation.
func buildSummaryCompletedMessage(requestID string) *agentv1.AgentServerMessage {
	var message *string
	if strings.TrimSpace(requestID) != "" {
		value := strings.TrimSpace(requestID)
		message = &value
	}
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_SummaryCompleted{
					SummaryCompleted: &agentv1.SummaryCompletedUpdate{
						HookMessage: message,
					},
				},
			},
		},
	}
}

// buildToolCallStartedMessage builds a tool call started message.
func buildToolCallStartedMessage(callID string, modelCallID string, toolCall *agentv1.ToolCall) *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_ToolCallStarted{
					ToolCallStarted: &agentv1.ToolCallStartedUpdate{
						CallId:      callID,
						ToolCall:    toolCall,
						ModelCallId: modelCallID,
					},
				},
			},
		},
	}
}

// buildPartialToolCallMessage builds a compatibility message emitted while tool call arguments are still being streamed.
func buildPartialToolCallMessage(callID string, modelCallID string, toolCall *agentv1.ToolCall, argsTextDelta string) *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_PartialToolCall{
					PartialToolCall: &agentv1.PartialToolCallUpdate{
						CallId:        callID,
						ToolCall:      toolCall,
						ArgsTextDelta: argsTextDelta,
						ModelCallId:   modelCallID,
					},
				},
			},
		},
	}
}

// buildToolCallDeltaMessage builds a streaming delta message for tool calls.
func buildToolCallDeltaMessage(callID string, modelCallID string, delta *agentv1.ToolCallDelta) *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_ToolCallDelta{
					ToolCallDelta: &agentv1.ToolCallDeltaUpdate{
						CallId:        callID,
						ToolCallDelta: delta,
						ModelCallId:   modelCallID,
					},
				},
			},
		},
	}
}

// buildToolCallCompletedMessage builds a tool call completed message.
func buildToolCallCompletedMessage(callID string, modelCallID string, toolCall *agentv1.ToolCall) *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_ToolCallCompleted{
					ToolCallCompleted: &agentv1.ToolCallCompletedUpdate{
						CallId:      callID,
						ToolCall:    toolCall,
						ModelCallId: modelCallID,
					},
				},
			},
		},
	}
}

// buildShellOutputDeltaMessage wraps shell stream output into a compatibility message.
func buildShellOutputDeltaMessage(delta *agentv1.ShellOutputDeltaUpdate) *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_ShellOutputDelta{
					ShellOutputDelta: delta,
				},
			},
		},
	}
}

// buildTurnEndedMessage builds a turn-ended message carrying normalized token statistics.
func buildTurnEndedMessage(inputTokens int64, outputTokens int64, cacheReadTokens int64, cacheWriteTokens int64) *agentv1.AgentServerMessage {
	inputTokensValue := inputTokens
	outputTokensValue := outputTokens
	cacheReadTokensValue := cacheReadTokens
	cacheWriteTokensValue := cacheWriteTokens
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionUpdate{
			InteractionUpdate: &agentv1.InteractionUpdate{
				Message: &agentv1.InteractionUpdate_TurnEnded{
					TurnEnded: &agentv1.TurnEndedUpdate{
						InputTokens:      &inputTokensValue,
						OutputTokens:     &outputTokensValue,
						CacheReadTokens:  &cacheReadTokensValue,
						CacheWriteTokens: &cacheWriteTokensValue,
					},
				},
			},
		},
	}
}

// buildCheckpointMessage generates a legacy checkpoint message from the projected state.
func buildCheckpointMessage(state *agentv1.ConversationStateStructure) *agentv1.AgentServerMessage {
	cloned := &agentv1.ConversationStateStructure{}
	if state != nil {
		if next, ok := proto.Clone(state).(*agentv1.ConversationStateStructure); ok && next != nil {
			cloned = next
		}
	}
	if cloned.TokenDetails == nil {
		cloned.TokenDetails = &agentv1.ConversationTokenDetails{}
	}
	if cloned.TokenDetails.MaxTokens == 0 {
		cloned.TokenDetails.MaxTokens = projectedConversationMaxTokens
	}
	cloned.TurnTimings = nil
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_ConversationCheckpointUpdate{
			ConversationCheckpointUpdate: cloned,
		},
	}
}

// buildExecAbortMessage builds an abort control message for the client execution bridge.
func buildExecAbortMessage(pending runtimecore.PendingExec) *agentv1.AgentServerMessage {
	return &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_ExecServerControlMessage{
			ExecServerControlMessage: &agentv1.ExecServerControlMessage{
				Message: &agentv1.ExecServerControlMessage_Abort{
					Abort: &agentv1.ExecServerAbort{
						Id: pending.MessageID,
					},
				},
			},
		},
	}
}

// buildStartedToolCall maps a tool intent into a started ToolCall struct that can be sent to the client.
func buildStartedToolCall(invocation runtimecore.ToolInvocation) *agentv1.ToolCall {
	switch strings.TrimSpace(invocation.ToolName) {
	case "Glob":
		args, err := execbridge.DecodeGlobToolArgs(invocation.ArgsJSON)
		if err != nil {
			args = &agentv1.GlobToolArgs{}
		}
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_GlobToolCall{
				GlobToolCall: &agentv1.GlobToolCall{Args: args},
			},
		}
	case "Grep":
		args, err := execbridge.DecodeGrepToolArgs(invocation.ArgsJSON, invocation.CallID)
		if err != nil && args == nil {
			args = &agentv1.GrepArgs{ToolCallId: invocation.CallID}
		}
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_GrepToolCall{
				GrepToolCall: &agentv1.GrepToolCall{Args: args},
			},
		}
	case "Read":
		args, err := execbridge.DecodeReadToolArgs(invocation.ArgsJSON)
		if err != nil && args == nil {
			args = &agentv1.ReadToolArgs{}
		}
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_ReadToolCall{
				ReadToolCall: &agentv1.ReadToolCall{
					Args: args,
				},
			},
		}
	case "TodoWrite":
		args, _ := decodeUpdateTodosArgsJSON(invocation.ArgsJSON)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_UpdateTodosToolCall{
				UpdateTodosToolCall: &agentv1.UpdateTodosToolCall{Args: args},
			},
		}
	case "ReadTodos":
		args, _ := decodeReadTodosArgsJSON(invocation.ArgsJSON)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_ReadTodosToolCall{
				ReadTodosToolCall: &agentv1.ReadTodosToolCall{Args: args},
			},
		}
	case "Delete":
		var input struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(invocation.ArgsJSON, &input)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_DeleteToolCall{
				DeleteToolCall: &agentv1.DeleteToolCall{
					Args: &agentv1.DeleteArgs{Path: strings.TrimSpace(input.Path), ToolCallId: invocation.CallID},
				},
			},
		}
	case "Shell":
		var input struct {
			Command          string `json:"command"`
			Description      string `json:"description,omitempty"`
			WorkingDirectory string `json:"working_directory,omitempty"`
			NotifyOnOutput   *struct {
				Pattern           string   `json:"pattern"`
				Reason            string   `json:"reason"`
				DebounceMS        *float64 `json:"debounce_ms,omitempty"`
				NotificationLimit *int32   `json:"notification_limit,omitempty"`
			} `json:"notify_on_output,omitempty"`
		}
		_ = json.Unmarshal(invocation.ArgsJSON, &input)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_ShellToolCall{
				ShellToolCall: &agentv1.ShellToolCall{
					Args: &agentv1.ShellArgs{
						Command:            strings.TrimSpace(input.Command),
						WorkingDirectory:   strings.TrimSpace(input.WorkingDirectory),
						ToolCallId:         invocation.CallID,
						Description:        stringPtr(strings.TrimSpace(input.Description)),
						OutputNotification: buildShellOutputNotificationConfig(input.NotifyOnOutput),
					},
				},
			},
		}
	case "AwaitShell":
		args, err := decodeAwaitShellArgs(invocation.ArgsJSON)
		if err != nil {
			args = awaitShellArgs{}
		}
		return buildAwaitShellToolCall(buildAwaitArgsFromAwaitShellArgs(args), nil)
	case "WriteShellStdin":
		var input struct {
			ShellID uint32 `json:"shell_id"`
			Chars   string `json:"chars"`
		}
		_ = json.Unmarshal(invocation.ArgsJSON, &input)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_WriteShellStdinToolCall{
				WriteShellStdinToolCall: &agentv1.WriteShellStdinToolCall{
					Args: &agentv1.WriteShellStdinArgs{
						ShellId: input.ShellID,
						Chars:   input.Chars,
					},
				},
			},
		}
	case "Task":
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_TaskToolCall{
				TaskToolCall: &agentv1.TaskToolCall{
					Args: buildTaskArgsFromJSON(invocation.ArgsJSON),
				},
			},
		}
	case "Ls":
		var input struct {
			Path   string   `json:"path"`
			Ignore []string `json:"ignore,omitempty"`
		}
		_ = json.Unmarshal(invocation.ArgsJSON, &input)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_LsToolCall{
				LsToolCall: &agentv1.LsToolCall{
					Args: &agentv1.LsArgs{
						Path:       strings.TrimSpace(input.Path),
						Ignore:     append([]string(nil), input.Ignore...),
						ToolCallId: invocation.CallID,
					},
				},
			},
		}
	case "Write":
		var input struct {
			Path     string `json:"path"`
			Contents string `json:"contents"`
		}
		_ = json.Unmarshal(invocation.ArgsJSON, &input)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_EditToolCall{
				EditToolCall: &agentv1.EditToolCall{
					Args: &agentv1.EditArgs{
						Path:          strings.TrimSpace(input.Path),
						StreamContent: literalStringPtr(input.Contents),
					},
				},
			},
		}
	case "PatchEdit":
		var input struct {
			FilePath string `json:"file_path"`
			Path     string `json:"path,omitempty"`
		}
		_ = json.Unmarshal(invocation.ArgsJSON, &input)
		path := strings.TrimSpace(input.FilePath)
		if path == "" {
			path = strings.TrimSpace(input.Path)
		}
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_EditToolCall{
				EditToolCall: &agentv1.EditToolCall{
					Args: &agentv1.EditArgs{
						Path: path,
					},
				},
			},
		}
	case "CallMcpTool":
		var input struct {
			Server    string         `json:"server"`
			ToolName  string         `json:"toolName"`
			Arguments map[string]any `json:"arguments,omitempty"`
		}
		_ = json.Unmarshal(invocation.ArgsJSON, &input)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_McpToolCall{
				McpToolCall: &agentv1.McpToolCall{
					Args: &agentv1.McpArgs{
						Name:               forwarderCanonicalMCPToolLookupName(input.Server, input.ToolName),
						Args:               forwarderStructValueMap(input.Arguments),
						ToolCallId:         invocation.CallID,
						ProviderIdentifier: strings.TrimSpace(input.Server),
						ToolName:           strings.TrimSpace(input.ToolName),
					},
				},
			},
		}
	case "ListMcpResources":
		var input struct {
			Server string `json:"server,omitempty"`
		}
		_ = json.Unmarshal(invocation.ArgsJSON, &input)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_ListMcpResourcesToolCall{
				ListMcpResourcesToolCall: &agentv1.ListMcpResourcesToolCall{
					Args: &agentv1.ListMcpResourcesExecArgs{
						Server: stringPtr(strings.TrimSpace(input.Server)),
					},
				},
			},
		}
	case "AskQuestion":
		var args agentv1.AskQuestionArgs
		_ = json.Unmarshal(invocation.ArgsJSON, &args)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_AskQuestionToolCall{
				AskQuestionToolCall: &agentv1.AskQuestionToolCall{Args: &args},
			},
		}
	case "WebSearch":
		var args agentv1.WebSearchArgs
		_ = json.Unmarshal(invocation.ArgsJSON, &args)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_WebSearchToolCall{
				WebSearchToolCall: &agentv1.WebSearchToolCall{Args: &args},
			},
		}
	case "WebFetch":
		var args agentv1.WebFetchArgs
		_ = json.Unmarshal(invocation.ArgsJSON, &args)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_WebFetchToolCall{
				WebFetchToolCall: &agentv1.WebFetchToolCall{Args: &args},
			},
		}
	case "CreatePlan":
		args, err := runtimecore.DecodeCreatePlanArgsJSON(invocation.ArgsJSON)
		if err != nil {
			args = &agentv1.CreatePlanArgs{}
		}
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_CreatePlanToolCall{
				CreatePlanToolCall: &agentv1.CreatePlanToolCall{Args: args},
			},
		}
	case "GenerateImage":
		carrier, _ := decodeGenerateImageToolCarrier(invocation.ArgsJSON)
		args := buildGenerateImageArgsFromCarrier(carrier)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_GenerateImageToolCall{
				GenerateImageToolCall: &agentv1.GenerateImageToolCall{Args: args},
			},
		}
	case "SwitchMode":
		var args agentv1.SwitchModeArgs
		_ = json.Unmarshal(invocation.ArgsJSON, &args)
		args.ToolCallId = invocation.CallID
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_SwitchModeToolCall{
				SwitchModeToolCall: &agentv1.SwitchModeToolCall{Args: &args},
			},
		}
	case "FetchMcpResource":
		var input struct {
			Server       string `json:"server"`
			URI          string `json:"uri"`
			DownloadPath string `json:"downloadPath,omitempty"`
		}
		_ = json.Unmarshal(invocation.ArgsJSON, &input)
		return &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_ReadMcpResourceToolCall{
				ReadMcpResourceToolCall: &agentv1.ReadMcpResourceToolCall{
					Args: &agentv1.ReadMcpResourceExecArgs{
						Server:       strings.TrimSpace(input.Server),
						Uri:          strings.TrimSpace(input.URI),
						DownloadPath: stringPtr(strings.TrimSpace(input.DownloadPath)),
					},
				},
			},
		}
	default:
		return nil
	}
}

func forwarderCanonicalMCPToolLookupName(server string, toolName string) string {
	trimmedServer := strings.TrimSpace(server)
	trimmedToolName := strings.TrimSpace(toolName)
	if trimmedToolName == "" {
		return ""
	}
	if trimmedServer == "" {
		return trimmedToolName
	}
	return trimmedServer + "-" + trimmedToolName
}

func forwarderStructValueMap(items map[string]any) map[string]*structpb.Value {
	if len(items) == 0 {
		return map[string]*structpb.Value{}
	}
	result := make(map[string]*structpb.Value, len(items))
	for key, value := range items {
		item, err := structpb.NewValue(value)
		if err != nil {
			continue
		}
		result[key] = item
	}
	return result
}

// stringPtr returns a pointer when the string is non-empty, avoiding empty strings in optional fields.
func buildShellOutputNotificationConfig(input *struct {
	Pattern           string   `json:"pattern"`
	Reason            string   `json:"reason"`
	DebounceMS        *float64 `json:"debounce_ms,omitempty"`
	NotificationLimit *int32   `json:"notification_limit,omitempty"`
}) *agentv1.ShellOutputNotificationConfig {
	if input == nil {
		return nil
	}
	pattern := strings.TrimSpace(input.Pattern)
	reason := strings.TrimSpace(input.Reason)
	if pattern == "" || reason == "" {
		return nil
	}
	var debounce *float64
	if input.DebounceMS != nil {
		value := *input.DebounceMS / 1000
		if value < 5 {
			value = 5
		}
		debounce = &value
	}
	return &agentv1.ShellOutputNotificationConfig{
		Pattern:           pattern,
		Reason:            reason,
		Debounce:          debounce,
		NotificationLimit: input.NotificationLimit,
	}
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	next := strings.TrimSpace(value)
	return &next
}

func literalStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	next := value
	return &next
}
