// types.go định nghĩa giao diện thống nhất về request, sự kiện và định tuyến của tầng adapter mô hình.
package modeladapter

import (
	"context"
	"encoding/json"
	"time"

	"cursor/gen/agentv1"
	runtimecore "cursor/internal/backend/agent/core"
)

const (
	// ReasoningSignatureSourceAnthropic thể hiện signature đến từ Anthropic thinking signature.
	ReasoningSignatureSourceAnthropic = "anthropic"
	// ReasoningSignatureSourceOpenAIResponses thể hiện signature đến từ OpenAI Responses encrypted reasoning content.
	ReasoningSignatureSourceOpenAIResponses = "openai_responses"
)

// Message thể hiện cấu trúc thông điệp thống nhất được tầng adapter mô hình sử dụng.
type Message struct {
	// Role thể hiện vai trò của thông điệp.
	Role string `json:"role"`
	// Content thể hiện nội dung văn bản của thông điệp.
	Content string `json:"content"`
	// ContentParts thể hiện các khối nội dung có cấu trúc trong thông điệp, ví dụ văn bản hoặc hình ảnh.
	ContentParts []ContentPart `json:"content_parts,omitempty"`
	// ReasoningContent thể hiện nội dung suy luận (dành cho mô hình hỗ trợ reasoning).
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// ReasoningSignature thể hiện chữ ký provider cấp cho nội dung suy luận (ví dụ Anthropic thinking signature).
	ReasoningSignature string `json:"reasoning_signature,omitempty"`
	// ReasoningSignatureSource thể hiện nguồn ngữ nghĩa provider của reasoning signature.
	ReasoningSignatureSource string `json:"reasoning_signature_source,omitempty"`
	// OpenAIResponsesReasoningID lưu id gốc của reasoning output item trong Responses.
	OpenAIResponsesReasoningID string `json:"openai_responses_reasoning_id,omitempty"`
	// OpenAIResponsesReasoningStatus lưu status gốc của reasoning output item trong Responses.
	OpenAIResponsesReasoningStatus string `json:"openai_responses_reasoning_status,omitempty"`
	// OpenAIResponsesReasoningSummary lưu summary gốc của reasoning output item trong Responses.
	OpenAIResponsesReasoningSummary json.RawMessage `json:"openai_responses_reasoning_summary,omitempty"`
	// ToolCalls thể hiện các lời gọi hàm do assistant khởi xướng.
	ToolCalls []ToolCallDescriptor `json:"tool_calls,omitempty"`
	// ToolCallID thể hiện id lời gọi mà role tool liên kết.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Name thể hiện tên công cụ của role tool.
	Name string `json:"name,omitempty"`
}

type ToolCallDescriptor struct {
	ID                    string                `json:"id"`
	Index                 int                   `json:"index,omitempty"`
	Type                  string                `json:"type"`
	Function              ToolCallFunctionShape `json:"function"`
	OpenAIResponsesID     string                `json:"openai_responses_id,omitempty"`
	OpenAIResponsesCallID string                `json:"openai_responses_call_id,omitempty"`
	OpenAIResponsesStatus string                `json:"openai_responses_status,omitempty"`
}

