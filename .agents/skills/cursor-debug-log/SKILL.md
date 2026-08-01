---
name: cursor-debug-log
description: Use when investigating Cursor local mode debug/log evidence - config.yaml log hot reload, history/<conversationId>/debug JSONL files, Bidi raw/decoded records, RunSSE records, runtime/provider debug records, debug file missing reasons, or explaining how debug files are generated and queried.
---

# Cursor Debug Log

Use this skill to explain and inspect the local debug log subsystem. The goal is to reconstruct what happened around a request without modifying installed Cursor client code or relying on legacy artifacts.

## Positioning & Scope

Debug logs form an optional evidence layer for local mode request pipelines. They are distinct from model-visible history:

- Answers "What did the client actually send?".
- Answers "How did the backend interpret/decode this request?".
- Answers "Which fields were attached to the active request?".
- Answers "What did the final provider request body look like?".
- Answers "What did RunSSE actually transmit to the client?".
- Do NOT treat debug logs as replay history, prompt inputs, or state facts.

The stable sources of truth remain:

- `history/<conversationId>/state.json`
- `history/<conversationId>/context.json`
- `history/usage.json`
- `logs/app.log`

Debug files complement these sources of truth with raw or near-raw pipeline evidence.

## Fixed Paths

- Assistant root directory: `~/.cursor-local-assistant-v2`
- Config file: `~/.cursor-local-assistant-v2/config.yaml`
- History root directory: `~/.cursor-local-assistant-v2/history`
- App log: `~/.cursor-local-assistant-v2/logs/app.log`
- Conversation debug directory: `history/<conversationId>/debug/`
- Orphan debug directory: `history/_debug/orphan/<requestId>/`

Enable debug logging via configuration:

```yaml
log: true
```

The current implementation checks `config.yaml` hot reloads using lightweight file snapshotting. After changing `log`, allow ~500ms before expecting next request events to reflect the new setting. Legacy binary versions may still require a restart.

## File Generation Pipeline

The debug layer writes to disk progressively as requests cross backend boundaries:

1. `BidiAppend` receives client upstream data.
   - Raw hex is written to `bidi.raw.jsonl`.
   - Decoded known-schema protobuf and backend-extracted intents are written to `bidi.decoded.jsonl`.
2. Forwarder converts decoded input into active runtime state.
   - Stream/request state decisions are written to `runtime.jsonl`.
3. Provider pass is prepared and executed.
   - Request summary, `model_call_id`, `provider_pass`, etc. prior to adapter are written to `provider.jsonl`.
   - Provider artifact callback appends final request/summary payload to `provider.jsonl`.
4. `RunSSE` streams backend outputs to client.
   - Sent messages, terminal events, send errors, disconnections, and heartbeats are written to `runsse.jsonl`.

If `conversationId` is unknown when early messages arrive, initial events land in `_debug/orphan/<requestId>/`. Once `conversationId` becomes known, subsequent events land in `history/<conversationId>/debug/`. Check both locations when reconstructing early or out-of-order requests.

## Debug File Semantics

`bidi.raw.jsonl`

- Direction: Client -> Backend.
- Contains `request_id`, optional `conversation_id`, `append_seqno`, `status`, raw `data_hex`.
- Inspect first when precise verification of client uploaded bytes is required.

`bidi.decoded.jsonl`

- Direction: Client -> Backend (post-protobuf decoding).
- Schema v2 contains complete known-schema `AgentClientMessage` protojson: `message`.
- Contains backend-extracted intent from upstream payload: `intent` (expands related proto sub-objects like `client_message`, `user_message`, `request_context`, `conversation_state`, exec/interaction/kv replies).
- Also contains indexing fields `message_case`, `requested_model`, `conversation_action` to facilitate search (these indices are search helpers, not full evidence bodies).
- Inspect when verifying how backend understood client request. For proof of raw client bytes, refer to `bidi.raw.jsonl`.
- Legacy binaries or old logs may only contain schema v1 summaries without expanded fields in `message` or `intent`.

`runtime.jsonl`

- Direction: Internal Backend Runtime.
- Contains state transitions and fields attached to active stream/request.
- Inspect when connecting decoded input to subsequent provider behavior.

`provider.jsonl`

- Direction: Backend -> Provider Adapter / Provider.
- Contains provider pass metadata, `model_call_id`, request knobs, final provider request artifact, provider summary artifact.
- Inspect when final outbound provider body or provider summary is key evidence.

