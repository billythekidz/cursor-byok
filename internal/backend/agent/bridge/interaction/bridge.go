// bridge.go implements the MVP-stage interaction bridge protocol mapping.
package interaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	readability "codeberg.org/readeck/go-readability/v2"
	htmlmarkdown "github.com/firecrawl/html-to-markdown"
	mdplugin "github.com/firecrawl/html-to-markdown/plugin"

	"cursor/gen/agentv1"
	"cursor/internal/backend/agent/core"
	"cursor/internal/netproxy"
	"cursor/internal/search/openserp"
)

// InteractionApplyResult represents the minimal normalized result of an interaction bridge result.
type InteractionApplyResult struct {
	// ToolCallID is the tool call identifier the result belongs to.
	ToolCallID string
	// InteractionID is the interaction bridge identifier the result belongs to.
	InteractionID string
	// IsTerminal reports whether the interaction bridge has been finalized.
	IsTerminal bool
	// ToolResultPayload represents the result summary that can continue to be fed to the model.
	ToolResultPayload string
	// ToolCall holds the tool call object that can be used to send ToolCallCompletedUpdate.
	ToolCall *agentv1.ToolCall
}

// InteractionBridge defines the interaction bridge interface.
type InteractionBridge interface {
	// OpenQuery opens an interaction-type tool call.
	OpenQuery(toolCall runtimecore.ToolInvocation) (*agentv1.AgentServerMessage, runtimecore.PendingInteraction, error)
	// ApplyInteractionResponse handles interaction responses.
	ApplyInteractionResponse(msg *agentv1.InteractionResponse, pending runtimecore.PendingInteraction) (InteractionApplyResult, error)
}

// Bridge implements the interaction bridge for the current MVP stage.
type Bridge struct {
	// nextID generates interaction message sequence numbers.
	nextID atomic.Uint32
	// httpClient is responsible for operations that require external network access, such as web search / web fetch.
	httpClient *http.Client
	// openSERP is the local OpenSERP-backed web search client.
	openSERP          *openserp.Client
	openSERPMu        sync.Mutex
	webSearchAttempts map[string]webSearchAttempt
}

type webSearchAttempt struct {
	Count         int
	TerminalClass openserp.FailureClass
	UpdatedAt     time.Time
}

// NewBridge creates an interaction bridge instance.
func NewBridge() *Bridge {
	httpClient := netproxy.NewHTTPClient(15 * time.Second)
	return &Bridge{
		httpClient:        httpClient,
		openSERP:          openserp.NewClient(httpClient),
		webSearchAttempts: make(map[string]webSearchAttempt),
	}
}

// OpenQuery opens an interaction-type tool call.
func (bridge *Bridge) OpenQuery(toolCall runtimecore.ToolInvocation) (*agentv1.AgentServerMessage, runtimecore.PendingInteraction, error) {
	switch toolCall.ToolName {
	case "AskQuestion":
		return bridge.openAskQuestion(toolCall)
	case "CreatePlan":
		return bridge.openCreatePlan(toolCall)
	case "WebSearch":
		return bridge.openWebSearch(toolCall)
	case "WebFetch":
		return bridge.openWebFetch(toolCall)
	case "SwitchMode":
		return bridge.openSwitchMode(toolCall)
	default:
		return nil, runtimecore.PendingInteraction{}, fmt.Errorf("unsupported interaction tool: %s", toolCall.ToolName)
	}
}

// ApplyInteractionResponse handles interaction responses.
func (bridge *Bridge) ApplyInteractionResponse(msg *agentv1.InteractionResponse, pending runtimecore.PendingInteraction) (InteractionApplyResult, error) {
	if msg == nil {
		return InteractionApplyResult{}, fmt.Errorf("interaction response is required")
	}

	result := InteractionApplyResult{
		ToolCallID:    pending.ToolCallID,
		InteractionID: pending.InteractionID,
		IsTerminal:    true,
	}
	switch pending.InteractionKind {
	case "ask_question":
		var args agentv1.AskQuestionArgs
		_ = json.Unmarshal(pending.ArgsJSON, &args)
		result.ToolResultPayload = summarizeAskQuestionResponse(msg.GetAskQuestionInteractionResponse())
		result.ToolCall = &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_AskQuestionToolCall{
				AskQuestionToolCall: &agentv1.AskQuestionToolCall{
					Args:   &args,
					Result: msg.GetAskQuestionInteractionResponse().GetResult(),
				},
			},
		}
		return result, nil
	case "create_plan":
		args, err := runtimecore.DecodeCreatePlanArgsJSON(pending.ArgsJSON)
		if err != nil {
			args = &agentv1.CreatePlanArgs{}
		}
		createPlanResult := normalizeCreatePlanResult(msg.GetCreatePlanRequestResponse())
		result.ToolResultPayload = summarizeCreatePlanResult(createPlanResult)
		result.ToolCall = &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_CreatePlanToolCall{
				CreatePlanToolCall: &agentv1.CreatePlanToolCall{
					Args:   args,
					Result: createPlanResult,
				},
			},
		}
		return result, nil
	case "web_search":
		var args agentv1.WebSearchArgs
		_ = json.Unmarshal(pending.ArgsJSON, &args)
		webSearchResult, payload := bridge.applyWebSearchResponse(msg.GetWebSearchRequestResponse(), &args)
		result.ToolResultPayload = payload
		result.ToolCall = &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_WebSearchToolCall{
				WebSearchToolCall: &agentv1.WebSearchToolCall{
					Args:   &args,
					Result: webSearchResult,
				},
			},
		}
		return result, nil
	case "web_fetch":
		var args agentv1.WebFetchArgs
		_ = json.Unmarshal(pending.ArgsJSON, &args)
		webFetchResult, payload := bridge.applyWebFetchResponse(msg.GetWebFetchRequestResponse(), &args)
		result.ToolResultPayload = payload
		result.ToolCall = &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_WebFetchToolCall{
				WebFetchToolCall: &agentv1.WebFetchToolCall{
					Args:   &args,
					Result: webFetchResult,
				},
			},
		}
		return result, nil
	case "switch_mode":
		var args agentv1.SwitchModeArgs
		_ = json.Unmarshal(pending.ArgsJSON, &args)
		switchModeResult := buildSwitchModeResult(msg.GetSwitchModeRequestResponse(), &args)
		result.ToolResultPayload = summarizeSwitchModeResponse(switchModeResult)
		result.ToolCall = &agentv1.ToolCall{
			Tool: &agentv1.ToolCall_SwitchModeToolCall{
				SwitchModeToolCall: &agentv1.SwitchModeToolCall{
					Args:   &args,
					Result: switchModeResult,
				},
			},
		}
		return result, nil
	default:
		return InteractionApplyResult{}, fmt.Errorf("unsupported pending interaction kind: %s", pending.InteractionKind)
	}
}

