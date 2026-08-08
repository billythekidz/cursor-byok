package forwarder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	maxProviderPasses        = 12
	maxNoProgressPasses      = 4
	providerProgressGuardEnv = "CURSOR_BYOK_PROVIDER_PROGRESS_GUARD"
)

type providerProgressKind string

const (
	providerProgressNone         providerProgressKind = "none"
	providerProgressObservation  providerProgressKind = "observation"
	providerProgressMutation     providerProgressKind = "mutation"
	providerProgressVerification providerProgressKind = "verification"
	providerProgressFailure      providerProgressKind = "failure"
	providerProgressDuplicate    providerProgressKind = "duplicate"
)

type toolPairingIssue struct {
	Reason string
	Detail string
}

func (issue *toolPairingIssue) Error() string {
	if issue == nil || strings.TrimSpace(issue.Detail) == "" {
		return "tool call/result pairing failed"
	}
	return issue.Detail
}

func (kind providerProgressKind) isMeaningful() bool {
	switch kind {
	case providerProgressObservation, providerProgressMutation, providerProgressVerification:
		return true
	default:
		return false
	}
}

func (kind providerProgressKind) rank() int {
	switch kind {
	case providerProgressVerification:
		return 3
	case providerProgressMutation:
		return 2
	case providerProgressObservation:
		return 1
	default:
		return 0
	}
}

func classifyToolProgress(toolName string, argsJSON []byte, resultText string) providerProgressKind {
	if strings.TrimSpace(resultText) == "" {
		return providerProgressNone
	}
	if strings.TrimSpace(toolName) == "WebSearch" {
		normalized := strings.ToLower(resultText)
		if strings.Contains(normalized, "\"failure_class\"") || strings.Contains(normalized, "status=error") {
			return providerProgressFailure
		}
		return providerProgressObservation
	}
	if toolResultIndicatesFailure(resultText) {
		return providerProgressFailure
	}
	switch strings.TrimSpace(toolName) {
	case "PatchEdit", "Write", "Delete", "GenerateImage", "Task", "WriteShellStdin":
		return providerProgressMutation
	case "ReadLints":
		return providerProgressVerification
	case "Shell":
		if isVerificationShellCommand(shellCommandFromArgs(argsJSON)) {
			return providerProgressVerification
		}
		return providerProgressObservation
	case "Read", "Grep", "Glob", "Ls", "WebFetch", "WebSearch", "FetchMcpResource", "CallMcpTool", "AwaitShell", "SwitchMode":
		return providerProgressObservation
	default:
		return providerProgressObservation
	}
}

func toolResultIndicatesFailure(resultText string) bool {
	normalized := strings.ToLower(strings.TrimSpace(resultText))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "no error") || strings.Contains(normalized, "without error") {
		return false
	}
	for _, marker := range []string{
		"<tool_error>",
		"tool error",
		"permission denied",
		"timed out",
		"timeout",
		" failed",
		"failure",
		" error:",
		"error: ",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	if strings.Contains(normalized, "exit code") && !strings.Contains(normalized, "exit code 0") {
		return true
	}
	return strings.HasPrefix(normalized, "error")
}

