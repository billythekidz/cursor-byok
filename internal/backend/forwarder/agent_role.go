package forwarder

import "strings"

type agentRole string

const (
	agentRoleMain     agentRole = "main"
	agentRoleExplorer agentRole = "explorer"
	agentRolePlanner  agentRole = "planner"
	agentRoleWorker   agentRole = "worker"
)

var readOnlyChildToolNames = map[string]struct{}{
	"FetchMcpResource": {},
	"Glob":             {},
	"Grep":             {},
	"Ls":               {},
	"Read":             {},
	"ReadLints":        {},
	"Shell":            {},
	"WebFetch":         {},
	"WebSearch":        {},
}

func resolveAgentRole(subagentTypeName string) agentRole {
	switch strings.ToLower(strings.TrimSpace(subagentTypeName)) {
	case "":
		return agentRoleMain
	case "explore", "explorer":
		return agentRoleExplorer
	case "plan", "planner":
		return agentRolePlanner
	default:
		return agentRoleWorker
	}
}

func isChildAgentRole(role agentRole) bool {
	return role != agentRoleMain
}

func isAgentRoleToolAllowed(role agentRole, toolName string) bool {
	trimmedToolName := strings.TrimSpace(toolName)
	if trimmedToolName == "" {
		return false
	}
	switch role {
	case agentRoleExplorer, agentRolePlanner:
		_, ok := readOnlyChildToolNames[trimmedToolName]
		return ok
	case agentRoleWorker:
		if trimmedToolName == "AskQuestion" {
			return false
		}
		_, ok := agentModeToolNames[trimmedToolName]
		return ok
	default:
		return false
	}
}

func isAgentRoleInvocationAllowed(role agentRole, toolName string, argsJSON []byte) bool {
	if !isAgentRoleToolAllowed(role, toolName) {
		return false
	}
	if role != agentRoleExplorer && role != agentRolePlanner {
		return true
	}
	if strings.TrimSpace(toolName) != "Shell" {
		return true
	}
	return isReadOnlyShellCommand(shellCommandFromArgs(argsJSON))
}

func isReadOnlyShellCommand(command string) bool {
	normalized := strings.ToLower(strings.TrimSpace(command))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		">", ">>",
		"| out-file", "| set-content", "| add-content",
		"set-content ", "add-content ", "remove-item ", "move-item ", "copy-item ", "new-item ",
		"mkdir ", "rmdir ", "del ", "erase ", "rm ", "mv ", "cp ", "touch ", "chmod ", "chown ",
		"git add ", "git apply", "git checkout", "git clean", "git commit", "git merge", "git mv ", "git reset", "git stash", "git switch ",
		"npm install", "npm uninstall", "pnpm install", "pnpm add", "pnpm remove", "yarn add", "yarn remove", "pip install", "go generate", "cargo fmt",
	} {
		if strings.Contains(normalized, marker) {
			return false
		}
	}
	if strings.Contains(normalized, "powershell") || strings.Contains(normalized, "pwsh") {
		for _, marker := range []string{";", "|", "&", "$", "(", ")", "`", "{", "}", "<", ">"} {
			if strings.Contains(normalized, marker) {
				return false
			}
		}
		return strings.Contains(normalized, "get-content") || strings.Contains(normalized, "get-childitem") || strings.Contains(normalized, "get-location")
	}
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return false
	}
	if fields[0] == "cd" || fields[0] == "pwd" || fields[0] == "where" || fields[0] == "which" {
		return true
	}
	if fields[0] == "git" {
		if len(fields) < 2 {
			return false
		}
		switch fields[1] {
		case "branch", "cat-file", "diff", "log", "ls-files", "rev-parse", "show", "status", "whatchanged":
			return true
		default:
			return false
		}
	}
	for _, executable := range []string{"awk", "cat", "dir", "find", "findstr", "fd", "grep", "head", "ls", "rg", "sed", "tail", "tree", "type"} {
		if fields[0] == executable {
			return true
		}
	}
	if len(fields) >= 2 && fields[0] == "go" && fields[1] == "env" {
		return true
	}
	return false
}

func agentRoleSystemPrompt(role agentRole) string {
	switch role {
	case agentRoleExplorer:
		return "Child role: Explore. This child is strictly read-only. Investigate with read, search, and other non-mutating tools; do not edit or delete files, run state-changing commands, delegate work, or ask the user questions. Return a concise evidence-based finding to the parent."
	case agentRolePlanner:
		return "Child role: Plan. This child is strictly read-only. Inspect the repository and gather evidence for a concrete implementation plan; do not edit or delete files, run state-changing commands, delegate work, or ask the user questions. Return the plan and key risks to the parent."
	case agentRoleWorker:
		return "Child role: implementation worker. Carry the assigned task through investigation, changes, and relevant verification. Use the available mutation and execution tools when needed; do not stop at an investigation summary or a promise to act. If blocked after bounded recovery, report the exact blocker and evidence to the parent. Do not ask the user questions."
	default:
		return ""
	}
}
