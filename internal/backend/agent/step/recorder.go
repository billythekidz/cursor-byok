// recorder.go implements parsing and construction of pending assistant output records.
package step

import (
	"encoding/json"
	"strings"

	"cursor/internal/backend/agent/core"
)

const (
	// defaultTextPreviewLimit is the maximum number of runes allowed in a text summary preview.
	defaultTextPreviewLimit = 120
)

// assistantMessage represents the common assistant message structure found in `pending_tool_calls`.
type assistantMessage struct {
	// ID is the message-level id; in current captures the common value is 1.
	ID string `json:"id,omitempty"`
	// Role is the role of the current message; the common current value is assistant.
	Role string `json:"role,omitempty"`
	// Content holds the content block list of the message.
	Content []assistantContent `json:"content,omitempty"`
}

// assistantContent represents a single content block within an assistant message.
type assistantContent struct {
	// Type is the content block type, e.g. text, reasoning, or tool-call.
	Type string `json:"type,omitempty"`
	// Text holds the text of a text content block.
	Text string `json:"text,omitempty"`
	// ToolCallID holds the tool call identifier.
	ToolCallID string `json:"toolCallId,omitempty"`
	// ToolName holds the tool name.
	ToolName string `json:"toolName,omitempty"`
	// Args holds the raw tool call arguments.
	Args json.RawMessage `json:"args,omitempty"`
	// Result holds the raw tool call result.
	Result json.RawMessage `json:"result,omitempty"`
}

// Recorder is responsible for parsing and constructing pending assistant output records.
type Recorder struct {
}

// StepRecorder defines the step recording interface the runtime depends on.
type StepRecorder interface {
	// ParsePendingAssistantOutputs parses a set of raw pending assistant output records.
	ParsePendingAssistantOutputs(rawValues []string) []runtimecore.PendingAssistantOutput
	// ParsePendingAssistantOutput parses a single raw assistant output record.
	ParsePendingAssistantOutput(raw string) runtimecore.PendingAssistantOutput
	// BuildTextAssistantOutput constructs an assistant output record containing only text.
	BuildTextAssistantOutput(text string) (string, runtimecore.PendingAssistantOutput, error)
	// StartAssistantOutput creates a new assistant output builder.
	StartAssistantOutput() *AssistantOutputBuilder
}

// NewRecorder creates an assistant output record organizer.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// AssistantOutputBuilder is a builder for one assistant output record.
type AssistantOutputBuilder struct {
	// message holds the raw assistant message currently being constructed.
	message assistantMessage
}

// ParsePendingAssistantOutputs parses a set of raw `pending_tool_calls` strings.
func (recorder *Recorder) ParsePendingAssistantOutputs(rawValues []string) []runtimecore.PendingAssistantOutput {
	if len(rawValues) == 0 {
		return nil
	}

	outputs := make([]runtimecore.PendingAssistantOutput, 0, len(rawValues))
	for _, raw := range rawValues {
		outputs = append(outputs, recorder.ParsePendingAssistantOutput(raw))
	}
	return outputs
}

// ParsePendingAssistantOutput parses a single raw assistant output record.
func (recorder *Recorder) ParsePendingAssistantOutput(raw string) runtimecore.PendingAssistantOutput {
	output := runtimecore.PendingAssistantOutput{
		RawMessage: strings.TrimSpace(raw),
	}
	if output.RawMessage == "" {
		return output
	}

	var message assistantMessage
	if err := json.Unmarshal([]byte(output.RawMessage), &message); err != nil {
		output.TextPreview = truncateText(output.RawMessage, defaultTextPreviewLimit)
		return output
	}

	output.Role = strings.TrimSpace(message.Role)
	output.ContentKinds = make([]string, 0, len(message.Content))
	output.ToolCallIDs = make([]string, 0, len(message.Content))
	output.ToolNames = make([]string, 0, len(message.Content))

	textParts := make([]string, 0, len(message.Content))
	for _, part := range message.Content {
		kind := strings.TrimSpace(part.Type)
		if kind == "" {
			kind = "unknown"
		}
		output.ContentKinds = append(output.ContentKinds, kind)

		if trimmedToolCallID := strings.TrimSpace(part.ToolCallID); trimmedToolCallID != "" {
			output.ToolCallIDs = append(output.ToolCallIDs, trimmedToolCallID)
		}
		if trimmedToolName := strings.TrimSpace(part.ToolName); trimmedToolName != "" {
			output.ToolNames = append(output.ToolNames, trimmedToolName)
		}
		if trimmedText := strings.TrimSpace(part.Text); trimmedText != "" {
			textParts = append(textParts, trimmedText)
		}
	}

	output.TextPreview = truncateText(strings.Join(textParts, "\n"), defaultTextPreviewLimit)
	return output
}

