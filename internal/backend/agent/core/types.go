// types.go định nghĩa runtime, các lệnh dùng chung, sự kiện, trạng thái và cấu trúc pending.
package runtimecore

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"cursor/gen/agentv1"
)

// SubagentModelOverrideSelection thể hiện việc run cha ghi đè lựa chọn mô hình cho một loại subagent.
type SubagentModelOverrideSelection struct {
	SubagentType                  string `json:"subagent_type"`
	Selection                     string `json:"selection"`
	ModelID                       string `json:"model_id,omitempty"`
	MaxMode                       bool   `json:"max_mode,omitempty"`
	ParameterCount                int    `json:"parameter_count,omitempty"`
	BuiltInModel                  bool   `json:"built_in_model,omitempty"`
	IsVariantStringRepresentation bool   `json:"is_variant_string_representation,omitempty"`
}

// LookupSubagentModelOverride tra cứu ghi đè mô hình thời gian chạy theo subagent_type của Task.
func LookupSubagentModelOverride(overrides map[string]SubagentModelOverrideSelection, subagentType string) (SubagentModelOverrideSelection, string, bool) {
	if len(overrides) == 0 {
		return SubagentModelOverrideSelection{}, "", false
	}
	for _, key := range subagentModelOverrideLookupKeys(subagentType) {
		if selection, ok := overrides[key]; ok {
			return selection, key, true
		}
	}
	return SubagentModelOverrideSelection{}, "", false
}

func subagentModelOverrideLookupKeys(subagentType string) []string {
	trimmed := strings.TrimSpace(subagentType)
	if trimmed == "" {
		return nil
	}
	keys := []string{trimmed}
	switch trimmed {
	case "generalPurpose":
		keys = append(keys, "explore")
	case "explore":
		keys = append(keys, "generalPurpose")
	case "browserUse":
		keys = append(keys, "browser-use")
	case "browser-use":
		keys = append(keys, "browserUse")
	}
	return keys
}

type RunState string

const (
	// RunStateIdle thể hiện trạng thái nhàn rỗi: session đã tồn tại nhưng không có run đang hoạt động.
	RunStateIdle RunState = "IDLE"
	// RunStateRestoring thể hiện trạng thái khôi phục: đang nạp trạng thái phiên và thông tin khôi phục tối thiểu.
	RunStateRestoring RunState = "RESTORING"
	// RunStatePreparingModelInput thể hiện trạng thái chuẩn bị đầu vào mô hình.
	RunStatePreparingModelInput RunState = "PREPARING_MODEL_INPUT"
	// RunStateStreamingModel thể hiện trạng thái tiêu thụ luồng mô hình.
	RunStateStreamingModel RunState = "STREAMING_MODEL"
	// RunStateWaitingExec thể hiện trạng thái chờ cầu thực thi.
	RunStateWaitingExec RunState = "WAITING_EXEC"
	// RunStateWaitingInteraction thể hiện trạng thái chờ cầu tương tác.
	RunStateWaitingInteraction RunState = "WAITING_INTERACTION"
	// RunStateApplyingExternalResult thể hiện trạng thái ghi lại kết quả từ bên ngoài.
	RunStateApplyingExternalResult RunState = "APPLYING_EXTERNAL_RESULT"
	// RunStateCheckpointing thể hiện trạng thái ghi checkpoint.
	RunStateCheckpointing RunState = "CHECKPOINTING"
	// RunStateCompleted thể hiện trạng thái hoàn thành bình thường.
	RunStateCompleted RunState = "COMPLETED"
	// RunStateCanceled thể hiện trạng thái cuối bị hủy.
	RunStateCanceled RunState = "CANCELED"
	// RunStateFailed thể hiện trạng thái cuối thất bại.
	RunStateFailed RunState = "FAILED"
)

// CommandKind thể hiện loại lệnh đi lên mà runtime nhận được.
type CommandKind string

