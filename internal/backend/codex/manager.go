package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type RunTurnParams struct {
	BinaryPath            string
	ConversationID        string
	Workspace             string
	Model                 string
	Effort                string
	RuntimeVersion        string
	BaseInstructions      string
	DeveloperInstructions string
	Input                 string
}

type threadMapping struct {
	Provider       string `json:"provider"`
	ModelID        string `json:"modelID"`
	CodexThreadID  string `json:"codexThreadID"`
	Workspace      string `json:"workspace"`
	RuntimeVersion string `json:"runtimeVersion"`
}

type runtimeSession struct {
	client      Client
	mu          sync.Mutex
	initialized bool
}

type ClientManager struct {
	root     string
	mu       sync.Mutex
	sessions map[string]*runtimeSession
}

func NewClientManager(mappingRoot string) *ClientManager {
	return &ClientManager{
		root:     strings.TrimSpace(mappingRoot),
		sessions: make(map[string]*runtimeSession),
	}
}

func (manager *ClientManager) RunTurn(ctx context.Context, params RunTurnParams, sink func(Notification) error) error {
	if manager == nil {
		return errors.New("codex client manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	conversationID := strings.TrimSpace(params.ConversationID)
	workspace, err := filepath.Abs(strings.TrimSpace(params.Workspace))
	if err != nil {
		return fmt.Errorf("resolve Codex workspace: %w", err)
	}
	if conversationID == "" || strings.TrimSpace(params.Model) == "" || workspace == "" || strings.TrimSpace(params.Input) == "" {
		return errors.New("Codex conversation, workspace, model and input are required")
	}
	session, err := manager.session(ctx, params.BinaryPath)
	if err != nil {
		return err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.initialized {
		if err := session.client.Initialize(ctx); err != nil {
			manager.removeSession(params.BinaryPath, session)
			return err
		}
		session.initialized = true
	}
	mapping, mappingErr := manager.loadMapping(conversationID)
	threadID := ""
	if mappingErr == nil && mapping.compatible(params.Model, workspace, params.RuntimeVersion) {
		threadID = strings.TrimSpace(mapping.CodexThreadID)
	}
	if threadID != "" {
		if err := session.client.ResumeThread(ctx, threadID); err != nil {
			threadID = ""
		}
	}
	if threadID == "" {
		thread, err := session.client.StartThread(ctx, StartThreadParams{
			CWD:                   workspace,
			Model:                 strings.TrimSpace(params.Model),
			ApprovalPolicy:        "never",
			Sandbox:               "workspace-write",
			BaseInstructions:      params.BaseInstructions,
			DeveloperInstructions: params.DeveloperInstructions,
		})
		if err != nil {
			manager.removeSession(params.BinaryPath, session)
			return err
		}
		threadID = strings.TrimSpace(thread.ID)
		mapping = threadMapping{
			Provider:       "codex",
			ModelID:        strings.TrimSpace(params.Model),
			CodexThreadID:  threadID,
			Workspace:      workspace,
			RuntimeVersion: strings.TrimSpace(params.RuntimeVersion),
		}
		if err := manager.saveMapping(conversationID, mapping); err != nil {
			return err
		}
	}
	turnID, err := session.client.StartTurn(ctx, TurnStartParams{
		ThreadID:       threadID,
		Input:          []UserInput{{Type: "text", Text: params.Input, TextElements: []TextElement{}}},
		CWD:            workspace,
		Model:          strings.TrimSpace(params.Model),
		ApprovalPolicy: "never",
		Effort:         normalizeEffort(params.Effort),
	})
	if err != nil {
		manager.removeSession(params.BinaryPath, session)
		return err
	}
	for {
		select {
		case <-ctx.Done():
			interruptCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = session.client.InterruptTurn(interruptCtx, threadID, turnID)
			cancel()
			return ctx.Err()
		case notice, ok := <-session.client.Notifications():
			if !ok {
				manager.removeSession(params.BinaryPath, session)
				return errors.New("codex app-server notification stream closed")
			}
			if err := sink(notice); err != nil {
				return err
			}
			if notice.Method == "turn/completed" && notificationThreadID(notice.Params) == threadID {
				return nil
			}
			if notice.Method == "error" {
				return fmt.Errorf("Codex app-server reported an error")
			}
		}
	}
}

func (manager *ClientManager) Close() error {
	if manager == nil {
		return nil
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	var closeErr error
	for key, session := range manager.sessions {
		if err := session.client.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		delete(manager.sessions, key)
	}
	return closeErr
}

func (manager *ClientManager) session(ctx context.Context, binaryPath string) (*runtimeSession, error) {
	key := strings.TrimSpace(binaryPath)
	if key == "" {
		key = "codex"
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if session := manager.sessions[key]; session != nil {
		return session, nil
	}
	client, err := NewClient(context.Background(), key)
	if err != nil {
		return nil, err
	}
	session := &runtimeSession{client: client}
	manager.sessions[key] = session
	_ = ctx
	return session, nil
}

func (manager *ClientManager) removeSession(binaryPath string, expected *runtimeSession) {
	key := strings.TrimSpace(binaryPath)
	if key == "" {
		key = "codex"
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.sessions[key] != expected {
		return
	}
	_ = expected.client.Close()
	delete(manager.sessions, key)
}

func (mapping threadMapping) compatible(model string, workspace string, runtimeVersion string) bool {
	if mapping.Provider != "codex" || strings.TrimSpace(mapping.CodexThreadID) == "" {
		return false
	}
	if strings.TrimSpace(mapping.ModelID) != strings.TrimSpace(model) || filepath.Clean(mapping.Workspace) != filepath.Clean(workspace) {
		return false
	}
	return strings.TrimSpace(runtimeVersion) == "" || strings.TrimSpace(mapping.RuntimeVersion) == "" || strings.TrimSpace(mapping.RuntimeVersion) == strings.TrimSpace(runtimeVersion)
}

func (manager *ClientManager) mappingPath(conversationID string) (string, error) {
	if strings.TrimSpace(manager.root) == "" {
		return "", errors.New("Codex history root is not configured")
	}
	if filepath.Base(conversationID) != conversationID || strings.Contains(conversationID, "..") {
		return "", errors.New("invalid Codex conversation id")
	}
	return filepath.Join(manager.root, conversationID, "codex.json"), nil
}

func (manager *ClientManager) loadMapping(conversationID string) (threadMapping, error) {
	path, err := manager.mappingPath(conversationID)
	if err != nil {
		return threadMapping{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return threadMapping{}, err
	}
	var mapping threadMapping
	if err := json.Unmarshal(data, &mapping); err != nil {
		return threadMapping{}, err
	}
	return mapping, nil
}

func (manager *ClientManager) saveMapping(conversationID string, mapping threadMapping) error {
	path, err := manager.mappingPath(conversationID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Codex mapping directory: %w", err)
	}
	data, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "codex-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("persist Codex thread mapping: %w", err)
	}
	return nil
}

func notificationThreadID(params json.RawMessage) string {
	var payload struct {
		ThreadID string `json:"threadId"`
	}
	_ = json.Unmarshal(params, &payload)
	return strings.TrimSpace(payload.ThreadID)
}

func normalizeEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(value))
	case "max":
		return "xhigh"
	default:
		return "medium"
	}
}
