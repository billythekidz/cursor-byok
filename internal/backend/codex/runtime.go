package codex

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

type RuntimeStatus struct {
	Installed     bool   `json:"installed"`
	BinaryPath    string `json:"binaryPath"`
	Version       string `json:"version"`
	Authenticated bool   `json:"authenticated"`
	AuthMethod    string `json:"authMethod"`
	CodexHome     string `json:"codexHome"`
	Error         string `json:"error"`
}

type InstallResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error"`
}

func GetRuntimeStatus(ctx context.Context) RuntimeStatus {
	status := RuntimeStatus{CodexHome: resolveCodexHome()}
	path, err := exec.LookPath("codex")
	if err != nil {
		status.Error = "Codex 未安装，请先安装 Codex"
		return status
	}
	status.Installed = true
	status.BinaryPath = path
	status.Version, err = commandOutput(ctx, path, "--version")
	if err != nil {
		status.Error = fmt.Sprintf("读取 Codex 版本失败: %v", err)
		return status
	}
	authOutput, authErr := commandOutput(ctx, path, "login", "status")
	status.Authenticated = authErr == nil && isAuthenticatedOutput(authOutput)
	if status.Authenticated {
		status.AuthMethod = authMethodFromOutput(authOutput)
	} else {
		status.Error = "Codex 已安装但尚未登录"
	}
	return status
}

func Install(ctx context.Context, emit func(string)) InstallResult {
	cmd := exec.CommandContext(ctx, npmCommand(), "install", "--global", "@openai/codex")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return InstallResult{Error: err.Error()}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return InstallResult{Error: err.Error()}
	}
	if err := cmd.Start(); err != nil {
		return InstallResult{Error: fmt.Sprintf("启动 npm 失败，请确认 Node.js/npm 已安装: %v", err)}
	}
	output := make([]string, 0, 16)
	read := func(scanner *bufio.Scanner) {
		for scanner.Scan() {
			line := redactSecrets(strings.TrimSpace(scanner.Text()))
			if line == "" {
				continue
			}
			output = append(output, line)
			if emit != nil {
				emit(line)
			}
		}
	}
	stdoutDone := make(chan struct{})
	go func() { read(bufio.NewScanner(stdout)); close(stdoutDone) }()
	read(bufio.NewScanner(stderr))
	<-stdoutDone
	err = cmd.Wait()
	result := InstallResult{Success: err == nil, Output: strings.Join(output, "\n")}
	if err != nil {
		result.Error = fmt.Sprintf("npm 安装 Codex 失败: %v", err)
	}
	return result
}

func FindBinary() string {
	path, _ := exec.LookPath("codex")
	return strings.TrimSpace(path)
}

func npmCommand() string {
	if runtime.GOOS == "windows" {
		return "npm.cmd"
	}
	return "npm"
}

func commandOutput(ctx context.Context, path string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	return redactSecrets(strings.TrimSpace(string(output))), err
}

func resolveCodexHome() string {
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		return value
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".codex")
	}
	if current, err := user.Current(); err == nil {
		return filepath.Join(current.HomeDir, ".codex")
	}
	return ""
}

func isAuthenticatedOutput(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "logged in") || strings.Contains(lower, "authenticated") || strings.Contains(lower, "chatgpt") && !strings.Contains(lower, "not logged")
}

func authMethodFromOutput(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "api key") || strings.Contains(lower, "apikey") {
		return "api_key"
	}
	return "chatgpt_oauth"
}

func redactSecrets(value string) string {
	for _, key := range []string{"access_token", "refresh_token", "api_key", "apikey", "authorization", "token"} {
		lower := strings.ToLower(value)
		index := strings.Index(lower, key)
		if index < 0 {
			continue
		}
		separator := strings.IndexAny(value[index:], ":=")
		if separator < 0 {
			continue
		}
		start := index + separator + 1
		end := start
		for end < len(value) && !strings.ContainsRune(" ,;\t\r\n}", rune(value[end])) {
			end++
		}
		value = value[:start] + "<redacted>" + value[end:]
	}
	return value
}