const (
	// CommandKindRunRequested thể hiện đã nhận `run_request`.
	CommandKindRunRequested CommandKind = "run_requested"
	// CommandKindPrewarmRequested thể hiện đã nhận `prewarm_request`.
	CommandKindPrewarmRequested CommandKind = "prewarm_requested"
	// CommandKindCancelRequested thể hiện đã nhận `conversation_action.cancel_action`.
	CommandKindCancelRequested CommandKind = "cancel_requested"
	// CommandKindConversationActionRecordOnly thể hiện đã nhận `conversation_action` không thuộc loại hủy; giai đoạn hiện tại chỉ ghi lại, không đẩy tiến trạng thái.
	CommandKindConversationActionRecordOnly CommandKind = "conversation_action_record_only"
	// CommandKindExecClientMessage thể hiện đã nhận `exec_client_message`.
	CommandKindExecClientMessage CommandKind = "exec_client_message"
	// CommandKindInteractionResponse thể hiện đã nhận `interaction_response`.
	CommandKindInteractionResponse CommandKind = "interaction_response"
	// CommandKindExecClientControlMessage thể hiện đã nhận `exec_client_control_message`; giai đoạn hiện tại chỉ ghi lại, không đẩy tiến trạng thái.
	CommandKindExecClientControlMessage CommandKind = "exec_client_control_message"
	// CommandKindClientHeartbeat thể hiện đã nhận heartbeat từ client; giai đoạn hiện tại chỉ ghi lại, không đẩy tiến trạng thái.
	CommandKindClientHeartbeat CommandKind = "client_heartbeat"
	// CommandKindKVClientMessage thể hiện đã nhận `kv_client_message`; giai đoạn hiện tại chỉ ghi lại, không đẩy tiến trạng thái.
	CommandKindKVClientMessage CommandKind = "kv_client_message"
)

// Command mô tả một lệnh đi lên được đưa vào tầng điều phối runtime.
type Command struct {
	// Kind xác định ngữ nghĩa runtime của lệnh này.
	Kind CommandKind
	// IsResume đánh dấu lệnh hiện tại có phải khởi động kiểu khôi phục hay không.
	IsResume bool
	// ClientKind giữ lại loại thông điệp cấp cao nhất của tầng giao thức để dễ quan sát và gỡ lỗi.
	ClientKind string
	// HistoryEntry lưu văn bản tóm tắt giao thức, phục vụ phản hồi tổng hợp của MVP hiện tại.
	HistoryEntry string
	// ClientMessage lưu thông điệp giao thức đi lên đã giải mã đầy đủ.
	ClientMessage *agentv1.AgentClientMessage
}

// EventKind thể hiện loại nghiệp vụ của một sự kiện đi xuống có thể phát lại.
type EventKind string

const (
	// EventKindRunStarted thể hiện sự kiện run mới đã được tạo và bắt đầu đi vào đường khôi phục.
	EventKindRunStarted EventKind = "run_started"
	// EventKindStepStarted thể hiện sự kiện bắt đầu bước.
	EventKindStepStarted EventKind = "step_started"
	// EventKindTextDelta thể hiện sự kiện gia tăng văn bản.
	EventKindTextDelta EventKind = "text_delta"
	// EventKindStepCompleted thể hiện sự kiện hoàn thành bước.
	EventKindStepCompleted EventKind = "step_completed"
	// EventKindTurnEnded thể hiện sự kiện kết thúc lượt.
	EventKindTurnEnded EventKind = "turn_ended"
	// EventKindCheckpoint thể hiện sự kiện checkpoint phiên.
	EventKindCheckpoint EventKind = "checkpoint"
	// EventKindCanceled thể hiện sự kiện hủy.
	EventKindCanceled EventKind = "canceled"
	// EventKindHeartbeat thể hiện sự kiện heartbeat phía máy chủ.
	EventKindHeartbeat EventKind = "heartbeat"
)

// Event thể hiện một bản ghi sự kiện đi xuống có thể phát sóng và phát lại.
type Event struct {
	// Seq là số thứ tự sự kiện tăng dần trong phạm vi request.
	Seq int64
	// RequestID là định danh request mà sự kiện thuộc về.
	RequestID string
	// RunID là định danh run mà sự kiện thuộc về.
	RunID string
	// Kind xác định loại nghiệp vụ của sự kiện.
	Kind EventKind
	// Message là phần thân thông điệp giao thức cần chuyển tiếp tới RunSSE.
	Message *agentv1.AgentServerMessage
	// End thể hiện sự kiện này sẽ kết thúc việc đọc SSE hiện tại.
	End bool
	// TerminalErrorCode thể hiện mã lỗi kết nối mà SSE trạng thái cuối cần trả về, ví dụ canceled.
	TerminalErrorCode string
	// TerminalErrorMessage thể hiện thông báo lỗi mà SSE trạng thái cuối cần trả về.
	TerminalErrorMessage string
	// CreatedAt là thời điểm sự kiện được ghi vào kho.
	CreatedAt time.Time
}