// nextMessageID returns the next interaction message sequence number.
func (bridge *Bridge) nextMessageID() uint32 {
	current := bridge.nextID.Add(1)
	if current == 0 {
		current = bridge.nextID.Add(1)
	}
	return current
}

// openAskQuestion constructs an AskQuestion interaction query.
func (bridge *Bridge) openAskQuestion(toolCall runtimecore.ToolInvocation) (*agentv1.AgentServerMessage, runtimecore.PendingInteraction, error) {
	var args agentv1.AskQuestionArgs
	if err := json.Unmarshal(toolCall.ArgsJSON, &args); err != nil {
		return nil, runtimecore.PendingInteraction{}, fmt.Errorf("decode AskQuestion args failed: %w", err)
	}
	messageID := bridge.nextMessageID()
	serverMessage := &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionQuery{
			InteractionQuery: &agentv1.InteractionQuery{
				Id: messageID,
				Query: &agentv1.InteractionQuery_AskQuestionInteractionQuery{
					AskQuestionInteractionQuery: &agentv1.AskQuestionInteractionQuery{
						Args:       &args,
						ToolCallId: toolCall.CallID,
					},
				},
			},
		},
	}
	return serverMessage, runtimecore.PendingInteraction{
		InteractionID:   fmt.Sprintf("%d", messageID),
		ArgsJSON:        append([]byte(nil), toolCall.ArgsJSON...),
		ToolCallID:      toolCall.CallID,
		InteractionKind: "ask_question",
	}, nil
}

// openCreatePlan constructs a CreatePlan interaction query.
func (bridge *Bridge) openCreatePlan(toolCall runtimecore.ToolInvocation) (*agentv1.AgentServerMessage, runtimecore.PendingInteraction, error) {
	args, err := runtimecore.DecodeCreatePlanArgsJSON(toolCall.ArgsJSON)
	if err != nil {
		return nil, runtimecore.PendingInteraction{}, fmt.Errorf("decode CreatePlan args failed: %w", err)
	}
	messageID := bridge.nextMessageID()
	serverMessage := &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionQuery{
			InteractionQuery: &agentv1.InteractionQuery{
				Id: messageID,
				Query: &agentv1.InteractionQuery_CreatePlanRequestQuery{
					CreatePlanRequestQuery: &agentv1.CreatePlanRequestQuery{
						Args:       args,
						ToolCallId: toolCall.CallID,
					},
				},
			},
		},
	}
	return serverMessage, runtimecore.PendingInteraction{
		InteractionID:   fmt.Sprintf("%d", messageID),
		ArgsJSON:        append([]byte(nil), toolCall.ArgsJSON...),
		ToolCallID:      toolCall.CallID,
		InteractionKind: "create_plan",
	}, nil
}

// openWebSearch constructs a WebSearch interaction query.
func (bridge *Bridge) openWebSearch(toolCall runtimecore.ToolInvocation) (*agentv1.AgentServerMessage, runtimecore.PendingInteraction, error) {
	var input struct {
		SearchTerm string `json:"search_term"`
	}
	if err := json.Unmarshal(toolCall.ArgsJSON, &input); err != nil {
		return nil, runtimecore.PendingInteraction{}, fmt.Errorf("decode WebSearch args failed: %w", err)
	}
	messageID := bridge.nextMessageID()
	serverMessage := &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionQuery{
			InteractionQuery: &agentv1.InteractionQuery{
				Id: messageID,
				Query: &agentv1.InteractionQuery_WebSearchRequestQuery{
					WebSearchRequestQuery: &agentv1.WebSearchRequestQuery{
						Args: &agentv1.WebSearchArgs{
							SearchTerm: input.SearchTerm,
							ToolCallId: toolCall.CallID,
						},
					},
				},
			},
		},
	}
	return serverMessage, runtimecore.PendingInteraction{
		InteractionID:   fmt.Sprintf("%d", messageID),
		ArgsJSON:        append([]byte(nil), toolCall.ArgsJSON...),
		ToolCallID:      toolCall.CallID,
		InteractionKind: "web_search",
	}, nil
}

