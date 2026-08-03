// Package codex contains the small stdio app-server client used by the Codex
// model adapter. It intentionally implements only the protocol surface needed
// by the MVP and keeps credentials inside the Codex process.
package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"cursor/internal/logger"
)

var errProcessExited = errors.New("codex app-server process exited")

type Notification struct {
	Method string
	Params json.RawMessage
}

type Thread struct {
	ID string
}

type StartThreadParams struct {
	CWD                   string
	Model                 string
	ApprovalPolicy        string
	Sandbox               string
	BaseInstructions      string
	DeveloperInstructions string
}

type TurnStartParams struct {
	ThreadID       string
	Input          []UserInput
	CWD            string
	Model          string
	ApprovalPolicy string
	Effort         string
}

type UserInput struct {
	Type         string        `json:"type"`
	Text         string        `json:"text,omitempty"`
	TextElements []TextElement `json:"text_elements,omitempty"`
}

type TextElement struct{}

type Client interface {
	Initialize(ctx context.Context) error
	StartThread(ctx context.Context, params StartThreadParams) (Thread, error)
	ResumeThread(ctx context.Context, threadID string) error
	StartTurn(ctx context.Context, params TurnStartParams) (string, error)
	InterruptTurn(ctx context.Context, threadID string, turnID string) error
	Notifications() <-chan Notification
	Close() error
}

type rpcClient struct {
	cmd             *exec.Cmd
	stdin           io.WriteCloser
	writeMu         sync.Mutex
	pendingMu       sync.Mutex
	pending         map[int64]chan rpcResult
	nextID          atomic.Int64
	notices         chan Notification
	done            chan struct{}
	closeOnce       sync.Once
	noticeCloseOnce sync.Once
	errMu           sync.Mutex
	err             error
}

type rpcResult struct {
	result json.RawMessage
	err    error
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewClient(ctx context.Context, binaryPath string) (Client, error) {
	path := strings.TrimSpace(binaryPath)
	if path == "" {
		path = "codex"
	}
	cmd := exec.CommandContext(ctx, path, "app-server", "--listen", "stdio://")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open codex app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open codex app-server stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open codex app-server stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	client := &rpcClient{
		cmd:     cmd,
		stdin:   stdin,
		pending: make(map[int64]chan rpcResult),
		notices: make(chan Notification, 128),
		done:    make(chan struct{}),
	}
	go client.readStdout(stdout)
	go client.readStderr(stderr)
	go client.waitProcess()
	return client, nil
}

func (client *rpcClient) Initialize(ctx context.Context) error {
	_, err := client.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]string{
			"name":    "cursor-byok",
			"title":   "Cursor BYOK Codex adapter",
			"version": "mvp",
		},
		"capabilities": map[string]any{
			"experimentalApi": false,
		},
	})
	if err != nil {
		return err
	}
	return client.notify("initialized", map[string]any{})
}