// BuildTextAssistantOutput constructs an assistant output record containing only text.
func (recorder *Recorder) BuildTextAssistantOutput(text string) (string, runtimecore.PendingAssistantOutput, error) {
	message := assistantMessage{
		ID:   "1",
		Role: "assistant",
		Content: []assistantContent{
			{
				Type: "text",
				Text: text,
			},
		},
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return "", runtimecore.PendingAssistantOutput{}, err
	}

	raw := string(payload)
	return raw, recorder.ParsePendingAssistantOutput(raw), nil
}

// StartAssistantOutput creates a new assistant output builder.
func (recorder *Recorder) StartAssistantOutput() *AssistantOutputBuilder {
	return &AssistantOutputBuilder{
		message: assistantMessage{
			ID:      "1",
			Role:    "assistant",
			Content: make([]assistantContent, 0, 4),
		},
	}
}

// AppendTextDelta appends a text segment.
func (builder *AssistantOutputBuilder) AppendTextDelta(text string) {
	if builder == nil {
		return
	}
	builder.message.Content = append(builder.message.Content, assistantContent{
		Type: "text",
		Text: text,
	})
}

// AppendReasoningDelta appends a reasoning segment, for replay by reasoning models when resuming.
func (builder *AssistantOutputBuilder) AppendReasoningDelta(text string) {
	if builder == nil {
		return
	}
	builder.message.Content = append(builder.message.Content, assistantContent{
		Type: "reasoning",
		Text: text,
	})
}

// OpenToolCall appends a not-yet-completed tool call block.
func (builder *AssistantOutputBuilder) OpenToolCall(toolCall runtimecore.ToolInvocation) {
	if builder == nil {
		return
	}
	builder.message.Content = append(builder.message.Content, assistantContent{
		Type:       "tool-call",
		ToolCallID: strings.TrimSpace(toolCall.CallID),
		ToolName:   strings.TrimSpace(toolCall.ToolName),
		Args:       append(json.RawMessage(nil), toolCall.ArgsJSON...),
	})
}

// CompleteToolCall fills in the result content for the specified tool call.
func (builder *AssistantOutputBuilder) CompleteToolCall(toolCallID string, resultJSON []byte) {
	if builder == nil {
		return
	}
	for index := range builder.message.Content {
		if builder.message.Content[index].Type != "tool-call" {
			continue
		}
		if strings.TrimSpace(builder.message.Content[index].ToolCallID) != strings.TrimSpace(toolCallID) {
			continue
		}
		builder.message.Content[index].Result = append(json.RawMessage(nil), resultJSON...)
		return
	}
}

// SnapshotRaw outputs the current builder's raw JSON and parsed result.
func (builder *AssistantOutputBuilder) SnapshotRaw(recorder *Recorder) (string, runtimecore.PendingAssistantOutput, error) {
	if builder == nil {
		return "", runtimecore.PendingAssistantOutput{}, nil
	}
	payload, err := json.Marshal(builder.message)
	if err != nil {
		return "", runtimecore.PendingAssistantOutput{}, err
	}
	raw := string(payload)
	if recorder == nil {
		recorder = NewRecorder()
	}
	return raw, recorder.ParsePendingAssistantOutput(raw), nil
}

// truncateText truncates text by rune count to avoid cutting in the middle of a multi-byte character.
func truncateText(text string, maxRunes int) string {
	trimmed := strings.TrimSpace(text)
	if maxRunes <= 0 || trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	return string(runes[:maxRunes]) + "..."
}