`runsse.jsonl`

- Direction: Backend -> Client.
- Contains decoded `AgentServerMessage` dispatches, terminal events, send errors, disconnections, and heartbeats.
- Inspect backend attempts to return data to client. Note: represents decoded message evidence, not raw HTTP/SSE framing.

## Query Workflow

1. Determine ID type.
   - Check `history/<id>/state.json` to verify if ID is `conversationId`.
   - Search request/model-call/tool id in `history/*/{state.json,context.json}`, `history/usage.json`, `logs/app.log`.
2. After obtaining `conversationId`, list debug directory:
   - `ls -la "$HOME/.cursor-local-assistant-v2/history/<conversationId>/debug"`
3. If debug directory does not exist, check if debug logging was enabled when request occurred.
   - Read `config.yaml`.
   - Compare mtime across `config.yaml`, `state.json`, `context.json`.
   - Search `logs/app.log` for config hot reload or provider start records.
4. Read JSONL chronologically and correlate using:
   - `request_id`
   - `conversation_id`
   - `model_call_id`
   - `provider_pass`
   - `append_seqno`
   - Event timestamp
5. In final responses, summarize only required evidence fields. Do not paste secrets, API keys, full provider bodies, or large raw payloads.

Useful commands:

```bash
ROOT="$HOME/.cursor-local-assistant-v2"
REQ="<requestId>"
CONV="<conversationId>"

rg -n "$REQ" "$ROOT/history" "$ROOT/logs/app.log"
find "$ROOT/history" -path "*/debug/*" -type f | sort
rg -n "$REQ|model_call_id|provider_request_prepared|llm_request" "$ROOT/history/$CONV/debug"
```

Compact JSONL inspection:

```bash
jq -c 'select(.request_id == "<requestId>")' "$ROOT/history/$CONV/debug/provider.jsonl"
jq -c 'select(.request_id == "<requestId>") | {append_seqno, message_case, conversation_action, message, intent}' "$ROOT/history/$CONV/debug/bidi.decoded.jsonl"
```

## How to Use Evidence

Select target file based on problem type:

- Raw client upstream issues: check `bidi.raw.jsonl`.
- Known-schema client fields: check `bidi.decoded.jsonl.message` (e.g. `user_message.message_id`, selected images, conversation state bytes).
- Backend request comprehension: check `bidi.decoded.jsonl.intent`, then `runtime.jsonl`.
- Provider request issues: check `provider.jsonl`, especially `llm_request`.
- UI / streaming output issues: check `runsse.jsonl`.
- Request state issues: check `state.json`, `context.json`, `usage.json`, then corroborate with debug files.
- Missing debug logs: check `config.yaml`, mtime, app log, orphan debug directory.

For example, runtime model parameters (like thinking strength) are one form of provider request evidence:

- `bidi.raw.jsonl` shows what raw data client uploaded.
- `bidi.decoded.jsonl.message` shows decoded fields according to current known schema.
- `bidi.decoded.jsonl.intent` shows what backend extracted and prepared.
- `runtime.jsonl` shows what backend attached to request state.
- `provider.jsonl` shows what was prepared for provider.

Do not over-assert with plain history alone. For example, `reasoning_content` in `context.json` proves reasoning text was generated, but cannot independently prove which runtime parameter value caused it.

Note evidence boundaries:

- `bidi.decoded.jsonl` decodes using current known proto schema. Unknown fields or raw framing differences must be verified in `bidi.raw.jsonl`.
- `context.json` remains persistent history source of truth; debug files only prove what occurred during specific request pipelines.
- `provider.jsonl` provider body and `bidi.raw.jsonl` / `bidi.decoded.jsonl` can be large; only quote necessary fields without pasting full images, full bodies, or secrets.

## Missing Debug Files

If a request lacks debug files, state clearly "no direct debug log evidence". Common reasons:

- `log: false` when request occurred.
- Running binary version predates debug logging or hot reload implementation.
- Events occurred before conversation ID was resolved (recorded in `_debug/orphan/<requestId>/`).
- Request completed before enabling `log`.
- File write failure (check app log for warnings if available).

When debug evidence is missing, fall back to `state.json`, `context.json`, `usage.json`, `logs/app.log`, and label conclusions as inferences rather than direct proof.
