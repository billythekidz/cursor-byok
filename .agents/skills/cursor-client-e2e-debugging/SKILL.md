---
name: cursor-client-e2e-debugging
description: Use when debugging Cursor client agent/local-mode/tool/backend-store/provider-replay failures in this repo, especially after the state/context history-store refactor, when triaging installed app bundles read-only, correlating installed-client behavior with repo code, mapping a user-provided id to conversation/request/model-call evidence, replaying provider requests from debug logs, or locating the current client/backend/protocol/log files quickly.
---

Use this skill when users report Cursor agent, local mode, tool execution, protocol bridge, or client bundle issues, or when performing read-only validation of installed client behavior against repository code and logs.

Also use this skill when a user provides a single UUID / ID and wants to determine whether it represents a `conversationId`, `requestId`, `modelCallId`, `toolCallId`, or other runtime ID, and locate corresponding conversation history, provider execution status, or protocol logs.

Also use this skill when encountering provider 400 / parameter errors or SSE `event: error`, or when extracting the final provider body from `debug/provider.jsonl` to reproduce via standalone curl.

## Primary Constraints

- Do NOT modify installed Cursor client code, bundle, signature, or app copies.
- Reading, searching, comparing, and analyzing client bundles, logs, protocol events, and repo implementation is allowed and recommended.
- If `.cursor-app-formatted/` exists in this repository, prioritize reading this formatted snapshot to search and reference client bundle code. Read `/Applications/Cursor.app` raw files only if snapshot is missing, outdated, or requires hash verification.
- If the user asks to extract, format, refresh, or normalize Cursor.app snapshot workflows, use the `cursor-app-formatted` skill; read formatted code during investigation, but do NOT patch bundle code in the snapshot.
- If the user requests patching client for e2e, switch to read-only evidence gathering and diff localization without executing client modifications.
- The repository uses the `state.json + context.json` history-store architecture; do NOT use legacy `data.sqlite`, `conversation.json`, or `turns/<n>/request.json|sse.jsonl|summary.json` troubleshooting paths.

## Routing Judgment

- `history + logs` Trace Layer:
  - Symptom: User provides an ID to classify (`conversationId`, `requestId`, `modelCallId`, `toolCallId`) and requires tracing runtime status and semantic history from `history/<conversationId>/state.json` and `history/<conversationId>/context.json`.
  - Read [references/backend-store-log-tracing.md](references/backend-store-log-tracing.md) first.
- `provider replay / debug` Layer:
  - Symptom: Provider returns 400 / parameter error, SSE `event: error`, or user needs to verify if final outbound provider body can be reproduced via standalone curl.
  - Read [references/provider-replay-debugging.md](references/provider-replay-debugging.md) first, and use [scripts/provider-replay.sh](scripts/provider-replay.sh) if necessary.
- `cursor-agent` Layer:
  - Symptom: `CursorAgentProvider`, `ClaudeSDKClient`, `AnthropicProxy`, `registerAgentProvider`, `InteractionUpdate` mappings, or model bridge anomalies.
  - Read [references/file-map.md](references/file-map.md) and [references/search-patterns.md](references/search-patterns.md) first.
- `cursor-always-local` / Local Mode Protocol Layer:
  - Symptom: `BidiAppend`, `RunSSE`, `AgentServerMessage`, `ExecClientMessage`, `InteractionResponse`, live checkpoint, or pending closure anomalies.
  - Read [references/file-map.md](references/file-map.md) and [references/search-patterns.md](references/search-patterns.md) first.
- Installed Client Read-Only Localization Layer:
  - Symptom: Need to inspect installed app bundle, verify running copy, validate whether behavior matches, and determine if discrepancy originates from client or repo code.
  - Read [references/installed-client-readonly-validation.md](references/installed-client-readonly-validation.md) first.

If an issue spans multiple layers, start with the layer closest to the failure symptom rather than inspecting all paths simultaneously.

## Current Workflow

1. If user provides an ID, search `history/` directory, `context.json.items`, `state.json`, and `logs/app.log` to determine ID type; do not assume it is a `requestId`.
2. Once `conversationId` is obtained, inspect both sources of truth:
   - `history/<conversationId>/state.json`: Conversation metadata and current state (loop, token, current todo/plan, `latest_request_prefix`, `last_provider_call`).
   - `history/<conversationId>/context.json`: Append-only semantic history entries; prompt replay is projected from here by `ProjectPromptReplay()`.