// RunSnapshot thể hiện thông tin snapshot tối thiểu của một run.
type RunSnapshot struct {
	// RunID là định danh duy nhất của run.
	RunID string
	// RequestID là định danh request mà run hiện tại gắn kết.
	RequestID string
	// ConversationID là định danh phiên mà run hiện tại gắn kết.
	ConversationID string
	// ModelID thể hiện định danh mô hình mà lần chạy hiện tại sử dụng.
	ModelID string
	// State thể hiện trạng thái hiện tại của run.
	State RunState
	// Mode thể hiện chế độ phiên mà run hiện tại sử dụng.
	Mode agentv1.AgentMode
	// Version là số phiên bản runtime, để dễ mở rộng cập nhật lạc quan sau này.
	Version int64
	// StartedAt ghi thời điểm khởi động run.
	StartedAt time.Time
	// UpdatedAt ghi thời điểm cập nhật trạng thái gần nhất của run.
	UpdatedAt time.Time
	// CurrentUserMessageText lưu văn bản đầu vào người dùng của turn hiện tại cho đến khi turn này được đưa vào `turns`.
	CurrentUserMessageText string
	// CustomSystemPrompt lưu system prompt tùy chỉnh đi kèm run hiện tại.
	CustomSystemPrompt string
	// RequestContextPayload lưu kết quả tuần tự hóa proto request_context của run hiện tại.
	RequestContextPayload []byte
	// IsPrewarm đánh dấu run hiện tại có được kích hoạt bởi `prewarm_request` hay không.
	IsPrewarm bool
}

// PendingAssistantOutput thể hiện một bản ghi đầu ra assistant chưa được chốt.
type PendingAssistantOutput struct {
	// RawMessage lưu thông điệp assistant đã tuần tự hóa gốc.
	RawMessage string
	// Role thể hiện role của bản ghi này; giá trị phổ biến hiện tại là assistant.
	Role string
	// ContentKinds ghi thứ tự loại khối nội dung, ví dụ text hoặc tool-call.
	ContentKinds []string
	// ToolCallIDs ghi tất cả tool_call_id xuất hiện trong đầu ra này.
	ToolCallIDs []string
	// ToolNames ghi tất cả tên công cụ xuất hiện trong đầu ra này.
	ToolNames []string
	// TextPreview lưu tóm tắt ngắn gọn của khối văn bản.
	TextPreview string
}

// PendingExec thể hiện một bản ghi cầu thực thi chưa được chốt.
type PendingExec struct {
	// MessageID là số thứ tự thông điệp cầu được gửi xuống client khi mở cầu thực thi này.
	MessageID uint32
	// ExecID là định danh duy nhất của cầu thực thi.
	ExecID string
	// ProviderPass thể hiện provider pass mà cầu thực thi này thuộc về khi được tạo.
	ProviderPass int
	// ModelCallID là định danh lời gọi mô hình kích hoạt cầu thực thi này.
	ModelCallID string
	// ToolCallID là định danh lời gọi công cụ liên kết với cầu thực thi này.
	ToolCallID string
	// ArgsJSON lưu JSON tham số gốc khi mở cầu thực thi, để dễ khôi phục ToolCall đã hoàn thành.
	ArgsJSON []byte
	// ReasoningContent lưu văn bản thinking khi kích hoạt lời gọi công cụ này, để tái sử dụng khi tiếp tục chạy từ checkpoint/replay.
	ReasoningContent string
	// ReasoningSignature lưu chữ ký mà provider cấp cho văn bản thinking hiện tại.
	ReasoningSignature string
	// ReasoningSignatureSource lưu nguồn ngữ nghĩa provider của reasoning signature.
	ReasoningSignatureSource string
	// ExecKind mô tả loại cầu thực thi, ví dụ read, write, shellStream.
	ExecKind string
	// StreamState mô tả giai đoạn hiện tại của cầu thực thi dạng luồng.
	StreamState string
	// OpenedAt thể hiện thời điểm yêu cầu cầu thực thi được gửi đi.
	OpenedAt time.Time
	// FirstChunkAt thể hiện thời điểm khối đầu ra đầu tiên của shellStream.
	FirstChunkAt time.Time
	// ChunkCount thể hiện số khối đầu ra mà shellStream đã nhận.
	ChunkCount int64
	// LastShellActivityAt ghi thời điểm gần nhất của sự kiện đi lên liên quan shell, bao gồm đầu ra, start, heartbeat và close.
	LastShellActivityAt time.Time
	// LastShellHeartbeatAt ghi thời điểm heartbeat shell gần nhất đến.
	LastShellHeartbeatAt time.Time
	// ShellForegroundDeadline thể hiện thời điểm muộn nhất dự kiến shell nền trước nhận trạng thái cuối.
	ShellForegroundDeadline time.Time
	// ShellRecoveryScheduled đánh dấu đã sắp xếp goroutine chốt bất thường cho shell này chưa.
	ShellRecoveryScheduled bool
	// StdoutBuffer lưu văn bản stdout đã tích lũy của shell hiện tại.
	StdoutBuffer string
	// StderrBuffer lưu văn bản stderr đã tích lũy của shell hiện tại.
	StderrBuffer string
	// ArtifactPath lưu đường dẫn artifact cầu nối gốc tương ứng với exec này.
	ArtifactPath string
}

