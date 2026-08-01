---
name: coding-guidance
description: Local mode implementation guidance
---
Use this guide when the user is working on local mode tasks.

First observe this constraint:

- Do NOT modify installed Cursor client code, bundle, or app copy.
- Reading, searching, comparing, and analyzing client bundles, logs, protocol, and repo code is permitted and recommended.
- If the user mentions "temporarily patching client for e2e", change it to read-only investigation: check actual running copy, collect evidence, compare against repo implementation, and apply fixes strictly within repo code or output clear findings.

If the issue involves any of the following items, also read `../cursor-client-e2e-debugging/SKILL.md`:

- Need to read-only inspect installed Cursor client bundle, logs, or running copy
- Need to confirm which app copy is currently running
- Need to debug collaboration issues between client bundle and repo forwarder
- Need to compare installed client behavior with repo implementation differences

Local mode protocols require prioritizing these files:
- proto/agent_v1.proto
- proto/aiserver_v1.proto
Client is: /Users/leokun/Library/Application\ Support/Cursor 
Client bundle is: /Applications/Cursor.app/Contents/Resources/app/extensions/cursor-always-local/dist/main.js

## Cursor Client Formatted Snapshot

- If the user asks to extract, format, refresh, or normalize Cursor.app snapshot workflows, use the `cursor-app-formatted` skill.
- If `.cursor-app-formatted/` exists in this repository, prioritize reading formatted copies here when inspecting client bundles.
- `.cursor-app-formatted/` is a local snapshot extracted read-only from `/Applications/Cursor.app/Contents/Resources/app` and formatted; it must not be written back, replace, or affect installed Cursor.app.
- Common formatted paths:
  - `.cursor-app-formatted/extensions/cursor-always-local/dist/main.js`
  - `.cursor-app-formatted/extensions/cursor-agent-exec/dist/main.js`
  - `.cursor-app-formatted/extensions/cursor-agent-worker/dist/main.js`
  - `.cursor-app-formatted/out/vs/workbench/workbench.desktop.main.js`
  - `.cursor-app-formatted/out/vs/workbench/api/node/extensionHostProcess.js`
- If `.cursor-app-formatted/` does not exist, is visibly outdated, or requires checking real installation hash, read the raw bundle in `/Applications/Cursor.app`.

## Repo Persisted Conversation Rules

- The source of truth for history passed to LLM in subsequent turns is `history/<conversationId>/state.json` + `history/<conversationId>/context.json`.
- `state.json` stores conversation metadata and current state, e.g. `next_turn_seq`, `next_entry_seq`, `context_version`, `current_todos`, `current_plans`, `latest_request_prefix`, `last_provider_call`.
- `context.json.items` stores append-only semantic history entries; provider messages are not the main store facts, but projected from entries by `ProjectPromptReplay()`.
- Model channel uniqueness is no longer determined by `modelID`; the normalized channel ID is a short `SHA-256` hash of `baseURL + modelID + apiKey + displayName + openAIEndpoint`. The resolver remains compatible with legacy `baseURL + modelID + apiKey + displayName`.
- Replayable history should be appended stably in entry order; history already sent to the model that needs to be preserved cannot be moved to a new position.
- Latest state and volatile state (e.g. active todos, current plans, latest edit protection, dynamic reminders) should be stored in `state.json` or as current turn latest-only suffix; do not accidentally persist them as history that replays endlessly in subsequent turns.
- When a new turn `run_request` arrives, the server loads `state.json + context.json` via `LoadConversation()`, and the projector projects prompt replay; checkpoints/replays brought back by the client do not participate in determining history truth.
- `summary.json`, `replay.json`, `runtime.json`, `request.json`, `conversation.json`, `entries.jsonl`, `turns/` and numeric turn directories are old persistence artifacts and will be cleaned as legacy artifacts by history maintenance.
- If request history is inconsistent with local state, check whether `context.json.items` is missing, duplicated, or out of order, and whether `state.json` fields (`next_entry_seq`, `next_turn_seq`, `context_version`, current state) match entry derivations; do not troubleshoot using old `summary.json` paths.

# Confirmed Conclusions

## 1. `AgentServerMessage` is NOT uniformly expecting a "completion ACK"

Categorize by `oneof message`:

- `exec_server_message`
  - Server request sent to client to execute.
  - Client must explicitly return `ExecClientMessage`.
  - In streaming/error scenarios, client returns `ExecClientControlMessage`:
    - `stream_close`
    - `throw`
    - `heartbeat`
- `interaction_query`
  - Interaction request sent to client.
  - Client must explicitly return `InteractionResponse`.
- `interaction_update`
  - Display/status update message, normally does not require client reply.
- `conversation_checkpoint_update`
  - Checkpoint sync message, normally does not require client reply.
- `kv_server_message`
  - KV sync message, normally does not require client reply.
- `exec_server_control_message`
  - Server control message for exec bridge (e.g. abort), client handles per control semantics, not a generic completion ACK.

## 2. Cursor client has no universal "auto-ACK any ServerMessage" layer

In `cursor-always-local/dist/main.js`, `BidiTransport.startYieldingInputsToTheServer` only sends messages actively produced by client logic to `BidiAppend`:

- It hex encodes `p.value.toBinary()` on the input iterable, then sends `BidiAppendRequest.data`
- Indicates only `AgentClientMessage` actively produced by client logic goes upstream
- No universal mechanism exists that automatically returns completed/ack upon receiving `AgentServerMessage`

Therefore whether client replies depends on whether upper business logic actively constructs a new `AgentClientMessage`.

## 2.1 More Specific Client Conclusions

Directly confirmed from `cursor-always-local/dist/main.js`:

- `AgentServerMessage` downstream types:
  - `interaction_update`
  - `exec_server_message`
  - `exec_server_control_message`
  - `conversation_checkpoint_update`
  - `interaction_query`
- `AgentClientMessage` upstream types:
  - `run_request`
  - `exec_client_message`
  - `exec_client_control_message`
  - `interaction_response`

This means local mode is not a "server message -> generic ACK" model, but:

- `exec_server_message`
  -> Client executes local tool
  -> Produces `exec_client_message`
  -> And optional `exec_client_control_message`
- `interaction_query`
  -> Client displays or handles interaction
  -> Produces `interaction_response`
- Other downstream messages
  -> Generally only update UI / checkpoint / stream status
  -> Will not naturally produce a completion ACK

## 2.2 `exec_server_message` Common Client Reply Forms

Confirmed reply types in client protocol model:

- `ExecClientMessage`
  - Normal result surface
  - Includes `read_result` / `write_result` / `grep_result` / `ls_result` / `diagnostics_result` / `mcp_result` / `shell_stream` etc.
- `ExecClientControlMessage`
  - Control surface
  - Includes:
    - `stream_close`
    - `throw`
    - `heartbeat`

When investigating local mode exec issues, do not focus solely on `ExecClientMessage`:

- Some tools only reply once with a result message
- Streaming tools like shell return mixed:
  - Multiple `shell_stream`
  - And control messages (e.g. `stream_close` / `heartbeat`)

## 2.3 Complete Reply Structure for `exec_server_message`

Factual basis:

- `proto/agent_v1.proto`
  - `ExecServerMessage.oneof message`
  - `ExecClientMessage.oneof message`
  - `ExecClientControlMessage.oneof message`
- `cursor-always-local/dist/main.js`
  - Bundle contains matching proto model
  - `BidiTransport.startYieldingInputsToTheServer` proves client upstream messages originate from active business logic construction

### Result Surface Replies: `ExecServerMessage` -> `ExecClientMessage`

`ExecServerMessage` `message` branches correspond 1-to-1 with `ExecClientMessage` `message` branches:

- `shell_args` -> `shell_result`
- `write_args` -> `write_result`
- `delete_args` -> `delete_result`
- `grep_args` -> `grep_result`
- `read_args` -> `read_result`
- `ls_args` -> `ls_result`
- `diagnostics_args` -> `diagnostics_result`
- `request_context_args` -> `request_context_result`
- `mcp_args` -> `mcp_result`
- `shell_stream_args` -> `shell_stream`
- `background_shell_spawn_args` -> `background_shell_spawn_result`
- `list_mcp_resources_exec_args` -> `list_mcp_resources_exec_result`
- `read_mcp_resource_exec_args` -> `read_mcp_resource_exec_result`
- `fetch_args` -> `fetch_result`
- `record_screen_args` -> `record_screen_result`
- `computer_use_args` -> `computer_use_result`
- `write_shell_stdin_args` -> `write_shell_stdin_result`
- `execute_hook_args` -> `execute_hook_result`
- `subagent_args` -> `subagent_result`