3. Do not search for legacy provider invocation artifacts: `RecordLLMRequest` no longer writes `request.json`, `AppendLLMResponseChunk` is a no-op, and `RecordLLMSummary` only updates in-memory state and `state.latest_request_prefix` / usage.
4. Confirm whether failure lies in `cursor-agent`, `cursor-always-local`, or repository `internal/backend` protocol compatibility layer.
5. Use fixed search terms from references to locate entry functions, protocol messages, and bridge points quickly.
6. If provider returns 400 / parameter error, SSE `event: error`, or outbound provider body requires verification:
   - Read [references/provider-replay-debugging.md](references/provider-replay-debugging.md) first.
   - Trace `conversationId`, `requestId`, `modelCallId` from ID, then locate `history/<conversationId>/debug/provider.jsonl`.
   - Run [scripts/provider-replay.sh](scripts/provider-replay.sh) if needed; save replay artifacts only without writing API keys or full request bodies into skills/responses.
7. If user asks to check prefix cache / cache hit rate:
   - Run `go run ./scripts/historymetrics [conversationId|path]`
   - It reads current `history/<conversationId>/state.json` and `history/<conversationId>/context.json`, combining with `history/usage.json` statistics.
   - Focus on `cache_read_tokens / prompt_tokens_total`, and verify if `context.json.items` has missing, duplicate, or out-of-order entries.
8. If comparing installed app against repo behavior:
   - Perform read-only verification and evidence collection only; do NOT modify client bundle / app copy / signature.
   - Use `.cursor-app-formatted/` formatted copy to locate symbols, line numbers, and control flows; read raw `/Applications/Cursor.app` files only when verifying hashes or running copies.
   - Confirm active running app copy and target bundle path.
   - Read bundle content, logs, ports, and history state, and compare with repo implementation.
9. If evidence points to client bundle behavior difference:
   - Document specific file, symbol, log, and protocol evidence chain.
   - Determine if repository can accommodate, bypass, or output analytical conclusion.
   - Do NOT patch, re-sign, replace files, or execute write-based validations on installed Cursor client.

## Constraints

- Do NOT modify installed Cursor client code, bundle, signature, or app copies.
- Do NOT attempt to replicate entire Cursor backend by default; confirm if only model bridge layer requires changes.
- Do NOT assume provided ID is `requestId`; evaluate `conversationId`, `requestId`, `modelCallId`, `toolCallId`.
- Do NOT confuse `history/<conversationId>/state.json` and `history/<conversationId>/context.json`: state is metadata and current status; context is replayable semantic history.
- Do NOT rely on `agent_request_runs`, `agent_conversations`, `agent_history_entries`, `protocol_traces`, or `data.sqlite`; DB-backed store / trace debug UI is deprecated.
- Do NOT embed transient troubleshooting logs, temporary conclusions, or one-off request_ids / ports / tokens into skills.
- Keep only stable workflows, fixed entry points, reusable search terms, and read-only validation rules in skills.

## Model Channel Rules

- Model channel uniqueness is not determined by `modelID`.
- The normalized channel ID is a short `SHA-256` hash (first 16 hex characters) of `baseURL + modelID + apiKey + displayName + openAIEndpoint`.
- Resolver remains compatible with legacy channel IDs: `baseURL + modelID + apiKey + displayName`.
- `modelID` represents provider model only; when inspecting selectors, default models, and hit channels, prioritize channel ID and `openAIEndpoint`.

## Reference Loading Rules

- `history + logs` path, ID lookup, state/context generation pipeline: read [references/backend-store-log-tracing.md](references/backend-store-log-tracing.md)
- Provider 400/parameter error, SSE `event: error`, outbound provider body curl replay: read [references/provider-replay-debugging.md](references/provider-replay-debugging.md)
- File map: read [references/file-map.md](references/file-map.md)
- Search terms and decision tree: read [references/search-patterns.md](references/search-patterns.md)
- Installed client read-only verification, process confirmation, behavior validation: read [references/installed-client-readonly-validation.md](references/installed-client-readonly-validation.md)

## Included Scripts

- Prefix cache / cache hit statistics: run `go run ./scripts/historymetrics [conversationId|path]`
- Compatibility wrapper script: run [scripts/cache-hit-rate.mjs](scripts/cache-hit-rate.mjs)
- Provider curl replay: run [scripts/provider-replay.sh](scripts/provider-replay.sh) (requires `REQUEST_LOG`, `REQUEST_ID`, `MODEL_CALL_ID`)
