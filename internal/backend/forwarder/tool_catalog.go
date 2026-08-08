// tool_catalog.go loads and filters the canonical tool catalog from static prompt assets.
package forwarder

import (
	"encoding/json"
	"fmt"
	"strings"

	"cursor/gen/agentv1"
	promptassets "cursor/prompt"
)

type DefaultToolCatalog struct {
}

// NewToolCatalog creates the default tool catalog implementation.
func NewToolCatalog() *DefaultToolCatalog {
	return &DefaultToolCatalog{}
}

// Load reads the tool assets per mode and filters out the tools actually allowed to be exposed at the current stage.
func (catalog *DefaultToolCatalog) Load(mode agentv1.AgentMode, subagentTypeName string) ([]json.RawMessage, []string, error) {
	assetMode, err := toolAssetModeForConversation(mode, subagentTypeName)
	if err != nil {
		return nil, nil, err
	}
	rawTools, err := promptassets.ReadTools(assetMode)
	if err != nil {
		return nil, nil, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(rawTools, &items); err != nil {
		return nil, nil, fmt.Errorf("decode tools asset failed: %w", err)
	}
	filtered := make([]json.RawMessage, 0, len(items))
	names := make([]string, 0, len(items))
	for _, item := range items {
		name, err := extractToolName(item)
		if err != nil {
			return nil, nil, err
		}
		if !isToolAllowedInMode(mode, subagentTypeName, name) {
			continue
		}
		filtered = append(filtered, item)
		names = append(names, name)
	}
	return filtered, names, nil
}

var agentModeToolNames = map[string]struct{}{
	"AskQuestion":          {},
	"CallMcpTool":          {},
	"Delete":               {},
	"FetchMcpResource":     {},
	"GenerateImage":        {},
	"Glob":                 {},
	"Grep":                 {},
	"Ls":                   {},
	"PatchEdit":            {},
	"Read":                 {},
	"ReadLints":            {},
	"Shell":                {},
	"AwaitShell":           {},
	"WriteShellStdin":      {},
	"ForceBackgroundShell": {},
	"SwitchMode":           {},
	"Task":                 {},
	"TodoWrite":            {},
	"WebFetch":             {},
	"WebSearch":            {},
	"Write":                {},
}

var multitaskModeToolNames = map[string]struct{}{
	"AskQuestion":          {},
	"CallMcpTool":          {},
	"Delete":               {},
	"FetchMcpResource":     {},
	"GenerateImage":        {},
	"Glob":                 {},
	"Grep":                 {},
	"Ls":                   {},
	"PatchEdit":            {},
	"Read":                 {},
	"ReadLints":            {},
	"Shell":                {},
	"AwaitShell":           {},
	"WriteShellStdin":      {},
	"ForceBackgroundShell": {},
	"SwitchMode":           {},
	"Task":                 {},
	"TodoWrite":            {},
	"WebFetch":             {},
	"WebSearch":            {},
	"Write":                {},
}

var debugModeToolNames = map[string]struct{}{
	"AskQuestion":          {},
	"CallMcpTool":          {},
	"Delete":               {},
	"FetchMcpResource":     {},
	"Glob":                 {},
	"Grep":                 {},
	"Ls":                   {},
	"PatchEdit":            {},
	"Read":                 {},
	"ReadLints":            {},
	"Shell":                {},
	"AwaitShell":           {},
	"WriteShellStdin":      {},
	"ForceBackgroundShell": {},
	"Task":                 {},
	"TodoWrite":            {},
	"WebFetch":             {},
	"WebSearch":            {},
	"Write":                {},
}

var askModeToolNames = map[string]struct{}{
	"AskQuestion":          {},
	"CallMcpTool":          {},
	"Delete":               {},
	"FetchMcpResource":     {},
	"Glob":                 {},
	"Grep":                 {},
	"Ls":                   {},
	"PatchEdit":            {},
	"Read":                 {},
	"ReadLints":            {},
	"Shell":                {},
	"AwaitShell":           {},
	"WriteShellStdin":      {},
	"ForceBackgroundShell": {},
	"Task":                 {},
	"TodoWrite":            {},
	"WebFetch":             {},
	"WebSearch":            {},
	"Write":                {},
}

var planModeToolNames = map[string]struct{}{
	"AskQuestion":          {},
	"CallMcpTool":          {},
	"CreatePlan":           {},
	"FetchMcpResource":     {},
	"Glob":                 {},
	"Grep":                 {},
	"Ls":                   {},
	"Read":                 {},
	"ReadLints":            {},
	"Shell":                {},
	"AwaitShell":           {},
	"WriteShellStdin":      {},
	"ForceBackgroundShell": {},
	"Task":                 {},
	"TodoWrite":            {},
	"WebFetch":             {},
	"WebSearch":            {},
}

func supportedToolNamesForMode(mode agentv1.AgentMode) map[string]struct{} {
	switch normalizeMode(mode) {
	case agentv1.AgentMode_AGENT_MODE_AGENT:
		return agentModeToolNames
	case agentv1.AgentMode_AGENT_MODE_ASK:
		return askModeToolNames
	case agentv1.AgentMode_AGENT_MODE_PLAN:
		return planModeToolNames
	case agentv1.AgentMode_AGENT_MODE_DEBUG:
		return debugModeToolNames
	case agentv1.AgentMode_AGENT_MODE_MULTITASK:
		return multitaskModeToolNames
	default:
		return nil
	}
}

func isToolAllowedInMode(mode agentv1.AgentMode, subagentTypeName string, toolName string) bool {
	trimmedToolName := strings.TrimSpace(toolName)
	if trimmedToolName == "" {
		return false
	}
	if role := resolveAgentRole(subagentTypeName); isChildAgentRole(role) {
		return isAgentRoleToolAllowed(role, trimmedToolName)
	}
	supported := supportedToolNamesForMode(mode)
	if supported == nil {
		return false
	}
	_, ok := supported[trimmedToolName]
	return ok
}

func isChildConversationSubagentTypeName(subagentTypeName string) bool {
	return strings.TrimSpace(subagentTypeName) != ""
}

func selectToolsByOrderedNames(items []json.RawMessage, orderedNames []string) ([]json.RawMessage, []string, error) {
	byName := make(map[string]json.RawMessage, len(items))
	for _, item := range items {
		name, err := extractToolName(item)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := byName[name]; !exists {
			byName[name] = item
		}
	}
	filtered := make([]json.RawMessage, 0, len(orderedNames))
	names := make([]string, 0, len(orderedNames))
	for _, name := range orderedNames {
		item, ok := byName[name]
		if !ok {
			return nil, nil, fmt.Errorf("tool descriptor %q not found in prompt asset", name)
		}
		filtered = append(filtered, item)
		names = append(names, name)
	}
	return filtered, names, nil
}

func toolAssetModeForConversation(mode agentv1.AgentMode, subagentTypeName string) (promptassets.Mode, error) {
	if isChildConversationSubagentTypeName(subagentTypeName) {
		return promptassets.ModeAgent, nil
	}
	return mapPromptMode(mode)
}

func promptAssetModeForConversation(mode agentv1.AgentMode, subagentTypeName string) (promptassets.Mode, error) {
	if isChildConversationSubagentTypeName(subagentTypeName) {
		return promptassets.ModeSubagent, nil
	}
	return mapPromptMode(mode)
}

// mapPromptMode maps a protocol mode to the directory name of the corresponding static prompt asset.
func mapPromptMode(mode agentv1.AgentMode) (promptassets.Mode, error) {
	switch normalizeMode(mode) {
	case agentv1.AgentMode_AGENT_MODE_AGENT:
		return promptassets.ModeAgent, nil
	case agentv1.AgentMode_AGENT_MODE_ASK:
		return promptassets.ModeAsk, nil
	case agentv1.AgentMode_AGENT_MODE_PLAN:
		return promptassets.ModePlan, nil
	case agentv1.AgentMode_AGENT_MODE_DEBUG:
		return promptassets.ModeDebug, nil
	case agentv1.AgentMode_AGENT_MODE_MULTITASK:
		return promptassets.ModeMultitask, nil
	default:
		return "", fmt.Errorf("unsupported prompt asset mode: %s", mode.String())
	}
}

// extractToolName extracts the function name from the raw tool descriptor JSON.
func extractToolName(raw json.RawMessage) (string, error) {
	var wrapper struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return "", fmt.Errorf("decode tool descriptor failed: %w", err)
	}
	name := strings.TrimSpace(wrapper.Function.Name)
	if name == "" {
		return "", fmt.Errorf("tool descriptor name is required")
	}
	return name, nil
}

// sanitizePromptAsset strips descriptive titles from asset files, keeping only the actual prompt text.
func sanitizePromptAsset(text string, modelName string) string {
	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "# 通用系统提示词", "# 模式静态补充", "---":
			continue
		default:
			filtered = append(filtered, line)
		}
	}
	return promptassets.RenderPromptTemplate(strings.TrimSpace(strings.Join(filtered, "\n")), modelName)
}
