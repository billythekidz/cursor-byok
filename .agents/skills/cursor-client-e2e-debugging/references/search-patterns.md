# Search Terms & Decision Tree

## Layer Classification

### `history + logs` / ID Lookup Layer

When symptom is "user provided an ID", "determine whether it is `conversationId`, `requestId`, `modelCallId`, `toolCallId`", "trace state from local history and logs":

Prioritize searching:

- `state.json`
- `context.json`
- `usage.json`
- `logs/app.log`
- `conversation_id`
- `current_request_id`
- `request_id`
- `model_call_id`
- `tool_call_id`
- `latest_request_prefix`
- `last_provider_call`
- `current_loop_status`
- `context_version`
- `next_entry_seq`
- `next_turn_seq`
- `LoadConversation`
- `CreateConversation`
- `SaveConversationWithEntries`
- `AppendEntries`
- `UpdateConversationMeta`
- `ReplaceEntries`
- `ProjectPromptReplay`
- `UsageFileStore`
- `UpsertEvent`
- `LookupEvent`

Do NOT prioritize searching or relying on:

- `data.sqlite`
- `protocol_traces`
- `agent_request_runs`
- `conversation.json`
- `entries.jsonl`
- `turns/<n>`
- `request.json`
- `sse.jsonl`
- `summary.json`

These represent old implementations or legacy artifacts.

### `cursor-agent-exec` / `cursor-agent-worker` Layer

When symptom involves agent main loop, model bridging, `InteractionUpdate` mappings, tool started/completed events, session/provider status:

Prioritize searching installed client split bundles:

- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent-exec/dist/main.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent-exec/dist/*.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent-worker/dist/main.js`

Search terms:

- `registerAgentProvider`
- `CursorAgentProvider`
- `CursorAgentProviderHandle`
- `ClaudeSDKClient`
- `streamInteractionUpdates`
- `handlePartialMessage`
- `AnthropicProxy`
- `getAnthropicProxyPort`
- `getAnthropicProxyAuthToken`
- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_API_KEY`
- `InteractionUpdate`
- `checkpoint`

Note: Legacy path `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent/dist/main.js` may not exist. Confirm `extensions/` structure first.

### Agent Window / Conversation Metadata UI Layer

When symptom involves agent window title, window info, titlebar buttons, clickability, conversation name / metadata updates:

Search in installed client main UI and split bundles:

- `/Applications/Cursor.app/Contents/Resources/app/out/vs/workbench/workbench.desktop.main.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent-exec/dist/main.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent-exec/dist/*.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-always-local/dist/main.js`

Search terms:

- `shouldShowAgentWindowTitleHelperText`
- `glass_open_agents_titlebar_button`
- `open_agent_window_top`
- `open_agent_window_bottom_convo`
- `glass.enable_open_agent_in_window`
- `NameAgentRequest`
- `NameAgentResponse`
- `UpdateConversationMetadataRequest`
- `UpdateConversationMetadataResponse`
- `CreateTranscriptOverviewRequest`
- `createTranscriptOverview`
- `updateConversationMetadata`
- `conversation_checkpoint_update`

Rules:

- `ConversationStateStructure` / `conversation_checkpoint_update` are UI sync snapshots, not persistent modification entry points.
- If client invokes `UpdateConversationMetadata` / `NameAgent`, verify if local backend registers corresponding `/agent.v1.AgentService/*` route explicitly.
- Local mode conversation name / metadata updates land in `history/<conversationId>/state.json` or equivalent persistent metadata, then publish checkpoint to sync UI.

### `cursor-always-local` / Protocol Layer

When symptom involves local mode, client not responding, pending calls failing to close, live checkpoint reconnects scrambling:

Search terms:

- `BidiTransport`
- `startYieldingInputsToTheServer`
- `BidiAppend`
- `RunSSE`
- `AgentServerMessage`
- `AgentClientMessage`
- `ExecServerMessage`
- `ExecClientMessage`
- `ExecClientControlMessage`
- `InteractionQuery`
- `InteractionResponse`
- `conversation_checkpoint_update`

### Repository Forwarder Layer

When symptom involves local backend sending/receiving, provider continue/pause, exec/interaction bridge, history projection:

Search terms:

- `handleRunIntent`
- `driveProvider`
- `startStreamActor`
- `streamCommandEnvelope`
- `handleToolInvocation`
- `handleExecResult`
- `handleExecControl`
- `publishCheckpoint`
- `CheckpointConversation`
- `snapshotCheckpointConversation`
- `appendConversationEntries`
- `OpenExec`
- `OpenQuery`
- `StartStream`
- `deriveConversationLoopState`
- `historyEntryToolCallID`
- `recordProviderUsage`
- `recordTurnUsage`

### Provider / Model Adaptation Layer

When symptom involves provider 400/500, thinking/reasoning, tool_call_id, OpenAI/Anthropic request shapes, usage/cache mismatches:

Search terms:

- `StartStream`
- `StreamRequest`
- `ResolvedChannelID`
- `ResolvedChannelName`
- `ProviderModelID`
- `ThinkingEnabled`
- `buildAnthropicThinkingConfig`
- `normalizeAnthropicProviderMessages`
- `normalizeOpenAIProviderMessages`
- `normalizeOpenAIResponsesInput`
- `reasoning_content`
- `ReasoningContent`
- `ReasoningSignature`
- `RecordLLMRequest`
- `RecordLLMSummary`
- `http_error`
- `namespaceToolCallID`

## Decision Tree

- Single ID provided for log lookup: Check if `history/<id>/state.json` exists first, then scan `history/*/state.json`, `history/*/context.json`, `logs/app.log`.
- Model output semantics incorrect: Check projection from `context.json.items` to `ProjectPromptReplay()`, then provider request normalization.
- Provider 400 / parameter error: Check model adaptation request building, `state.latest_request_prefix`, `state.last_provider_call`, `logs/app.log`.
- Client missing tool results / pending not closing: Inspect `cursor-always-local` and forwarder; check if same `turn_seq` has `tool_result` or control error entry.
- Checkpoint not restoring pending after backend restart: Disk checkpoint is live state only; source of truth after restart is `state.json + context.json`.
- Multiple channels sharing same `modelID`: Check normalized channel ID (`baseURL + modelID + apiKey + displayName + openAIEndpoint` short SHA-256). Resolver remains compatible with legacy `baseURL + modelID + apiKey + displayName`.
- Goal is bridging to another LLM: Inspect model bridge layer; do not delve into entire local runtime by default.
- Installed app behavior conflicts with repo code: Verify active running bundle and perform read-only diff comparison; do NOT patch client.

## Protocol Keywords

Upstream:

- `run_request`
- `exec_client_message`
- `exec_client_control_message`
- `interaction_response`

Downstream:

- `interaction_update`
- `exec_server_message`
- `exec_server_control_message`
- `interaction_query`
- `conversation_checkpoint_update`

If downstream request is observed without matching upstream result or control message, check:

- `exec_id`
- `id`
- `tool_call_id`
- `request_id`
- `model_call_id`
- Pending closure logic

If raw ID is provided, check concurrently:

- `history/<id>/state.json`
- `current_request_id`, `latest_request_prefix`, `last_provider_call` in `history/*/state.json`
- `items[].request_id`, `items[].tool_call_id`, `items[].payload` in `history/*/context.json`
- `event_index` / `recent_events` in `history/usage.json`
- `logs/app.log`
