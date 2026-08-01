# Backend / Store Log Tracing

Read this reference when a user provides an ID or asks to trace a request, conversation, model invocation, tool execution, or provider error from local records.

## Fixed Paths

Root directory on user machine:

- `~/.cursor-local-assistant-v2`

The three core persistence components are:

- `history/<conversationId>/state.json`
  - Conversation metadata and current state.
  - Key fields: `request_id`, `conversation_id`, `root_conversation_id`, `parent_conversation_id`, `parent_tool_call_id`, `mode`, `current_loop_status`, `current_request_id`, `current_turn_seq`, `context_version`, `next_turn_seq`, `next_entry_seq`, `latest_request_prefix`, `last_provider_call`, `current_todos`, `current_plans`, token / compaction fields.
- `history/<conversationId>/context.json`
  - Append-only semantic history.
  - Key fields: `version`, `items[]`.
  - Each entry in `items[]` contains `seq`, `turn_seq`, `request_id`, `role`, `kind`, `tool_call_id`, `parent_tool_call_id`, `payload`, `created_at`.
- `history/usage.json`
  - Provider call and turn usage aggregations.
  - Key fields: `totals`, `daily`, `recent_events`, `event_index`.

`logs/app.log` contains application runtime logs; it serves to supplement runtime evidence and is not a conversation source of truth.

The following legacy mechanisms are no longer supported:

- DB-backed store / searchable conversation memory
- HTTP / protocol trace debug UI
- `data/data.sqlite` / `protocol_traces`
- `history/<conversationId>/conversation.json`
- `history/<conversationId>/turns/<n>/request.json|sse.jsonl|summary.json`
- Legacy artifacts under root or conversation directories: `latest.json`, `summary.json`, `replay.json`, `runtime.json`, `request.json`, `recovery.json`, `entries.jsonl`, numeric turn directories.

These legacy artifacts are cleaned by `internal/backend/forwarder/history_maintenance.go`. Do not treat them as current sources of truth.

## Steps When an ID is Provided

### 1. Determine ID Type

Do not assume the ID is a `requestId`. Narrow down in this order:

1. Check if `conversationId`:
   - Check if `history/<id>/state.json` and `history/<id>/context.json` exist.
2. Check if `requestId`:
   - Search `current_request_id`, `latest_request_prefix.request_id`, `last_provider_call.request_id` in `history/*/state.json`.
   - Search `items[].request_id` in `history/*/context.json`.
   - Search in `logs/app.log`.
3. Check if `modelCallId`:
   - Search `latest_request_prefix.model_call_id`, `last_provider_call.model_call_id` in `state.json`.
   - Search `model_call_id` in `context.json.items[].payload`.
   - Search `model_call_id=<id>` in `logs/app.log`.
4. Check if `toolCallId` / `exec_id`:
   - Search `context.json.items[].tool_call_id`, `items[].payload`.
   - Search in protocol/tool logs.

Use local scripts or `rg` for read-only lookup. Do not use SQLite query templates.

### 2. Inspect Sources of Truth After Obtaining `conversationId`

```bash
HISTORY_ROOT="$HOME/.cursor-local-assistant-v2/history"
CONV_ID="<conversation-id>"

ls -la "$HISTORY_ROOT/$CONV_ID"
```

Key checks:

- `state.json`
  - `current_loop_status`: `idle`, `running`, `waiting_tool`, `completed`, `canceled`, `provider_error`, `failed`
  - `current_request_id`, `current_turn_seq`
  - `latest_request_prefix`: Provider/model/openai_endpoint/model_call_id/prompt token summary of latest provider call
  - `last_provider_call`: Status and error text of latest provider call
  - `next_entry_seq`, `next_turn_seq`, `context_version`
  - `current_todos`, `current_plans`
- `context.json`
  - Verify `version` aligns with `state.context_version`
  - Check if `items[]` increases monotonically by `seq`
  - Confirm expected user/request_context/prompt_context/assistant/tool_result/metadata entries exist under same `turn_seq`
  - Check for duplicates, missing entries, or ordering anomalies
