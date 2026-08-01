// inbound.go implements decoding, summarization, and command type identification of the upstream protocol.
package protocol

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"cursor/gen/agentv1"
	"cursor/gen/aiserverv1"
	"cursor/internal/backend/agent/core"

	"google.golang.org/protobuf/proto"
)

// ReadAppendRequestID reads the request_id text from a BidiAppendRequest.
func ReadAppendRequestID(input *aiserverv1.BidiAppendRequest) string {
	if input == nil {
		return ""
	}
	return ReadBidiRequestID(input.GetRequestId())
}

// ReadBidiRequestID extracts the request id from a BidiRequestId structure and trims leading/trailing whitespace.
func ReadBidiRequestID(input *aiserverv1.BidiRequestId) string {
	if input == nil {
		return ""
	}
	return strings.TrimSpace(input.GetRequestId())
}

// NormalizeRequestID normalizes the request id and trims leading/trailing whitespace.
func NormalizeRequestID(requestID string) string {
	return strings.TrimSpace(requestID)
}

// DecodeAgentClientMessage parses hex text into an AgentClientMessage and returns a message type label.
func DecodeAgentClientMessage(hexData string) (*agentv1.AgentClientMessage, string, error) {
	trimmed := strings.TrimSpace(hexData)
	if trimmed == "" {
		return nil, "", nil
	}
	payload, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, "", fmt.Errorf("bidi append data is not valid hex: %w", err)
	}
	clientMessage := &agentv1.AgentClientMessage{}
	if err := proto.Unmarshal(payload, clientMessage); err != nil {
		return nil, "", fmt.Errorf("decode agent client message failed: %w", err)
	}
	return clientMessage, detectClientMessageKind(clientMessage), nil
}

// MapClientMessageToCommandKind maps upstream protocol messages to runtime command kinds.
func MapClientMessageToCommandKind(message *agentv1.AgentClientMessage, clientKind string) (runtimecore.CommandKind, error) {
	switch strings.TrimSpace(clientKind) {
	case "run_request":
		return runtimecore.CommandKindRunRequested, nil
	case "prewarm_request":
		return runtimecore.CommandKindPrewarmRequested, nil
	case "conversation_action":
		if message == nil || message.GetConversationAction() == nil {
			return "", fmt.Errorf("conversation_action payload is required")
		}
		switch message.GetConversationAction().GetAction().(type) {
		case *agentv1.ConversationAction_CancelAction:
			return runtimecore.CommandKindCancelRequested, nil
		case *agentv1.ConversationAction_UserMessageAction,
			*agentv1.ConversationAction_ResumeAction,
			*agentv1.ConversationAction_SummarizeAction,
			*agentv1.ConversationAction_ShellCommandAction,
			*agentv1.ConversationAction_StartPlanAction,
			*agentv1.ConversationAction_ExecutePlanAction,
			*agentv1.ConversationAction_AsyncAskQuestionCompletionAction,
			*agentv1.ConversationAction_CancelSubagentAction,
			*agentv1.ConversationAction_BackgroundShellAction,
			*agentv1.ConversationAction_BackgroundTaskCompletionAction:
			return runtimecore.CommandKindConversationActionRecordOnly, nil
		default:
			return "", fmt.Errorf("unsupported conversation_action payload")
		}
	case "exec_client_message":
		return runtimecore.CommandKindExecClientMessage, nil
	case "interaction_response":
		return runtimecore.CommandKindInteractionResponse, nil
	case "exec_client_control_message":
		return runtimecore.CommandKindExecClientControlMessage, nil
	case "client_heartbeat":
		return runtimecore.CommandKindClientHeartbeat, nil
	case "kv_client_message":
		return runtimecore.CommandKindKVClientMessage, nil
	default:
		return "", fmt.Errorf("unsupported client message kind: %s", clientKind)
	}
}

// IsResumeRunRequest reports whether the current message is a `run_request` carrying a `resume_action`.
func IsResumeRunRequest(message *agentv1.AgentClientMessage) bool {
	if message == nil || message.GetRunRequest() == nil {
		return false
	}
	action := message.GetRunRequest().GetAction()
	if action == nil {
		return false
	}
	_, ok := action.GetAction().(*agentv1.ConversationAction_ResumeAction)
	return ok
}