// openWebFetch constructs a WebFetch interaction query.
func (bridge *Bridge) openWebFetch(toolCall runtimecore.ToolInvocation) (*agentv1.AgentServerMessage, runtimecore.PendingInteraction, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(toolCall.ArgsJSON, &input); err != nil {
		return nil, runtimecore.PendingInteraction{}, fmt.Errorf("decode WebFetch args failed: %w", err)
	}
	messageID := bridge.nextMessageID()
	serverMessage := &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionQuery{
			InteractionQuery: &agentv1.InteractionQuery{
				Id: messageID,
				Query: &agentv1.InteractionQuery_WebFetchRequestQuery{
					WebFetchRequestQuery: &agentv1.WebFetchRequestQuery{
						Args: &agentv1.WebFetchArgs{
							Url:        input.URL,
							ToolCallId: toolCall.CallID,
						},
					},
				},
			},
		},
	}
	argsPayload, _ := json.Marshal(agentv1.WebFetchArgs{
		Url:        input.URL,
		ToolCallId: toolCall.CallID,
	})
	return serverMessage, runtimecore.PendingInteraction{
		InteractionID:   fmt.Sprintf("%d", messageID),
		ArgsJSON:        argsPayload,
		ToolCallID:      toolCall.CallID,
		InteractionKind: "web_fetch",
	}, nil
}

// openSwitchMode constructs a SwitchMode interaction query.
func (bridge *Bridge) openSwitchMode(toolCall runtimecore.ToolInvocation) (*agentv1.AgentServerMessage, runtimecore.PendingInteraction, error) {
	var args agentv1.SwitchModeArgs
	if err := json.Unmarshal(toolCall.ArgsJSON, &args); err != nil {
		return nil, runtimecore.PendingInteraction{}, fmt.Errorf("decode SwitchMode args failed: %w", err)
	}
	if err := validateSwitchModeTargetID(args.GetTargetModeId()); err != nil {
		return nil, runtimecore.PendingInteraction{}, err
	}
	args.ToolCallId = toolCall.CallID
	messageID := bridge.nextMessageID()
	serverMessage := &agentv1.AgentServerMessage{
		Message: &agentv1.AgentServerMessage_InteractionQuery{
			InteractionQuery: &agentv1.InteractionQuery{
				Id: messageID,
				Query: &agentv1.InteractionQuery_SwitchModeRequestQuery{
					SwitchModeRequestQuery: &agentv1.SwitchModeRequestQuery{
						Args: &args,
					},
				},
			},
		},
	}
	argsPayload, _ := json.Marshal(&args)
	return serverMessage, runtimecore.PendingInteraction{
		InteractionID:   fmt.Sprintf("%d", messageID),
		ArgsJSON:        argsPayload,
		ToolCallID:      toolCall.CallID,
		InteractionKind: "switch_mode",
	}, nil
}

func validateSwitchModeTargetID(raw string) error {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "agent", "ask", "plan":
		return nil
	default:
		return fmt.Errorf("unsupported target mode id: %q", strings.TrimSpace(raw))
	}
}

// summarizeAskQuestionResponse generates the AskQuestion response summary.
func summarizeAskQuestionResponse(response *agentv1.AskQuestionInteractionResponse) string {
	if response == nil || response.GetResult() == nil {
		return "ask question response missing"
	}
	switch item := response.GetResult().GetResult().(type) {
	case *agentv1.AskQuestionResult_Success:
		if len(item.Success.GetAnswers()) == 0 {
			return "ask question success"
		}
		return fmt.Sprintf("ask question answers=%d", len(item.Success.GetAnswers()))
	case *agentv1.AskQuestionResult_Error:
		return item.Error.GetErrorMessage()
	case *agentv1.AskQuestionResult_Rejected:
		return item.Rejected.GetReason()
	case *agentv1.AskQuestionResult_Async:
		return "ask question async accepted"
	default:
		return "unknown ask question response"
	}
}

const createPlanEmptyURIError = "create plan failed: Cursor returned success with empty planUri"

// normalizeCreatePlanResult handles the abnormal shape where the client succeeded but did not return planUri.
func normalizeCreatePlanResult(response *agentv1.CreatePlanRequestResponse) *agentv1.CreatePlanResult {
	if response == nil || response.GetResult() == nil {
		return nil
	}
	result := response.GetResult()
	if result.GetSuccess() != nil && strings.TrimSpace(result.GetPlanUri()) == "" {
		return &agentv1.CreatePlanResult{
			Result: &agentv1.CreatePlanResult_Error{
				Error: &agentv1.CreatePlanError{Error: createPlanEmptyURIError},
			},
		}
	}
	return result
}

// summarizeCreatePlanResult generates the CreatePlan response summary.
func summarizeCreatePlanResult(result *agentv1.CreatePlanResult) string {
	if result == nil {
		return "create plan response missing"
	}
	switch item := result.GetResult().(type) {
	case *agentv1.CreatePlanResult_Success:
		return fmt.Sprintf("create plan success uri=%s", result.GetPlanUri())
	case *agentv1.CreatePlanResult_Error:
		return item.Error.GetError()
	default:
		return "unknown create plan response"
	}
}

// summarizeWebSearchResponse generates the WebSearch response summary.
func summarizeWebSearchResponse(response *agentv1.WebSearchRequestResponse) string {
	if response == nil {
		return "web search response missing"
	}
	switch item := response.GetResult().(type) {
	case *agentv1.WebSearchRequestResponse_Approved_:
		_ = item
		return "web search approved"
	case *agentv1.WebSearchRequestResponse_Rejected_:
		return item.Rejected.GetReason()
	default:
		return "unknown web search response"
	}
}