- `usage.json`
  - Lookup usage related to request/model-call via `event_index` or `recent_events`
  - Rough cache hit estimate: `totals.cache_read_tokens / (totals.cache_read_tokens + totals.input_tokens)`

### 3. Location of Provider Invocation Evidence

Current provider artifact recorder behavior:

- `RecordLLMRequest(...)`
  - Caches request summary for active provider call.
  - Updates `state.latest_request_prefix` if provider/model/openai_endpoint can be parsed.
  - Does NOT write `request.json`.
- `AppendLLMResponseChunk(...)`
  - Currently a no-op.
  - Does NOT write `sse.jsonl`.
- `RecordLLMSummary(...)`
  - Fills current provider call summary and updates `state.latest_request_prefix.prompt_tokens_total`.
  - Writes aggregated usage to `history/usage.json`.
  - Does NOT write `summary.json`.

For provider error troubleshooting, prioritize inspecting:

- `state.last_provider_call`
- `state.latest_request_prefix`
- Metadata payloads (`provider_error`, `turn_completed`) in `context.json.items`
- `history/usage.json`
- `logs/app.log`
- Request construction and error parsing in `internal/backend/agent/model/openai.go` / `anthropic.go`

## File Generation Mechanism

### Root Paths

- `internal/appdata/paths.go`
  - `RootDir()` fixed to `~/.cursor-local-assistant-v2`
  - `HistoryRootPath()` fixed to `~/.cursor-local-assistant-v2/history`
  - `UsageFilePath()` fixed to `~/.cursor-local-assistant-v2/history/usage.json`
  - `LogsRootPath()` fixed to `~/.cursor-local-assistant-v2/logs`

### `state.json + context.json`

Pipeline source:

- `internal/backend/forwarder/file_store.go`
  - `CreateConversation`, `LoadConversation`, `AppendEntries`, `SaveConversationWithEntries`, `UpdateConversationMeta`, `ReplaceEntries`
- `internal/backend/forwarder/service.go`
  - `handleRunIntent` starts new loop / turn
  - `appendConversationEntries` appends semantic events
- `internal/backend/forwarder/projector.go`
  - `ProjectPromptReplay()` projects `context.json.items` into provider messages

Stable conclusions:

- `state.json` holds current state and mutable metadata.
- `context.json.items` holds replayable semantic history.
- History sent to LLM is projected by the projector from `context.json.items`, not replayed from provider artifacts.
- `state.json.entries` is an in-memory struct field on `ConversationFile`; persisted history lives in `context.json.items`.

### `usage.json`

Pipeline source:

- `internal/backend/forwarder/usage_store.go` (`UpsertEvent`, `LookupEvent`)
- `internal/backend/forwarder/token_usage.go`
- `internal/historymetrics/`

Stable conclusions:

- `usage.json` holds global usage aggregations, not single conversation semantic history.
- `recent_events` retains recent events; long-term totals rely on `totals` / `daily`.

### Legacy Cleanup

Pipeline source: `internal/backend/forwarder/history_maintenance.go`

Stable conclusions:

- `turns/`, `conversation.json`, `entries.jsonl`, `request.json`, `summary.json` are legacy artifacts.
- Startup history maintenance cleans up these old artifacts automatically.

## Quick Rules

- When an ID is provided, check if `history/<id>/state.json` exists first; if not, search `state.json / context.json / logs`.
- On request failure, inspect `state.last_provider_call`, error metadata in `context.json.items`, and `logs/app.log`; do not search for `turns/<n>/summary.json`.
- When pending / tool calls do not close, inspect tool call and result entries under same `turn_seq`, then check upstream messages (`ExecClientMessage`, `ExecClientControlMessage`, `InteractionResponse`).
- On prefix cache anomalies, verify entry append order in `context.json.items` and cache token fields in `usage.json`.
- If history conflicts with logs, trust actively updated `state.json / context.json` first, then use logs to explain runtime execution paths.
