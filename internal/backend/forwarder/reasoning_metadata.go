// reasoning_metadata.go holds the reasoning metadata decisions shared by history and replay.
package forwarder

import (
	"strings"

	modeladapter "cursor/internal/backend/agent/model"
)

func hasReplayableReasoningPayload(reasoningContent string, reasoningSignature string, reasoningSignatureSource string) bool {
	if strings.TrimSpace(reasoningContent) != "" {
		return true
	}
	return strings.TrimSpace(reasoningSignature) != "" &&
		strings.TrimSpace(reasoningSignatureSource) == modeladapter.ReasoningSignatureSourceOpenAIResponses
}