// applyWebSearchResponse converts the WebSearch approval response into the final tool result.
func (bridge *Bridge) applyWebSearchResponse(response *agentv1.WebSearchRequestResponse, args *agentv1.WebSearchArgs) (*agentv1.WebSearchResult, string) {
	if response == nil {
		message := "web search response missing"
		return &agentv1.WebSearchResult{
			Result: &agentv1.WebSearchResult_Error{
				Error: &agentv1.WebSearchError{Error: message},
			},
		}, formatWebSearchFailurePayload("", openserp.FailureTerminalNoResults, false, nil, message)
	}
	switch item := response.GetResult().(type) {
	case *agentv1.WebSearchRequestResponse_Approved_:
		_ = item
		searchTerm := strings.TrimSpace(args.GetSearchTerm())
		if searchTerm == "" {
			message := "web search search_term is required"
			return &agentv1.WebSearchResult{
				Result: &agentv1.WebSearchResult_Error{
					Error: &agentv1.WebSearchError{Error: message},
				},
			}, formatWebSearchFailurePayload("", openserp.FailureTerminalNoResults, false, nil, message)
		}
		queryKey := normalizedWebSearchQueryKey(searchTerm)
		queryFingerprint := webSearchQueryFingerprint(queryKey)
		if allowed, terminalClass := bridge.beginWebSearchAttempt(queryKey); !allowed {
			message := "web search blocked after a previous terminal failure for the same query"
			if terminalClass == "" {
				terminalClass = openserp.FailureTerminalTooManyErrors
			}
			return &agentv1.WebSearchResult{
				Result: &agentv1.WebSearchResult_Error{
					Error: &agentv1.WebSearchError{Error: message},
				},
			}, formatWebSearchFailurePayload(queryFingerprint, terminalClass, false, nil, message)
		}
		references, payload, err := bridge.executeWebSearch(searchTerm)
		if err != nil {
			failureClass, retryable, attemptedEngines := classifyWebSearchFailure(err)
			bridge.finishWebSearchAttempt(queryKey, failureClass, retryable)
			message := boundedWebSearchMessage(err.Error())
			return &agentv1.WebSearchResult{
				Result: &agentv1.WebSearchResult_Error{
					Error: &agentv1.WebSearchError{Error: message},
				},
			}, formatWebSearchFailurePayload(queryFingerprint, failureClass, retryable, attemptedEngines, message)
		}
		bridge.clearWebSearchAttempt(queryKey)
		references, payload = truncateWebSearchReplay(strings.TrimSpace(args.GetSearchTerm()), references, payload)
		return &agentv1.WebSearchResult{
			Result: &agentv1.WebSearchResult_Success{
				Success: &agentv1.WebSearchSuccess{References: references},
			},
		}, formatWebSearchSuccessPayload(queryFingerprint, payload)
	case *agentv1.WebSearchRequestResponse_Rejected_:
		message := item.Rejected.GetReason()
		return &agentv1.WebSearchResult{
			Result: &agentv1.WebSearchResult_Rejected{
				Rejected: &agentv1.WebSearchRejected{Reason: item.Rejected.GetReason()},
			},
		}, formatWebSearchFailurePayload("", openserp.FailureTerminalRateLimitOrBlock, false, nil, message)
	default:
		message := "unknown web search response"
		return &agentv1.WebSearchResult{
			Result: &agentv1.WebSearchResult_Error{
				Error: &agentv1.WebSearchError{Error: message},
			},
		}, formatWebSearchFailurePayload("", openserp.FailureTerminalNoResults, false, nil, message)
	}
}

// applyWebFetchResponse converts the WebFetch approval response into the final tool result.
func (bridge *Bridge) applyWebFetchResponse(response *agentv1.WebFetchRequestResponse, args *agentv1.WebFetchArgs) (*agentv1.WebFetchResult, string) {
	if response == nil {
		return &agentv1.WebFetchResult{
			Result: &agentv1.WebFetchResult_Error{
				Error: &agentv1.WebFetchError{
					Url:   args.GetUrl(),
					Error: "web fetch response missing",
				},
			},
		}, "web fetch response missing"
	}
	switch item := response.GetResult().(type) {
	case *agentv1.WebFetchRequestResponse_Approved_:
		_ = item
		markdown, err := bridge.executeWebFetch(strings.TrimSpace(args.GetUrl()))
		if err != nil {
			return &agentv1.WebFetchResult{
				Result: &agentv1.WebFetchResult_Error{
					Error: &agentv1.WebFetchError{
						Url:   args.GetUrl(),
						Error: err.Error(),
					},
				},
			}, err.Error()
		}
		return &agentv1.WebFetchResult{
			Result: &agentv1.WebFetchResult_Success{
				Success: &agentv1.WebFetchSuccess{
					Url:      args.GetUrl(),
					Markdown: markdown,
				},
			},
		}, markdown
	case *agentv1.WebFetchRequestResponse_Rejected_:
		return &agentv1.WebFetchResult{
			Result: &agentv1.WebFetchResult_Rejected{
				Rejected: &agentv1.WebFetchRejected{Reason: item.Rejected.GetReason()},
			},
		}, item.Rejected.GetReason()
	default:
		return &agentv1.WebFetchResult{
			Result: &agentv1.WebFetchResult_Error{
				Error: &agentv1.WebFetchError{
					Url:   args.GetUrl(),
					Error: "unknown web fetch response",
				},
			},
		}, "unknown web fetch response"
	}
}