// PendingInteraction thể hiện một bản ghi cầu tương tác chưa được chốt.
type PendingInteraction struct {
	// InteractionID là định danh duy nhất của cầu tương tác.
	InteractionID string
	// ProviderPass thể hiện provider pass mà cầu tương tác này thuộc về khi được tạo.
	ProviderPass int
	// ModelCallID là định danh lời gọi mô hình kích hoạt cầu tương tác này.
	ModelCallID string
	// ToolCallID là định danh lời gọi công cụ liên kết với cầu tương tác này.
	ToolCallID string
	// ArgsJSON lưu JSON tham số gốc khi mở cầu tương tác, để dễ khôi phục trạng thái có cấu trúc khi ghi lại kết quả.
	ArgsJSON []byte
	// ReasoningContent lưu văn bản thinking khi kích hoạt lời gọi công cụ này, để tái sử dụng khi tiếp tục chạy từ checkpoint/replay.
	ReasoningContent string
	// ReasoningSignature lưu chữ ký mà provider cấp cho văn bản thinking hiện tại.
	ReasoningSignature string
	// ReasoningSignatureSource lưu nguồn ngữ nghĩa provider của reasoning signature.
	ReasoningSignatureSource string
	// InteractionKind mô tả loại tương tác, ví dụ ask_question, create_plan.
	InteractionKind string
	// OpenedAt thể hiện thời điểm yêu cầu tương tác được gửi đi.
	OpenedAt time.Time
	// ArtifactPath lưu đường dẫn artifact cầu nối gốc tương ứng với interaction này.
	ArtifactPath string
}

// ActiveStep thể hiện siêu dữ liệu step đang được đẩy tiến, chưa được chốt.
type ActiveStep struct {
	// StepID là định danh duy nhất của step hiện tại.
	StepID uint64
	// ModelCallID là định danh lời gọi mô hình mà step hiện tại gắn kết.
	ModelCallID string
	// StartedAt là thời điểm bắt đầu của step hiện tại.
	StartedAt time.Time
	// InputTokens lưu số token đầu vào đã biết của step hiện tại.
	InputTokens int64
	// OutputTokens lưu số token đầu ra đã biết của step hiện tại.
	OutputTokens int64
}

// ExternalResultSummary thể hiện ngữ cảnh tối thiểu cần thiết để tiếp tục biên dịch vòng tiếp theo sau APPLYING_EXTERNAL_RESULT.
type ExternalResultSummary struct {
	// Source thể hiện nguồn của kết quả, ví dụ exec hoặc interaction.
	Source string
	// ToolName thể hiện tên công cụ hoặc tên tương tác tương ứng.
	ToolName string
	// Payload thể hiện tóm tắt kết quả có thể tiêm trực tiếp vào prompt.
	Payload string
}