type ToolCallFunctionShape struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// StreamRequest thể hiện một yêu cầu luồng mô hình thống nhất.
type StreamRequest struct {
	// RequestID thể hiện request mà lời gọi mô hình hiện tại thuộc về.
	RequestID string
	// RunID thể hiện run mà lời gọi mô hình hiện tại thuộc về.
	RunID string
	// ModelCallID thể hiện định danh lời gọi mô hình hiện tại.
	ModelCallID string
	// ConversationID thể hiện phiên mà lời gọi mô hình hiện tại thuộc về, dùng để ổn định định tuyến prompt cache phía provider.
	ConversationID string
	// Mode thể hiện chế độ chạy hiện tại.
	Mode agentv1.AgentMode
	// ModelID thể hiện định danh mô hình hiện tại.
	ModelID string
	// ThinkingEffort thể hiện lựa chọn cường độ suy nghĩ do client chọn ở vòng chạy này.
	ThinkingEffort string
	// Provider thể hiện loại provider đích, ví dụ openai hoặc anthropic.
	Provider string
	// BaseURL thể hiện địa chỉ cơ sở provider mà request nên được gửi đến.
	BaseURL string
	// APIKey thể hiện thông tin xác thực provider.
	APIKey string
	// ProviderModelID thể hiện định danh mô hình thực tế phía provider.
	ProviderModelID string
	// ResolvedChannelID thể hiện ID kênh adapter mà request này thực sự trúng.
	ResolvedChannelID string
	// ResolvedChannelName thể hiện tên hiển thị của kênh adapter mà request này thực sự trúng.
	ResolvedChannelName string
	// ResolvedContextWindowTokens thể hiện cửa sổ ngữ cảnh của kênh adapter mà request này thực sự trúng.
	ResolvedContextWindowTokens int
	// ReasoningEffort thể hiện cường độ suy luận của provider tương thích OpenAI.
	ReasoningEffort string
	// OpenAIEndpoint thể hiện endpoint API mà provider tương thích OpenAI sử dụng.
	OpenAIEndpoint string
	// OpenAIExtraParamsEnabled thể hiện có bật tham số yêu cầu bổ sung OpenAI hay không.
	OpenAIExtraParamsEnabled bool
	// OpenAIExtraParamsJSON thể hiện đối tượng JSON tham số yêu cầu bổ sung OpenAI.
	OpenAIExtraParamsJSON string
	// CustomHeadersEnabled thể hiện có bật header tùy chỉnh hay không.
	CustomHeadersEnabled bool
	// CustomHeadersJSON thể hiện đối tượng JSON header tùy chỉnh.
	CustomHeadersJSON string
	// AnthropicExtraParamsEnabled thể hiện có bật tham số yêu cầu bổ sung Anthropic hay không.
	AnthropicExtraParamsEnabled bool
	// AnthropicExtraParamsJSON thể hiện đối tượng JSON tham số yêu cầu bổ sung Anthropic.
	AnthropicExtraParamsJSON string
	// AnthropicMaxTokens thể hiện max_tokens của provider tương thích Anthropic.
	AnthropicMaxTokens int
	// AnthropicThinkingEffort thể hiện output_config.effort của Anthropic adaptive thinking.
	AnthropicThinkingEffort string
	// ThinkingBudgetTokens thể hiện ngân sách thinking của Anthropic.
	ThinkingBudgetTokens int
	// Messages thể hiện danh sách thông điệp theo thứ tự.
	Messages []Message
	// WorkspacePath là workspace đầu tiên của Cursor conversation, dùng làm cwd cho Codex.
	WorkspacePath string
	// StableMessageCount thể hiện số lượng thông điệp provider-visible trong messages có thể dùng làm tiền tố cache ổn định.
	StableMessageCount int
	// Tools thể hiện danh sách JSON mô tả công cụ gốc.
	Tools []json.RawMessage
	// MaxTokens thể hiện số token đầu ra tối đa của vòng này.
	MaxTokens int
	// Stream thể hiện request hiện tại phải dùng luồng.
	Stream bool
	// RequestKnobs lưu tóm tắt tham số bổ sung của request vòng này.
	RequestKnobs map[string]any
	// CompileSummary lưu tóm tắt biên dịch prompt hiện tại.
	CompileSummary string
	// Observer chịu trách nhiệm ghi các artifact LLM theo phạm vi request.
	Observer LLMArtifactObserver
	// ArtifactPaths để adapter điền lại đường dẫn artifact.
	ArtifactPaths *LLMArtifactPaths
	// RequestBodyOverride thể hiện thân yêu cầu gốc của provider được tái sử dụng trực tiếp; sau khi đặt, adapter gửi nguyên trạng.
	RequestBodyOverride map[string]any
	// ProviderStreamIdleTimeout thể hiện thời gian chờ idle khi luồng phản hồi provider không có nội dung hiệu lực.
	ProviderStreamIdleTimeout time.Duration
}

// LLMArtifactPaths thể hiện các đường dẫn artifact liên quan đến một lời gọi mô hình.
type LLMArtifactPaths struct {
	RequestPath  string
	ResponsePath string
	SummaryPath  string
}

// LLMArtifactObserver định nghĩa giao diện ghi artifact gốc của lời gọi mô hình.
type LLMArtifactObserver interface {
	RecordLLMRequest(requestID string, runID string, modelCallID string, payload map[string]any) (string, error)
	AppendLLMResponseChunk(requestID string, runID string, modelCallID string, chunk string) (string, error)
	RecordLLMSummary(requestID string, runID string, modelCallID string, payload map[string]any) (string, error)
}

// ModelEventKind thể hiện loại sự kiện mô hình thống nhất.
type ModelEventKind string