// buildSwitchModeResult converts the SwitchMode approval response into the final tool result.
func buildSwitchModeResult(response *agentv1.SwitchModeRequestResponse, args *agentv1.SwitchModeArgs) *agentv1.SwitchModeResult {
	if response == nil {
		return &agentv1.SwitchModeResult{
			Result: &agentv1.SwitchModeResult_Error{
				Error: &agentv1.SwitchModeError{Error: "switch mode response missing"},
			},
		}
	}
	switch item := response.GetResult().(type) {
	case *agentv1.SwitchModeRequestResponse_Approved_:
		_ = item
		targetModeID := strings.ToLower(strings.TrimSpace(args.GetTargetModeId()))
		return &agentv1.SwitchModeResult{
			Result: &agentv1.SwitchModeResult_Success{
				Success: &agentv1.SwitchModeSuccess{
					FromModeId: "unknown",
					ToModeId:   targetModeID,
				},
			},
		}
	case *agentv1.SwitchModeRequestResponse_Rejected_:
		return &agentv1.SwitchModeResult{
			Result: &agentv1.SwitchModeResult_Rejected{
				Rejected: &agentv1.SwitchModeRejected{Reason: item.Rejected.GetReason()},
			},
		}
	default:
		return &agentv1.SwitchModeResult{
			Result: &agentv1.SwitchModeResult_Error{
				Error: &agentv1.SwitchModeError{Error: "unknown switch mode response"},
			},
		}
	}
}

// summarizeSwitchModeResponse generates the SwitchMode response summary.
func summarizeSwitchModeResponse(result *agentv1.SwitchModeResult) string {
	if result == nil {
		return "switch mode result missing"
	}
	switch item := result.GetResult().(type) {
	case *agentv1.SwitchModeResult_Success:
		return fmt.Sprintf("switch mode success to=%s", item.Success.GetToModeId())
	case *agentv1.SwitchModeResult_Rejected:
		return item.Rejected.GetReason()
	case *agentv1.SwitchModeResult_Error:
		return item.Error.GetError()
	default:
		return "unknown switch mode result"
	}
}