func shellCommandFromArgs(argsJSON []byte) string {
	if len(argsJSON) == 0 {
		return ""
	}
	var args map[string]any
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return ""
	}
	for _, key := range []string{"command", "cmd", "script"} {
		if value, ok := args[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isVerificationShellCommand(command string) bool {
	normalized := strings.ToLower(strings.TrimSpace(command))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		" test",
		"go test",
		"cargo test",
		"npm test",
		"pnpm test",
		"yarn test",
		"pytest",
		" build",
		"go build",
		"cargo check",
		"cargo clippy",
		" lint",
		" typecheck",
		"tsc",
		" verify",
	} {
		if strings.Contains(normalized, marker) || strings.HasPrefix(normalized, strings.TrimSpace(marker)) {
			return true
		}
	}
	return false
}

func toolProgressFingerprint(toolName string, argsJSON []byte, resultText string) string {
	hash := sha256.New()
	hash.Write([]byte(strings.TrimSpace(toolName)))
	hash.Write([]byte{0})
	hash.Write(argsJSON)
	hash.Write([]byte{0})
	hash.Write([]byte(strings.TrimSpace(resultText)))
	return hex.EncodeToString(hash.Sum(nil))
}

func (service *Service) recordToolResultProgress(stream *ActiveStream, toolName string, argsJSON []byte, resultText string) {
	if stream == nil {
		return
	}
	kind := classifyToolProgress(toolName, argsJSON, resultText)
	fingerprint := toolProgressFingerprint(toolName, argsJSON, resultText)
	stream.mu.Lock()
	if kind.isMeaningful() && fingerprint == stream.ProviderLastProgressFingerprint {
		kind = providerProgressDuplicate
	}
	if kind.isMeaningful() {
		stream.ProviderLastProgressFingerprint = fingerprint
	}
	if kind.rank() > stream.ProviderPassProgress.rank() {
		stream.ProviderPassProgress = kind
	} else if stream.ProviderPassProgress == providerProgressNone {
		stream.ProviderPassProgress = kind
	}
	stream.ProviderPassLastToolName = strings.TrimSpace(toolName)
	stream.ProviderPassLastToolFingerprint = fingerprint
	if strings.TrimSpace(toolName) == "WebSearch" {
		stream.ProviderLastWebSearchFailureClass = boundedWebSearchFailureClass(resultText)
	}
	stream.UpdatedAt = nowUTC()
	stream.mu.Unlock()
}

func streamAgentRole(stream *ActiveStream) agentRole {
	if stream == nil {
		return agentRoleMain
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return streamAgentRoleLocked(stream)
}

func streamAgentRoleLocked(stream *ActiveStream) agentRole {
	if stream == nil || stream.CheckpointConversation == nil {
		return agentRoleMain
	}
	return resolveAgentRole(stream.CheckpointConversation.SubagentTypeName)
}

func (service *Service) continueProviderAfterToolPass(stream *ActiveStream) error {
	if service == nil || stream == nil {
		return nil
	}
	if err := service.repairCurrentTurnToolResultPairing(stream); err != nil {
		var pairingIssue *toolPairingIssue
		if !errors.As(err, &pairingIssue) {
			return err
		}
		return service.terminateProviderLoop(stream, firstNonEmpty(pairingIssue.Reason, "missing_tool_result"), streamAgentRole(stream), providerProgressFailure, 0, currentProviderPass(stream), "")
	}
	stream.mu.Lock()
	requestID := stream.RequestID
	conversationID := stream.ConversationID
	turnSeq := stream.TurnSeq
	modelCallID := stream.CurrentModelCallID
	providerPass := stream.ProviderPassCount
	progress := stream.ProviderPassProgress
	lastToolName := stream.ProviderPassLastToolName
	role := agentRoleMain
	if stream.CheckpointConversation != nil {
		role = resolveAgentRole(stream.CheckpointConversation.SubagentTypeName)
	}
	if !isChildAgentRole(role) {
		stream.mu.Unlock()
		return service.requestProviderAction(stream, providerActionResume)
	}
	if !providerProgressGuardEnabled() {
		stream.mu.Unlock()
		return service.requestProviderAction(stream, providerActionResume)
	}
	noProgressPasses := stream.ProviderNoProgressPasses
	nudgeIssued := stream.ProviderRecoveryNudgeIssued
	webSearchFailureClass := stream.ProviderLastWebSearchFailureClass
	if progress.isMeaningful() {
		noProgressPasses = 0
	} else {
		noProgressPasses++
	}
	stream.ProviderNoProgressPasses = noProgressPasses
	stream.UpdatedAt = nowUTC()
	stream.mu.Unlock()

	service.debug.LogRuntime(context.Background(), requestID, conversationID, "provider_loop_progress", map[string]any{
		"agent_role":               string(role),
		"progress_guard_enabled":   true,
		"provider_pass":            providerPass,
		"progress":                 string(progress),
		"last_outcome":             string(progress),
		"last_tool":                lastToolName,
		"web_search_failure_class": webSearchFailureClass,
		"no_progress_passes":       noProgressPasses,
		"recovery_nudge_issued":    nudgeIssued,
	})

	nudge := false
	terminalReason := ""
	if progress.isMeaningful() {
		// A successful observation, mutation, or verification is enough to keep the
		// loop alive and clears the consecutive no-progress streak.
	} else if role == agentRoleWorker {
		if noProgressPasses >= maxNoProgressPasses {
			if nudgeIssued {
				terminalReason = "blocked_no_progress"
			} else {
				nudge = true
			}
		}
	} else if !nudgeIssued {
		nudge = true
	} else {
		terminalReason = "blocked_no_progress"
	}
	if providerPass >= maxProviderPasses {
		terminalReason = "max_provider_passes"
		nudge = false
	}
	if terminalReason != "" {
		return service.terminateProviderLoop(stream, terminalReason, role, progress, noProgressPasses, providerPass, lastToolName)
	}
	if nudge {
		stream.mu.Lock()
		stream.ProviderRecoveryNudgeIssued = true
		stream.UpdatedAt = nowUTC()
		stream.mu.Unlock()
		if err := service.appendProviderLoopRecovery(stream, role, requestID, turnSeq, modelCallID, progress, noProgressPasses); err != nil {
			return err
		}
	}
	return service.requestProviderAction(stream, providerActionResume)
}

func (service *Service) repairCurrentTurnToolResultPairing(stream *ActiveStream) error {
	if service == nil || stream == nil {
		return nil
	}
	conversation, pendingExecs, pendingInteractions, err := service.snapshotCheckpointConversation(stream)
	if err != nil {
		return err
	}
	if len(pendingExecs)+len(pendingInteractions) > 0 {
		return &toolPairingIssue{Reason: "missing_tool_result", Detail: "external tool results are still pending"}
	}
	turnSeq := conversation.CurrentTurnSeq
	if turnSeq <= 0 {
		turnSeq = stream.TurnSeq
	}
	calls := make(map[string]toolCallEntryPayload)
	callOrder := make([]string, 0)
	results := make(map[string]int)
	for _, entry := range conversation.Entries {
		if entry.TurnSeq != turnSeq {
			continue
		}
		switch strings.TrimSpace(entry.Kind) {
		case "tool_call":
			var payload toolCallEntryPayload
			if err := json.Unmarshal(entry.Payload, &payload); err != nil {
				return &toolPairingIssue{Reason: "missing_tool_result", Detail: fmt.Sprintf("decode tool call %q: %v", entry.ToolCallID, err)}
			}
			toolCallID := firstNonEmpty(strings.TrimSpace(payload.ToolCallID), strings.TrimSpace(entry.ToolCallID))
			if toolCallID == "" {
				return &toolPairingIssue{Reason: "missing_tool_result", Detail: "tool call has no call id"}
			}
			if _, exists := calls[toolCallID]; exists {
				return &toolPairingIssue{Reason: "missing_tool_result", Detail: fmt.Sprintf("duplicate tool call id %q", toolCallID)}
			}
			payload.ToolCallID = toolCallID
			calls[toolCallID] = payload
			callOrder = append(callOrder, toolCallID)
		case "tool_result":
			var payload toolResultEntryPayload
			if err := json.Unmarshal(entry.Payload, &payload); err != nil {
				return &toolPairingIssue{Reason: "missing_tool_result", Detail: fmt.Sprintf("decode tool result %q: %v", entry.ToolCallID, err)}
			}
			toolCallID := firstNonEmpty(strings.TrimSpace(payload.ToolCallID), strings.TrimSpace(entry.ToolCallID))
			if toolCallID == "" {
				return &toolPairingIssue{Reason: "missing_tool_result", Detail: "tool result has no call id"}
			}
			results[toolCallID]++
		}
	}
	missing := make([]toolCallEntryPayload, 0)
	for _, toolCallID := range callOrder {
		call := calls[toolCallID]
		switch results[toolCallID] {
		case 0:
			missing = append(missing, call)
		case 1:
		default:
			return &toolPairingIssue{Reason: "missing_tool_result", Detail: fmt.Sprintf("multiple tool results for call id %q", toolCallID)}
		}
	}
	for toolCallID := range results {
		if _, exists := calls[toolCallID]; !exists {
			return &toolPairingIssue{Reason: "missing_tool_result", Detail: fmt.Sprintf("orphan tool result for call id %q", toolCallID)}
		}
	}
	for _, call := range missing {
		if err := service.appendToolResult(stream, call.ToolCallID, call.ToolName, nil, "tool execution error: missing_tool_result", "", nil); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) appendProviderLoopRecovery(stream *ActiveStream, role agentRole, requestID string, turnSeq int64, modelCallID string, progress providerProgressKind, noProgressPasses int) error {
	if stream == nil {
		return nil
	}
	text := "The latest provider pass made no new progress."
	if role == agentRoleWorker {
		text += " Take one concrete next action now: use the appropriate mutation or verification tool instead of returning only a plan or promise. If the task is blocked, report the exact blocker to the parent."
	} else {
		text += " Stop repeating the same attempt and return a concise evidence-based report to the parent, including the blocker or missing information. Do not request or perform a mutation."
	}
	text = fmt.Sprintf("%s Progress=%s; consecutive_no_progress_passes=%d.", text, progress, noProgressPasses)
	conversation, _, _, err := service.snapshotCheckpointConversation(stream)
	if err != nil {
		return err
	}
	if currentTurnHasPromptContextSource(conversation, turnSeq, promptContextSourceProviderLoopRecovery) {
		return nil
	}
	if _, err := service.appendConversationEntries(stream, stream.ConversationID, []HistoryEntry{
		newPromptContextEntry(turnSeq, requestID, newPromptContextReminder(promptContextSourceProviderLoopRecovery, text)),
	}); err != nil {
		return err
	}
	service.debug.LogRuntime(context.Background(), requestID, stream.ConversationID, "provider_loop_recovery_nudge", map[string]any{
		"agent_role":             string(role),
		"progress_guard_enabled": true,
		"provider_pass":          currentProviderPass(stream),
		"progress":               string(progress),
		"last_outcome":           string(progress),
		"no_progress_passes":     noProgressPasses,
		"model_call_id":          strings.TrimSpace(modelCallID),
	})
	return service.publishCheckpoint(requestID, stream.ConversationID)
}

func (service *Service) terminateProviderLoop(stream *ActiveStream, reason string, role agentRole, progress providerProgressKind, noProgressPasses int, providerPass int, lastToolName string) error {
	if stream == nil {
		return nil
	}
	stream.mu.Lock()
	stream.ProviderLoopTerminalReason = strings.TrimSpace(reason)
	requestID := stream.RequestID
	conversationID := stream.ConversationID
	turnSeq := stream.TurnSeq
	webSearchFailureClass := stream.ProviderLastWebSearchFailureClass
	stream.mu.Unlock()
	_, _ = service.appendConversationEntries(stream, conversationID, []HistoryEntry{
		newMetadataEntry(turnSeq, requestID, "provider_loop_terminal", map[string]any{
			"reason":                   strings.TrimSpace(reason),
			"agent_role":               string(role),
			"progress_guard_enabled":   providerProgressGuardEnabled(),
			"provider_pass":            providerPass,
			"progress":                 string(progress),
			"last_outcome":             string(progress),
			"last_tool":                lastToolName,
			"web_search_failure_class": webSearchFailureClass,
			"no_progress_passes":       noProgressPasses,
		}),
	})
	service.debug.LogRuntime(context.Background(), requestID, conversationID, "provider_loop_terminal", map[string]any{
		"reason":                   strings.TrimSpace(reason),
		"agent_role":               string(role),
		"progress_guard_enabled":   providerProgressGuardEnabled(),
		"provider_pass":            providerPass,
		"progress":                 string(progress),
		"last_outcome":             string(progress),
		"last_tool":                lastToolName,
		"web_search_failure_class": webSearchFailureClass,
		"no_progress_passes":       noProgressPasses,
	})
	message := fmt.Sprintf("provider loop stopped: %s", reason)
	switch reason {
	case "blocked_no_progress":
		message = fmt.Sprintf("stopped after repeated no-progress tool results; last_tool=%s", firstNonEmpty(strings.TrimSpace(lastToolName), "unknown"))
	case "max_provider_passes":
		message = fmt.Sprintf("stopped after reaching the provider pass safety cap (%d)", maxProviderPasses)
	case "missing_tool_result":
		message = "stopped because a tool call could not be paired with exactly one result"
	}
	return service.failStream(stream, reason, fmt.Errorf("%s", message))
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func providerProgressGuardEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(providerProgressGuardEnv))) {
	case "0", "false", "off", "disabled":
		return false
	default:
		return true
	}
}

func boundedWebSearchFailureClass(resultText string) string {
	if strings.TrimSpace(resultText) == "" {
		return ""
	}
	start := strings.Index(resultText, "{")
	end := strings.LastIndex(resultText, "}")
	if start < 0 || end <= start {
		return ""
	}
	var payload struct {
		FailureClass string `json:"failure_class"`
	}
	if err := json.Unmarshal([]byte(resultText[start:end+1]), &payload); err != nil {
		return ""
	}
	value := strings.TrimSpace(payload.FailureClass)
	if len(value) > 64 {
		return value[:64]
	}
	return value
}