All these result replies carry:

- `id`
- `exec_id`

Server matches using:
1. `exec_id`
2. `id`

### Control Surface Replies: `ExecServerMessage` -> `ExecClientControlMessage`

Besides result replies, client may return control surface messages:

- `stream_close`: indicates current exec stream is closed (`id`)
- `throw`: indicates execution error (`id` + `error` + optional `stack_trace`)
- `heartbeat`: indicates heartbeat during execution (`id`)

### Key Insight

- `ExecClientControlMessage` is not a single `ExecServerMessage` branch's exclusive result type
- It is a cross-exec generic control reply
- Therefore both upstreams must be inspected during investigation: `ExecClientMessage` and `ExecClientControlMessage`

### Investigation Rules

For any `exec_server_message`, confirm at least one of the following occurred:

- Received corresponding `ExecClientMessage`
- Or received `ExecClientControlMessage.throw`
- For streaming exec: check for multiple incremental `ExecClientMessage` and eventual `stream_close`

If only started / pending is seen with no result or control reply, server pending will likely not close.

## 3. Forwarder State Machine Implementation Rules

When modifying local mode forwarder in this repo, observe these constraints:

### 3.1 Resumes Must Be Isolated by Provider Pass

- `request_id` is not a provider call generation; one request can contain multiple provider passes.
- `scheduleProviderResume` cannot rely solely on request-level boolean state (e.g. single `ResumePending`).
- Resume requests must carry source pass to distinguish valid resume triggered by current pass results vs stale resume caused by late tool arrival.
- Clear previous turn resume state at start and end of `driveProvider`.

### 3.2 Late Tool Arrival is Normal; Must Only Affect Its Own Pass

- `ExecClientMessage` / `ExecClientControlMessage` arriving after provider `[DONE]` is normal.
- Late results can only drive checkpoint / history / resume judgment for their own pass, not subsequent passes.
- Non-streaming exec `stream_close` synthetic recovery must use tool source pass, not request-level global state.

### 3.3 Pending Exec Must Match Strictly by ID

- `selectPendingExec` / `selectPendingExecByControl` must match strictly by `exec_id` or `message_id`.
- Do not fallback to returning single remaining pending.
- Late result / `stream_close` / `throw` without pending should check `RecentCompletedExecs` for idempotent ignore instead of attaching to current turn pending.

### 3.4 Stale Resume / Stale Exec Triggers

If any of these occur, check forwarder state machine first:

- New `model_call_id` appears after `[DONE]` for same `request_id`
- `turns/<n+1>/request.json` and `turns/<n>/request.json` messages are nearly identical
- Previous turn tool `grepResult/readResult/...` arrives after `[DONE]`
- Late `stream_close` crosses into next turn after provider starts

Check:
- `ProviderPassCount`
- Source pass of resume requests
- `PendingExec.ProviderPass`
- Mis-matches in `selectPendingExec`

Search key terms:
- `startYieldingInputsToTheServer`
- `bidiAppend({requestId:A,appendSeqno`
- `ExecServerMessage`
- `ExecClientMessage`
- `ExecClientControlMessage`
- `InteractionQuery`
- `InteractionResponse`

## 4. Key Protocol Understanding for Local Mode

- `exec_server_message` / `interaction_query` are request downstream messages (server pending will not close without client reply, causing provider 400 on reconnect)
- `interaction_update` / `conversation_checkpoint_update` are notification downstream messages (for UI/live checkpoint sync, no client ACK required)

## 5. Investigation Priority for Local Mode

1. Confirm downstream `AgentServerMessage` category
2. If `exec_server_message`: check if client returned `ExecClientMessage` or `ExecClientControlMessage`
3. If `interaction_query`: check if client returned `InteractionResponse`
4. If `conversation_checkpoint_update`: check `pending_tool_calls` / `root_prompt_messages_json` / `turns`

## 6. Server Implementation Requirements

- Server must distinguish request downstreams vs notification downstreams
- Server must not model all `ServerMessage` as expecting a completion ACK
- Maintain pending in local state machine for request messages (`PendingExec`, `PendingInteraction`)
- Live reconnects under same backend process should inspect checkpoint / `pending_tool_calls`
- After backend restart, use `history/<conversationId>/state.json` + `history/<conversationId>/context.json` as persistent recovery truth.