var (
	webSearchAnchorPattern  = regexp.MustCompile(`(?is)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	webSearchSnippetPattern = regexp.MustCompile(`(?is)<(?:a|div)[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</(?:a|div)>`)
	htmlTitlePattern        = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlTagPattern          = regexp.MustCompile(`(?is)<[^>]+>`)
	webSearchURLOverride    = "https://html.duckduckgo.com/html/?q="
)

const (
	webFetchBodyLimit     = 2 * 1024 * 1024
	webFetchMarkdownLimit = 32 * 1024
	webSearchPayloadLimit = 16 * 1024
	webSearchTitleLimit   = 512
	webSearchChunkLimit   = 2 * 1024
	webSearchTimeout      = 45 * time.Second
	maxWebSearchAttempts  = 2
	maxTrackedWebSearches = 128
)

type webSearchFailure struct {
	Class            openserp.FailureClass
	Retryable        bool
	AttemptedEngines []string
	Cause            error
}

func (err *webSearchFailure) Error() string {
	if err == nil || err.Cause == nil {
		return "web search failed"
	}
	return err.Cause.Error()
}

func (err *webSearchFailure) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (bridge *Bridge) beginWebSearchAttempt(queryKey string) (bool, openserp.FailureClass) {
	if bridge == nil {
		return false, openserp.FailureTerminalTooManyErrors
	}
	bridge.openSERPMu.Lock()
	defer bridge.openSERPMu.Unlock()
	if bridge.webSearchAttempts == nil {
		bridge.webSearchAttempts = make(map[string]webSearchAttempt)
	}
	if _, exists := bridge.webSearchAttempts[queryKey]; !exists && len(bridge.webSearchAttempts) >= maxTrackedWebSearches {
		oldestKey := ""
		var oldestAt time.Time
		for key, candidate := range bridge.webSearchAttempts {
			if oldestKey == "" || candidate.UpdatedAt.Before(oldestAt) {
				oldestKey = key
				oldestAt = candidate.UpdatedAt
			}
		}
		if oldestKey != "" {
			delete(bridge.webSearchAttempts, oldestKey)
		}
	}
	state := bridge.webSearchAttempts[queryKey]
	if state.TerminalClass != "" || state.Count >= maxWebSearchAttempts {
		return false, state.TerminalClass
	}
	state.Count++
	state.UpdatedAt = time.Now().UTC()
	bridge.webSearchAttempts[queryKey] = state
	return true, ""
}

func (bridge *Bridge) finishWebSearchAttempt(queryKey string, class openserp.FailureClass, retryable bool) {
	if bridge == nil {
		return
	}
	bridge.openSERPMu.Lock()
	defer bridge.openSERPMu.Unlock()
	state := bridge.webSearchAttempts[queryKey]
	if !retryable || state.Count >= maxWebSearchAttempts {
		state.TerminalClass = class
	}
	state.UpdatedAt = time.Now().UTC()
	bridge.webSearchAttempts[queryKey] = state
}

func (bridge *Bridge) clearWebSearchAttempt(queryKey string) {
	if bridge == nil {
		return
	}
	bridge.openSERPMu.Lock()
	delete(bridge.webSearchAttempts, queryKey)
	bridge.openSERPMu.Unlock()
}

func classifyWebSearchFailure(err error) (openserp.FailureClass, bool, []string) {
	if typed, ok := err.(*webSearchFailure); ok && typed != nil {
		return typed.Class, typed.Retryable, append([]string(nil), typed.AttemptedEngines...)
	}
	if err == nil {
		return openserp.FailureTerminalNoResults, false, nil
	}
	normalized := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(normalized, "captcha"), strings.Contains(normalized, "http status 403"), strings.Contains(normalized, "http status 429"):
		return openserp.FailureTerminalRateLimitOrBlock, false, []string{"duckduckgo", "baidu"}
	case strings.Contains(normalized, "no parseable results"):
		return openserp.FailureTerminalNoResults, false, []string{"duckduckgo", "baidu"}
	}
	if class, retryable, attempted := openserp.FailureDetails(err); class != "" {
		return class, retryable, attempted
	}
	return openserp.FailureRetryableTransport, true, []string{"duckduckgo", "baidu"}
}

func normalizedWebSearchQueryKey(searchTerm string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(searchTerm))), " ")
}

func webSearchQueryFingerprint(queryKey string) string {
	sum := sha256.Sum256([]byte(queryKey))
	return fmt.Sprintf("%x", sum[:8])
}

func formatWebSearchFailurePayload(queryFingerprint string, class openserp.FailureClass, retryable bool, attemptedEngines []string, message string) string {
	if len(attemptedEngines) > 8 {
		attemptedEngines = attemptedEngines[:8]
	}
	payload, _ := json.Marshal(map[string]any{
		"status":            "error",
		"failure_class":     string(class),
		"retryable":         retryable,
		"query_fingerprint": queryFingerprint,
		"attempted_engines": attemptedEngines,
		"message":           boundedWebSearchMessage(message),
	})
	return "<web_search_result>" + string(payload) + "</web_search_result>"
}

func boundedWebSearchMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 512 {
		return message[:512] + "..."
	}
	return message
}

func formatWebSearchSuccessPayload(queryFingerprint string, payload string) string {
	return fmt.Sprintf("<web_search_result status=success query_fingerprint=%s>\n%s\n</web_search_result>", queryFingerprint, strings.TrimSpace(payload))
}

func (bridge *Bridge) executeWebSearch(searchTerm string) ([]*agentv1.WebSearchReference, string, error) {
	searchTerm = strings.TrimSpace(searchTerm)
	if searchTerm == "" {
		return nil, "", fmt.Errorf("web search search_term is required")
	}
	client := bridge.httpClient
	if client == nil {
		client = netproxy.NewHTTPClient(15 * time.Second)
	}

	searchContext, cancel := context.WithTimeout(context.Background(), webSearchTimeout)
	defer cancel()
	bridge.openSERPMu.Lock()
	if bridge.openSERP == nil {
		bridge.openSERP = openserp.NewClient(client)
	}
	openSERPClient := bridge.openSERP
	bridge.openSERPMu.Unlock()
	openSERPGroups, openSERPErr := openSERPClient.Search(searchContext, searchTerm)
	if openSERPErr == nil {
		references := openSERPReferences(openSERPGroups)
		if len(references) > 0 {
			return references, formatWebSearchPayload(searchTerm, references), nil
		}
	}

	// Keep the direct HTML path as a single bounded fallback when OpenSERP
	// cannot start or its engine set is unavailable, including ErrTooManyErrors.
	duckReferences, _, duckErr := bridge.tryDuckDuckGoWebSearch(searchContext, client, searchTerm)
	if duckErr == nil && len(duckReferences) > 0 {
		// Append up to five Baidu results after the primary results when available.
		baiduReferences, _, _ := bridge.tryBaiduWebSearch(searchContext, client, searchTerm)
		references := append(duckReferences, baiduReferences...)
		return references, formatWebSearchPayload(searchTerm, references), nil
	}

	// DuckDuckGo failed, fall back to Baidu.
	baiduReferences, baiduPayload, baiduErr := bridge.tryBaiduWebSearch(searchContext, client, searchTerm)
	if baiduErr == nil && len(baiduReferences) > 0 {
		return baiduReferences, baiduPayload, nil
	}

	// Both failed, return a combined error.
	if baiduErr != nil && duckErr != nil {
		cause := fmt.Errorf("web search failed: baidu=%v, duckduckgo=%v", baiduErr, duckErr)
		class, retryable, attempted := classifyWebSearchFailure(openSERPErr)
		if openSERPErr == nil {
			class, retryable, attempted = classifyWebSearchFailure(cause)
		}
		return nil, "", &webSearchFailure{Class: class, Retryable: retryable, AttemptedEngines: attempted, Cause: cause}
	}
	return nil, "", &webSearchFailure{
		Class:            openserp.FailureTerminalNoResults,
		Retryable:        false,
		AttemptedEngines: []string{"duckduckgo", "baidu"},
		Cause:            fmt.Errorf("web search returned no parseable results"),
	}
}

func openSERPReferences(groups []openserp.EngineResults) []*agentv1.WebSearchReference {
	references := make([]*agentv1.WebSearchReference, 0, len(groups)*5)
	for _, group := range groups {
		label := displaySearchEngine(group.SourceEngine)
		if group.Fallback {
			label = fmt.Sprintf("%s fallback for %s", label, displaySearchEngine(group.RequestedEngine))
		}
		for _, result := range group.Results {
			title := strings.TrimSpace(result.Title)
			if title == "" {
				title = strings.TrimSpace(result.URL)
			}
			snippet := strings.TrimSpace(result.Snippet)
			references = append(references, &agentv1.WebSearchReference{
				Title: fmt.Sprintf("[%s] %s", label, title),
				Url:   strings.TrimSpace(result.URL),
				Chunk: snippet,
			})
		}
	}
	return references
}

func displaySearchEngine(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "google":
		return "Google"
	case "baidu":
		return "Baidu"
	case "duckduckgo":
		return "DuckDuckGo"
	case "yandex":
		return "Yandex"
	case "ecosia":
		return "Ecosia"
	default:
		return engine
	}
}

func (bridge *Bridge) tryBaiduWebSearch(ctx context.Context, client *http.Client, searchTerm string) ([]*agentv1.WebSearchReference, string, error) {
	requestURL := baiduWebSearchBaseURL + neturl.QueryEscape(searchTerm)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/68.0.3440.106 Safari/537.36")
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	request.Header.Set("Referer", baiduWebSearchHostURL+"/")
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("baidu http status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return nil, "", err
	}
	references := extractBaiduWebSearchReferences(string(body))
	if len(references) == 0 {
		return nil, "", fmt.Errorf("baidu returned no parseable results")
	}
	if len(references) > 5 {
		references = references[:5]
	}
	resolveBaiduWebSearchRedirects(ctx, client, references)
	return references, formatWebSearchPayload(searchTerm, references), nil
}

func (bridge *Bridge) tryDuckDuckGoWebSearch(ctx context.Context, client *http.Client, searchTerm string) ([]*agentv1.WebSearchReference, string, error) {
	requestURL := webSearchURLOverride + neturl.QueryEscape(searchTerm)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("User-Agent", "cursor-local-agent/1.0")
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("web search http status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return nil, "", err
	}
	references := extractWebSearchReferences(string(body))
	if len(references) == 0 {
		return nil, "", fmt.Errorf("web search returned no parseable results")
	}
	if len(references) > 5 {
		references = references[:5]
	}
	return references, formatWebSearchPayload(searchTerm, references), nil
}

func extractWebSearchReferences(body string) []*agentv1.WebSearchReference {
	anchorMatches := webSearchAnchorPattern.FindAllStringSubmatch(body, 8)
	snippetMatches := webSearchSnippetPattern.FindAllStringSubmatch(body, 8)
	references := make([]*agentv1.WebSearchReference, 0, len(anchorMatches))
	for index, match := range anchorMatches {
		if len(match) < 3 {
			continue
		}
		title := cleanupWebSearchHTML(match[2])
		url := strings.TrimSpace(html.UnescapeString(match[1]))
		snippet := ""
		if index < len(snippetMatches) && len(snippetMatches[index]) >= 2 {
			snippet = cleanupWebSearchHTML(snippetMatches[index][1])
		}
		if title == "" || url == "" {
			continue
		}
		references = append(references, &agentv1.WebSearchReference{
			Title: title,
			Url:   url,
			Chunk: snippet,
		})
	}
	return references
}

func cleanupWebSearchHTML(value string) string {
	withoutTags := htmlTagPattern.ReplaceAllString(value, " ")
	unescaped := html.UnescapeString(withoutTags)
	return strings.Join(strings.Fields(unescaped), " ")
}

func formatWebSearchPayload(searchTerm string, references []*agentv1.WebSearchReference) string {
	lines := []string{
		fmt.Sprintf("Title: Web search results for query: %s", strings.TrimSpace(searchTerm)),
		"Content: Links:",
	}
	for index, reference := range references {
		if reference == nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. [%s](%s)", index+1, strings.TrimSpace(reference.GetTitle()), strings.TrimSpace(reference.GetUrl())))
	}
	snippets := make([]string, 0, len(references))
	for _, reference := range references {
		if reference == nil {
			continue
		}
		chunk := strings.TrimSpace(reference.GetChunk())
		if chunk == "" {
			continue
		}
		snippets = append(snippets, fmt.Sprintf("- %s", chunk))
	}
	if len(snippets) > 0 {
		lines = append(lines, "", strings.Join(snippets, "\n"))
	}
	return strings.Join(lines, "\n")
}

func truncateWebSearchReplay(searchTerm string, references []*agentv1.WebSearchReference, payload string) ([]*agentv1.WebSearchReference, string) {
	truncated := false
	nextReferences := make([]*agentv1.WebSearchReference, 0, len(references))
	for _, reference := range references {
		if reference == nil {
			continue
		}
		title := truncateInteractionText("WebSearch title", reference.GetTitle(), webSearchTitleLimit)
		chunk := truncateInteractionText("WebSearch snippet", reference.GetChunk(), webSearchChunkLimit)
		if title != reference.GetTitle() || chunk != reference.GetChunk() {
			truncated = true
		}
		nextReferences = append(nextReferences, &agentv1.WebSearchReference{
			Title: title,
			Url:   reference.GetUrl(),
			Chunk: chunk,
		})
	}
	nextPayload := formatWebSearchPayload(searchTerm, nextReferences)
	if strings.TrimSpace(payload) != "" && len(nextPayload) == 0 {
		nextPayload = payload
	}
	if len(nextPayload) > webSearchPayloadLimit {
		truncated = true
		nextPayload = truncateInteractionText("WebSearch", nextPayload, webSearchPayloadLimit)
	}
	if truncated && len(nextReferences) > 0 {
		last := nextReferences[len(nextReferences)-1]
		last.Chunk = strings.TrimSpace(last.GetChunk() + "\n\n" + interactionTruncationNotice("WebSearch", webSearchPayloadLimit, len(nextPayload), len(payload)))
		nextPayload = formatWebSearchPayload(searchTerm, nextReferences)
		nextPayload = truncateInteractionText("WebSearch", nextPayload, webSearchPayloadLimit)
	}
	return nextReferences, nextPayload
}

func (bridge *Bridge) executeWebFetch(rawURL string) (string, error) {
	parsedURL, err := validateWebFetchURL(rawURL)
	if err != nil {
		return "", err
	}
	client := bridge.httpClient
	if client == nil {
		client = netproxy.NewHTTPClient(15 * time.Second)
	}
	client = webFetchHTTPClient(client)
	request, err := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "cursor-local-agent/1.0")
	request.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,application/xml,application/json;q=0.9,*/*;q=0.1")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("web fetch http status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, webFetchBodyLimit+1))
	if err != nil {
		return "", err
	}
	if len(body) == 0 {
		return "", fmt.Errorf("web fetch returned empty body")
	}
	if len(body) > webFetchBodyLimit {
		body = body[:webFetchBodyLimit]
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	if !isWebFetchTextContentType(contentType) {
		return "", fmt.Errorf("web fetch unsupported content type %q", contentType)
	}
	markdown, title, err := renderWebFetchMarkdown(parsedURL, body, contentType)
	if err != nil {
		return "", err
	}
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return "", fmt.Errorf("web fetch returned empty markdown")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = parsedURL.String()
	}
	payload := fmt.Sprintf("Title: %s\nURL: %s\n\nContent:\n%s", title, parsedURL.String(), markdown)
	return truncateWebFetchMarkdown(payload), nil
}

func validateWebFetchURL(rawURL string) (*neturl.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("web fetch url is required")
	}
	parsedURL, err := neturl.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("web fetch invalid url: %w", err)
	}
	switch strings.ToLower(parsedURL.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("web fetch only supports http and https urls")
	}
	host := strings.TrimSpace(parsedURL.Hostname())
	if host == "" {
		return nil, fmt.Errorf("web fetch url host is required")
	}
	if isBlockedWebFetchHost(host) {
		return nil, fmt.Errorf("web fetch host is not public-web accessible")
	}
	return parsedURL, nil
}

func isBlockedWebFetchHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

func isWebFetchTextContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/xhtml+xml", "application/xml", "application/json", "application/ld+json", "application/rss+xml", "application/atom+xml":
		return true
	default:
		return strings.HasSuffix(mediaType, "+xml") || strings.HasSuffix(mediaType, "+json")
	}
}

func renderWebFetchMarkdown(pageURL *neturl.URL, body []byte, contentType string) (string, string, error) {
	if !isHTMLLikeContentType(contentType) {
		return string(body), "", nil
	}
	article, err := readability.FromReader(bytes.NewReader(body), pageURL)
	if err == nil {
		var articleHTML bytes.Buffer
		if renderErr := article.RenderHTML(&articleHTML); renderErr == nil && strings.TrimSpace(articleHTML.String()) != "" {
			if markdown, convertErr := convertHTMLToMarkdown(pageURL, articleHTML.String()); convertErr == nil && strings.TrimSpace(markdown) != "" {
				return markdown, article.Title(), nil
			}
		}
	}
	markdown, err := convertHTMLToMarkdown(pageURL, string(body))
	if err != nil {
		return "", "", fmt.Errorf("web fetch markdown conversion failed: %w", err)
	}
	return markdown, extractWebFetchHTMLTitle(string(body)), nil
}

func isHTMLLikeContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml" || mediaType == ""
}

func convertHTMLToMarkdown(pageURL *neturl.URL, htmlBody string) (string, error) {
	converter := htmlmarkdown.NewConverter(htmlmarkdown.DomainFromURL(pageURL.String()), true, nil)
	converter.Use(mdplugin.GitHubFlavored())
	return converter.ConvertString(htmlBody)
}

func extractWebFetchHTMLTitle(htmlBody string) string {
	matches := htmlTitlePattern.FindStringSubmatch(htmlBody)
	if len(matches) < 2 {
		return ""
	}
	return cleanupWebSearchHTML(matches[1])
}

func truncateWebFetchMarkdown(markdown string) string {
	return truncateInteractionText("WebFetch", markdown, webFetchMarkdownLimit)
}

func truncateInteractionText(toolName string, text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	original := len(text)
	notice := fmt.Sprintf("\n\n%s", interactionTruncationNotice(toolName, limit, limit, original))
	for {
		keep := limit - len(notice)
		if keep <= 0 {
			return truncateInteractionUTF8(text, limit)
		}
		kept := truncateInteractionUTF8(text, keep)
		nextNotice := fmt.Sprintf("\n\n%s", interactionTruncationNotice(toolName, limit, len(kept), original))
		output := strings.TrimRight(kept, "\n") + nextNotice
		if len(output) <= limit || nextNotice == notice {
			return output
		}
		notice = nextNotice
	}
}

func interactionTruncationNotice(toolName string, limit int, kept int, original int) string {
	return fmt.Sprintf("[truncated: %s result exceeded %d bytes; showing %d of %d bytes]", toolName, limit, kept, original)
}

func truncateInteractionUTF8(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	if limit > len(text) {
		limit = len(text)
	}
	truncated := text[:limit]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

func webFetchHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = netproxy.NewHTTPClient(15 * time.Second)
	}
	client := *base
	previousCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("web fetch stopped after 10 redirects")
		}
		if _, err := validateWebFetchURL(request.URL.String()); err != nil {
			return err
		}
		if previousCheckRedirect != nil {
			return previousCheckRedirect(request, via)
		}
		return nil
	}
	return &client
}