// ToolInvocation thể hiện một ý định gọi công cụ do mô hình tạo ra.
type ToolInvocation struct {
	// CallID là định danh lời gọi công cụ ở tầng mô hình.
	CallID string
	// ToolName thể hiện tên công cụ, ví dụ Read, Write, AskQuestion.
	ToolName string
	// ArgsJSON lưu JSON gốc của tham số công cụ.
	ArgsJSON []byte
	// ReasoningContent lưu văn bản thinking đi kèm trước lời gọi công cụ hiện tại.
	ReasoningContent string
	// ReasoningSignature lưu chữ ký mà provider cấp cho văn bản thinking hiện tại.
	ReasoningSignature string
	// ReasoningSignatureSource lưu nguồn ngữ nghĩa provider của reasoning signature.
	ReasoningSignatureSource string
	// ReasoningProviderItemID lưu id item đầu ra reasoning gốc của provider.
	ReasoningProviderItemID string
	// ReasoningProviderStatus lưu status item đầu ra reasoning gốc của provider.
	ReasoningProviderStatus string
	// ReasoningProviderSummary lưu summary item đầu ra reasoning gốc của provider.
	ReasoningProviderSummary json.RawMessage
	// ProviderItemID lưu id item đầu ra tool/function gốc của provider.
	ProviderItemID string
	// ProviderCallID lưu id lời gọi tool/function gốc của provider.
	ProviderCallID string
	// ProviderStatus lưu status item đầu ra tool/function gốc của provider.
	ProviderStatus string
	// ModelCallID thể hiện định danh lời gọi mô hình của vòng này.
	ModelCallID string
}

// NormalizeSupportedMode chuẩn hóa và xác thực chế độ phiên hiện được hỗ trợ.
//
// Quy ước mặc định hiện tại:
// 1. Khi không mang mode tường minh hoặc giá trị là `AGENT_MODE_UNSPECIFIED`, xử lý theo `AGENT_MODE_AGENT`;
// 2. Chỉ cho phép `AGENT_MODE_AGENT`, `AGENT_MODE_ASK`, `AGENT_MODE_PLAN`, `AGENT_MODE_DEBUG`, `AGENT_MODE_MULTITASK`;
// 3. Các mode khác đều báo lỗi, không cho phép hạ cấp im lặng.
func NormalizeSupportedMode(mode agentv1.AgentMode) (agentv1.AgentMode, error) {
	switch mode {
	case agentv1.AgentMode_AGENT_MODE_UNSPECIFIED:
		return agentv1.AgentMode_AGENT_MODE_AGENT, nil
	case agentv1.AgentMode_AGENT_MODE_AGENT,
		agentv1.AgentMode_AGENT_MODE_ASK,
		agentv1.AgentMode_AGENT_MODE_PLAN,
		agentv1.AgentMode_AGENT_MODE_DEBUG,
		agentv1.AgentMode_AGENT_MODE_MULTITASK:
		return mode, nil
	default:
		return agentv1.AgentMode_AGENT_MODE_UNSPECIFIED, fmt.Errorf("unsupported mode: %s", mode.String())
	}
}

// CloneToolCallMap sao chép sâu ánh xạ kết quả tool_call, tránh chia sẻ con trỏ proto.
func CloneToolCallMap(items map[string]*agentv1.ToolCall) map[string]*agentv1.ToolCall {
	if len(items) == 0 {
		return make(map[string]*agentv1.ToolCall)
	}

	cloned := make(map[string]*agentv1.ToolCall, len(items))
	for key, value := range items {
		if value == nil {
			cloned[key] = nil
			continue
		}
		typed, ok := proto.Clone(value).(*agentv1.ToolCall)
		if !ok {
			cloned[key] = nil
			continue
		}
		cloned[key] = typed
	}
	return cloned
}

// IsCurrentlySupportedTool xác định phiên bản ổn định hóa Phase 5 hiện tại có thực sự hỗ trợ công cụ này không.
//
// Quy tắc hiện tại:
// 1. Chỉ trả về những khả năng mà runtime/loop hiện đã có đầy đủ chuỗi đẩy tiến;
// 2. Kết quả dùng để giới hạn tập công cụ thực sự phơi bày cho mô hình, tránh việc mô hình gọi khả năng chưa được triển khai rồi làm thất bại cả vòng run;
// 3. Phải ưu tiên vòng khép kín tối thiểu, thay vì ưu tiên phơi bày các khả năng có trong bản chụp gói nhưng máy chủ chưa hỗ trợ.
func IsCurrentlySupportedTool(name string) bool {
	switch strings.TrimSpace(name) {
	case "Read", "Write", "PatchEdit", "Delete", "Shell", "AwaitShell", "WriteShellStdin", "ForceBackgroundShell",
		"Glob", "Grep", "ReadLints",
		"AskQuestion", "CreatePlan", "SwitchMode", "WebSearch", "WebFetch",
		"TodoWrite", "Task",
		"CallMcpTool", "FetchMcpResource":
		return true
	default:
		return false
	}
}