const (
	// ModelEventKindTextDelta thể hiện sự kiện gia tăng văn bản.
	ModelEventKindTextDelta ModelEventKind = "text_delta"
	// ModelEventKindThinkingDelta thể hiện sự kiện gia tăng suy nghĩ.
	ModelEventKindThinkingDelta ModelEventKind = "thinking_delta"
	// ModelEventKindThinkingCompleted thể hiện sự kiện kết thúc suy nghĩ.
	ModelEventKindThinkingCompleted ModelEventKind = "thinking_completed"
	// ModelEventKindPartialToolCall thể hiện lời gọi công cụ đã bắt đầu nhưng tham số vẫn đang được tạo theo luồng.
	ModelEventKindPartialToolCall ModelEventKind = "partial_tool_call"
	// ModelEventKindToolCallDelta thể hiện gia tăng theo luồng của tham số hoặc đầu ra lời gọi công cụ.
	ModelEventKindToolCallDelta ModelEventKind = "tool_call_delta"
	// ModelEventKindToolLikeCompleted thể hiện ý định công cụ đã được chốt đầy đủ.
	ModelEventKindToolLikeCompleted ModelEventKind = "tool_like_completed"
	// ModelEventKindTurnFinished thể hiện lượt mô hình hiện tại đã kết thúc.
	ModelEventKindTurnFinished ModelEventKind = "turn_finished"
	// ModelEventKindProviderError thể hiện phía provider trả về lỗi.
	ModelEventKindProviderError ModelEventKind = "provider_error"
)

// ModelEvent thể hiện một sự kiện mô hình thống nhất.
type ModelEvent struct {
	// Kind thể hiện loại sự kiện.
	Kind ModelEventKind
	// OccurredAt thể hiện thời điểm sự kiện provider hiện tại xảy ra.
	OccurredAt time.Time
	// Provider thể hiện provider mà sự kiện hiện tại thuộc về.
	Provider string
	// Model thể hiện định danh mô hình mà sự kiện hiện tại thuộc về.
	Model string
	// Text thể hiện gia tăng văn bản.
	Text string
	// ThinkingStyle thể hiện kiểu suy nghĩ.
	ThinkingStyle agentv1.ThinkingStyle
	// ThinkingDurationMS thể hiện thời lượng suy nghĩ.
	ThinkingDurationMS int32
	// ThinkingSignature thể hiện chữ ký suy nghĩ do provider trả về (ví dụ Anthropic signature_delta).
	ThinkingSignature string
	// ThinkingSignatureSource thể hiện nguồn ngữ nghĩa provider của chữ ký suy nghĩ.
	ThinkingSignatureSource string
	// ProviderItemID lưu id output item gốc của provider, dùng cho stateless Responses replay.
	ProviderItemID string
	// ProviderStatus lưu status output item gốc của provider, dùng cho stateless Responses replay.
	ProviderStatus string
	// ProviderSummary lưu summary output item gốc của provider, dùng cho stateless Responses replay.
	ProviderSummary json.RawMessage
	// ProviderCallID lưu id lời gọi tool/function gốc của provider, dùng cho stateless Responses replay.
	ProviderCallID string
	// ToolCallID thể hiện định danh lời gọi công cụ tương ứng với partial/delta hiện tại.
	ToolCallID string
	// ToolCall lưu snapshot có cấu trúc hiện có thể công bố của partial tool call.
	ToolCall *agentv1.ToolCall
	// ToolCallDelta lưu gia tăng theo luồng liên quan đến lời gọi công cụ hiện tại.
	ToolCallDelta *agentv1.ToolCallDelta
	// ArgsTextDelta lưu gia tăng văn bản tham số công cụ gốc, để tầng tương thích chuyển tiếp.
	ArgsTextDelta string
	// InputTokens thể hiện số token đầu vào đã biết hiện tại.
	InputTokens int64
	// OutputTokens thể hiện số token đầu ra đã biết hiện tại.
	OutputTokens int64
	// CacheReadTokens thể hiện số token cache read đã biết hiện tại.
	CacheReadTokens int64
	// CacheWriteTokens thể hiện số token cache write đã biết hiện tại.
	CacheWriteTokens int64
	// UsagePresent thể hiện provider trong luồng này có thực sự trả về đối tượng usage hay không.
	UsagePresent bool
	// CacheReadPresent thể hiện provider có tường minh trả về trường token cache read hay không.
	CacheReadPresent bool
	// CacheWritePresent thể hiện provider có tường minh trả về trường token cache write hay không.
	CacheWritePresent bool
	// ToolInvocation thể hiện ý định gọi công cụ đã được chốt đầy đủ.
	ToolInvocation *runtimecore.ToolInvocation
	// FinishReason thể hiện lý do kết thúc lượt.
	FinishReason string
	// Err thể hiện lỗi provider.
	Err error
}

// ModelAdapter định nghĩa giao diện adapter provider cụ thể.
type ModelAdapter interface {
	// Stream gửi yêu cầu theo kiểu luồng và liên tục tạo ra các sự kiện mô hình thống nhất.
	Stream(ctx context.Context, req StreamRequest, sink func(ModelEvent) error) error
}

// ModelAdapterRouter định nghĩa giao diện định tuyến provider.
type ModelAdapterRouter interface {
	// Stream chọn adapter provider cơ bản theo định danh mô hình.
	Stream(ctx context.Context, req StreamRequest, sink func(ModelEvent) error) error
}