// BuildClientHistoryEntry concatenates the message type and payload summary into conversation history record text.
func BuildClientHistoryEntry(kind string, message *agentv1.AgentClientMessage) string {
	normalizedKind := strings.TrimSpace(kind)
	if normalizedKind == "" {
		normalizedKind = "unknown"
	}

	payload := extractClientMessagePayload(message)
	summary := summarizePayload(payload)
	if summary == "" {
		if normalizedKind == "unknown" {
			return ""
		}
		return normalizedKind
	}
	return fmt.Sprintf("%s:%s", normalizedKind, summary)
}

// detectClientMessageKind reports which message branch kind the oneof message currently carries.
func detectClientMessageKind(message *agentv1.AgentClientMessage) string {
	if message == nil || message.GetMessage() == nil {
		return ""
	}
	switch message.GetMessage().(type) {
	case *agentv1.AgentClientMessage_RunRequest:
		return "run_request"
	case *agentv1.AgentClientMessage_PrewarmRequest:
		return "prewarm_request"
	case *agentv1.AgentClientMessage_ConversationAction:
		return "conversation_action"
	case *agentv1.AgentClientMessage_ExecClientMessage:
		return "exec_client_message"
	case *agentv1.AgentClientMessage_InteractionResponse:
		return "interaction_response"
	case *agentv1.AgentClientMessage_ExecClientControlMessage:
		return "exec_client_control_message"
	case *agentv1.AgentClientMessage_ClientHeartbeat:
		return "client_heartbeat"
	case *agentv1.AgentClientMessage_KvClientMessage:
		return "kv_client_message"
	default:
		return ""
	}
}

// extractClientMessagePayload extracts the raw bytes payload from a oneof branch.
func extractClientMessagePayload(message *agentv1.AgentClientMessage) []byte {
	if message == nil || message.GetMessage() == nil {
		return nil
	}

	switch item := message.GetMessage().(type) {
	case *agentv1.AgentClientMessage_RunRequest:
		return marshalProtoMessage(item.RunRequest)
	case *agentv1.AgentClientMessage_PrewarmRequest:
		return marshalProtoMessage(item.PrewarmRequest)
	case *agentv1.AgentClientMessage_ConversationAction:
		return marshalProtoMessage(item.ConversationAction)
	case *agentv1.AgentClientMessage_ExecClientMessage:
		return marshalProtoMessage(item.ExecClientMessage)
	case *agentv1.AgentClientMessage_InteractionResponse:
		return marshalProtoMessage(item.InteractionResponse)
	case *agentv1.AgentClientMessage_ExecClientControlMessage:
		return marshalProtoMessage(item.ExecClientControlMessage)
	case *agentv1.AgentClientMessage_ClientHeartbeat:
		return marshalProtoMessage(item.ClientHeartbeat)
	case *agentv1.AgentClientMessage_KvClientMessage:
		return marshalProtoMessage(item.KvClientMessage)
	default:
		return nil
	}
}

// marshalProtoMessage re-encodes a proto message into bytes for debug summary display.
func marshalProtoMessage(message proto.Message) []byte {
	if message == nil {
		return nil
	}
	payload, err := proto.Marshal(message)
	if err != nil {
		return nil
	}
	return payload
}

// summarizePayload generates a readable summary: text is preferred, falling back to a hex snippet when it cannot be read directly.
func summarizePayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}

	if utf8.Valid(payload) {
		text := strings.TrimSpace(string(payload))
		text = strings.ReplaceAll(text, "\n", " ")
		text = strings.ReplaceAll(text, "\r", " ")
		text = strings.TrimSpace(text)
		if text != "" {
			return truncateText(text, 120)
		}
	}
	return "hex:" + truncateText(hex.EncodeToString(payload), 120)
}

// truncateText truncates text by rune count to avoid cutting in the middle of a multi-byte character and producing garbled output.
func truncateText(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
