package modeladapter

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	maxProviderToolCallIDLen = 64
	toolCallNamespaceHashLen = 12
	toolCallValueHashLen     = 12
)

// namespaceToolCallID thêm namespace cấp model-call cho tool call id gốc của provider,
// tránh các id tái sử dụng qua nhiều lượt như functions.Shell:0 bị client nhầm thành cùng một bubble.
//
// Các provider như OpenAI giới hạn độ dài tool_call_id, vì vậy ở đây dùng băm ngắn của model_call_id
// thay vì UUID đầy đủ, đảm bảo tool_call_id lưu nội bộ vừa ổn định vừa phát lại an toàn cho provider.
func namespaceToolCallID(modelCallID string, rawToolCallID string) string {
	raw := strings.TrimSpace(rawToolCallID)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "::") {
		return providerToolCallID(raw)
	}
	model := strings.TrimSpace(modelCallID)
	if model == "" {
		return providerToolCallID(raw)
	}
	return buildProviderSafeToolCallID(shortToolCallHash(model, toolCallNamespaceHashLen), raw)
}

// providerToolCallID chỉnh tool_call_id lưu bền nội bộ thành độ dài an toàn mà provider chấp nhận.
// Như vậy legacy "<modelCallID>::<rawID>" đã ghi xuống trong các phiên cũ vẫn tiếp tục phát lại được.
func providerToolCallID(toolCallID string) string {
	trimmed := strings.TrimSpace(toolCallID)
	if trimmed == "" {
		return ""
	}
	namespace, raw, ok := splitLegacyToolCallID(trimmed)
	if ok {
		return buildProviderSafeToolCallID(shortToolCallHash(namespace, toolCallNamespaceHashLen), raw)
	}
	if len(trimmed) <= maxProviderToolCallIDLen {
		return trimmed
	}
	return buildProviderSafeToolCallID("", trimmed)
}

type providerToolCallDescriptor struct {
	ID       string                `json:"id"`
	Index    int                   `json:"index,omitempty"`
	Type     string                `json:"type"`
	Function ToolCallFunctionShape `json:"function"`
}

func normalizeToolCallDescriptors(toolCalls []ToolCallDescriptor) []providerToolCallDescriptor {
	if len(toolCalls) == 0 {
		return nil
	}
	normalized := make([]providerToolCallDescriptor, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		item := providerToolCallDescriptor{
			ID:       providerToolCallID(toolCall.ID),
			Index:    toolCall.Index,
			Type:     toolCall.Type,
			Function: toolCall.Function,
		}
		normalized = append(normalized, item)
	}
	return normalized
}

func buildProviderSafeToolCallID(namespace string, raw string) string {
	trimmedRaw := strings.TrimSpace(raw)
	if trimmedRaw == "" {
		return ""
	}
	if namespace == "" && len(trimmedRaw) <= maxProviderToolCallIDLen && !strings.Contains(trimmedRaw, "::") {
		return trimmedRaw
	}

	prefix := "tc"
	if namespace != "" {
		prefix += "_" + namespace
	}
	candidate := prefix + "_" + trimmedRaw
	if len(candidate) <= maxProviderToolCallIDLen {
		return candidate
	}

	rawHash := shortToolCallHash(trimmedRaw, toolCallValueHashLen)
	remaining := maxProviderToolCallIDLen - len(prefix) - len(rawHash) - 2
	if remaining <= 0 {
		return prefix + "_" + rawHash
	}
	suffix := trimmedRaw
	if len(suffix) > remaining {
		suffix = suffix[len(suffix)-remaining:]
	}
	return prefix + "_" + rawHash + "_" + suffix
}

func splitLegacyToolCallID(value string) (namespace string, raw string, ok bool) {
	namespace, raw, ok = strings.Cut(strings.TrimSpace(value), "::")
	if !ok {
		return "", "", false
	}
	namespace = strings.TrimSpace(namespace)
	raw = strings.TrimSpace(raw)
	if namespace == "" || raw == "" {
		return "", "", false
	}
	return namespace, raw, true
}

func shortToolCallHash(value string, size int) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	encoded := hex.EncodeToString(sum[:])
	if size <= 0 || size > len(encoded) {
		return encoded
	}
	return encoded[:size]
}
