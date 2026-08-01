package prompt

import (
	"embed"
	"fmt"
	"strings"
)

// Mode represents the runtime mode that a prompt asset corresponds to.
type Mode string

const (
	// ModeAsk represents the static assets for Ask mode.
	ModeAsk Mode = "ask"
	// ModePlan represents the static assets for Plan mode.
	ModePlan Mode = "plan"
	// ModeAgent represents the static assets for Agent mode.
	ModeAgent Mode = "agent"
	// ModeDebug represents the static assets for Debug mode.
	ModeDebug Mode = "debug"
	// ModeMultitask represents the static assets for Multitask mode.
	ModeMultitask Mode = "multitask"
	// ModeSubagent represents the static assets for subagent read-only sessions.
	ModeSubagent Mode = "subagent"
)

// assetFS holds static prompt and tools assets organized by mode.
//
//go:embed common_prefix.md ask/prompt.md ask/tools.json plan/prompt.md plan/system_reminder.txt plan/tools.json agent/prompt.md agent/tools.json debug/prompt.md debug/tools.json debug/system_reminder_initial.txt debug/system_reminder_continuing.txt multitask/prompt.md multitask/tools.json subagent/prompt.md subagent/tools.json compaction/prompt.md commit/prompt.md
var assetFS embed.FS

// normalizeMode validates and normalizes the incoming mode value.
func normalizeMode(mode Mode) (Mode, error) {
	switch mode {
	case ModeAsk, ModePlan, ModeAgent, ModeDebug, ModeMultitask, ModeSubagent:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported prompt mode: %q", mode)
	}
}

// PromptPath returns the static prompt asset path for the given mode.
func PromptPath(mode Mode) (string, error) {
	normalized, err := normalizeMode(mode)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/prompt.md", normalized), nil
}

// ToolsPath returns the static tools asset path for the given mode.
func ToolsPath(mode Mode) (string, error) {
	normalized, err := normalizeMode(mode)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/tools.json", normalized), nil
}

// ReadPrompt reads the static prompt text for the given mode.
func ReadPrompt(mode Mode) (string, error) {
	normalized, err := normalizeMode(mode)
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf("%s/prompt.md", normalized)
	if normalized == ModeSubagent || normalized == ModeDebug {
		data, err := assetFS.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read prompt asset %q: %w", path, err)
		}
		return string(data), nil
	}
	prefix, err := assetFS.ReadFile("common_prefix.md")
	if err != nil {
		return "", fmt.Errorf("read prompt common prefix asset %q: %w", "common_prefix.md", err)
	}
	data, err := assetFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt asset %q: %w", path, err)
	}
	return string(prefix) + "\n\n" + string(data), nil
}

// MustReadPrompt reads the static prompt text for the given mode and panics on failure.
func MustReadPrompt(mode Mode) string {
	text, err := ReadPrompt(mode)
	if err != nil {
		panic(err)
	}
	return text
}

// ReadTools reads the raw tools JSON for the given mode.
func ReadTools(mode Mode) ([]byte, error) {
	path, err := ToolsPath(mode)
	if err != nil {
		return nil, err
	}
	data, err := assetFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tools asset %q: %w", path, err)
	}
	return data, nil
}

// MustReadTools reads the raw tools JSON for the given mode and panics on failure.
func MustReadTools(mode Mode) []byte {
	data, err := ReadTools(mode)
	if err != nil {
		panic(err)
	}
	return data
}

// ReadDebugSystemReminder reads the reminder asset appended each round in Debug mode.
func ReadDebugSystemReminder(initial bool) (string, error) {
	path := "debug/system_reminder_continuing.txt"
	if initial {
		path = "debug/system_reminder_initial.txt"
	}
	data, err := assetFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read debug system reminder asset %q: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// MustReadDebugSystemReminder reads the Debug mode reminder asset and panics on failure.
func MustReadDebugSystemReminder(initial bool) string {
	text, err := ReadDebugSystemReminder(initial)
	if err != nil {
		panic(err)
	}
	return text
}

// ReadPlanSystemReminder reads the dynamic reminder asset appended each round in Plan mode.
func ReadPlanSystemReminder() (string, error) {
	const path = "plan/system_reminder.txt"
	data, err := assetFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read plan system reminder asset %q: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// MustReadPlanSystemReminder reads the Plan mode dynamic reminder asset and panics on failure.
func MustReadPlanSystemReminder() string {
	text, err := ReadPlanSystemReminder()
	if err != nil {
		panic(err)
	}
	return text
}

// ReadCompactionPrompt reads the shared compaction prompt asset.
func ReadCompactionPrompt() (string, error) {
	const path = "compaction/prompt.md"
	data, err := assetFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read compaction prompt asset %q: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// MustReadCompactionPrompt reads the shared compaction prompt asset and panics on failure.
func MustReadCompactionPrompt() string {
	text, err := ReadCompactionPrompt()
	if err != nil {
		panic(err)
	}
	return text
}

// ReadCommitPrompt reads the prompt asset dedicated to commit message generation.
func ReadCommitPrompt() (string, error) {
	const path = "commit/prompt.md"
	data, err := assetFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read commit prompt asset %q: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// MustReadCommitPrompt reads the commit message generation prompt asset and panics on failure.
func MustReadCommitPrompt() string {
	text, err := ReadCommitPrompt()
	if err != nil {
		panic(err)
	}
	return text
}