func (client *rpcClient) StartThread(ctx context.Context, params StartThreadParams) (Thread, error) {
	payload := map[string]any{
		"cwd":            strings.TrimSpace(params.CWD),
		"model":          strings.TrimSpace(params.Model),
		"approvalPolicy": strings.TrimSpace(params.ApprovalPolicy),
		"sandbox":        strings.TrimSpace(params.Sandbox),
	}
	if strings.TrimSpace(params.BaseInstructions) != "" {
		payload["baseInstructions"] = params.BaseInstructions
	}
	if strings.TrimSpace(params.DeveloperInstructions) != "" {
		payload["developerInstructions"] = params.DeveloperInstructions
	}
	result, err := client.call(ctx, "thread/start", payload)
	if err != nil {
		return Thread{}, err
	}
	var response struct {
		Thread Thread `json:"thread"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return Thread{}, fmt.Errorf("decode thread/start response: %w", err)
	}
	if strings.TrimSpace(response.Thread.ID) == "" {
		return Thread{}, errors.New("codex thread/start response did not contain a thread id")
	}
	return response.Thread, nil
}

func (client *rpcClient) ResumeThread(ctx context.Context, threadID string) error {
	_, err := client.call(ctx, "thread/resume", map[string]any{"threadId": strings.TrimSpace(threadID)})
	return err
}

func (client *rpcClient) StartTurn(ctx context.Context, params TurnStartParams) (string, error) {
	payload := map[string]any{
		"threadId": strings.TrimSpace(params.ThreadID),
		"input":    params.Input,
	}
	if strings.TrimSpace(params.CWD) != "" {
		payload["cwd"] = params.CWD
	}
	if strings.TrimSpace(params.Model) != "" {
		payload["model"] = params.Model
	}
	if strings.TrimSpace(params.ApprovalPolicy) != "" {
		payload["approvalPolicy"] = params.ApprovalPolicy
	}
	if strings.TrimSpace(params.Effort) != "" {
		payload["effort"] = params.Effort
	}
	result, err := client.call(ctx, "turn/start", payload)
	if err != nil {
		return "", err
	}
	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return "", fmt.Errorf("decode turn/start response: %w", err)
	}
	return strings.TrimSpace(response.Turn.ID), nil
}

func (client *rpcClient) InterruptTurn(ctx context.Context, threadID string, turnID string) error {
	_, err := client.call(ctx, "turn/interrupt", map[string]any{
		"threadId": strings.TrimSpace(threadID),
		"turnId":   strings.TrimSpace(turnID),
	})
	return err
}

func (client *rpcClient) Notifications() <-chan Notification {
	return client.notices
}

func (client *rpcClient) Close() error {
	client.closeOnce.Do(func() {
		close(client.done)
		_ = client.stdin.Close()
		if client.cmd.Process != nil {
			_ = client.cmd.Process.Kill()
		}
	})
	return nil
}

func (client *rpcClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := client.nextID.Add(1)
	responseCh := make(chan rpcResult, 1)
	client.pendingMu.Lock()
	client.pending[id] = responseCh
	client.pendingMu.Unlock()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		client.removePending(id)
		return nil, err
	}
	client.writeMu.Lock()
	_, writeErr := client.stdin.Write(append(payload, '\n'))
	client.writeMu.Unlock()
	if writeErr != nil {
		client.removePending(id)
		return nil, fmt.Errorf("send codex %s request: %w", method, writeErr)
	}
	select {
	case response := <-responseCh:
		return response.result, response.err
	case <-ctx.Done():
		client.removePending(id)
		return nil, ctx.Err()
	case <-client.done:
		client.removePending(id)
		return nil, client.processError()
	}
}

func (client *rpcClient) notify(method string, params any) error {
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return err
	}
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	if _, err := client.stdin.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("send codex %s notification: %w", method, err)
	}
	return nil
}

func (client *rpcClient) readStdout(reader io.Reader) {
	defer client.noticeCloseOnce.Do(func() { close(client.notices) })
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var envelope rpcEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			logger.Errorf("codex app-server emitted invalid JSONL: %v", err)
			continue
		}
		if len(envelope.ID) > 0 && (len(envelope.Result) > 0 || envelope.Error != nil) {
			client.resolveResponse(envelope)
			continue
		}
		if strings.TrimSpace(envelope.Method) == "" {
			continue
		}
		if len(envelope.ID) > 0 {
			_ = client.writeServerRequestError(envelope.ID)
			continue
		}
		select {
		case client.notices <- Notification{Method: envelope.Method, Params: append(json.RawMessage(nil), envelope.Params...)}:
		case <-client.done:
			return
		}
	}
	client.setError(scanner.Err())
	client.closeOnce.Do(func() { close(client.done) })
}

func (client *rpcClient) readStderr(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			logger.Infof("codex app-server stderr: %s", redactSecrets(line))
		}
	}
}

func (client *rpcClient) waitProcess() {
	err := client.cmd.Wait()
	if err != nil {
		client.setError(err)
	}
	client.closeOnce.Do(func() { close(client.done) })
	client.pendingMu.Lock()
	defer client.pendingMu.Unlock()
	for id, pending := range client.pending {
		pending <- rpcResult{err: client.processError()}
		delete(client.pending, id)
	}
}

func (client *rpcClient) resolveResponse(envelope rpcEnvelope) {
	var id int64
	if err := json.Unmarshal(envelope.ID, &id); err != nil {
		return
	}
	client.pendingMu.Lock()
	pending := client.pending[id]
	delete(client.pending, id)
	client.pendingMu.Unlock()
	if pending == nil {
		return
	}
	if envelope.Error != nil {
		pending <- rpcResult{err: fmt.Errorf("codex %s request failed (%d): %s", idMethod(envelope), envelope.Error.Code, redactSecrets(envelope.Error.Message))}
		return
	}
	pending <- rpcResult{result: append(json.RawMessage(nil), envelope.Result...)}
}

func (client *rpcClient) writeServerRequestError(id json.RawMessage) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": -32601, "message": "request is not supported by Cursor BYOK MVP"},
	})
	if err != nil {
		return err
	}
	_, err = client.stdin.Write(append(payload, '\n'))
	return err
}

func (client *rpcClient) removePending(id int64) {
	client.pendingMu.Lock()
	delete(client.pending, id)
	client.pendingMu.Unlock()
}

func (client *rpcClient) setError(err error) {
	if err == nil {
		return
	}
	client.errMu.Lock()
	if client.err == nil {
		client.err = err
	}
	client.errMu.Unlock()
}

func (client *rpcClient) processError() error {
	client.errMu.Lock()
	defer client.errMu.Unlock()
	if client.err != nil {
		return fmt.Errorf("%w: %v", errProcessExited, redactSecrets(client.err.Error()))
	}
	return errProcessExited
}

func idMethod(envelope rpcEnvelope) string {
	return "RPC"
}
