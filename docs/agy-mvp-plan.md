# AGY adapter MVP plan

## Goal

Expose the Antigravity SDK as a first-class `ModelAdapter` with `type: agy`.
Cursor sends only the latest user prompt. The Antigravity agent owns its
conversation history, shell/file/MCP tools, approvals and tool execution.
Cursor receives only assistant text deltas and turn completion metadata.

The MVP uses a small Python bridge around `google.antigravity.Agent`, launched
and supervised by Go. The bridge is not an OpenAI proxy and never forwards
Cursor tool definitions or provider tool calls back to Cursor.

## Scope

- Detect the Python runtime and the installed `google-antigravity` package.
- Start one long-lived AGY bridge process per runtime profile over stdio JSONL.
- Configure `Agent`/`LocalAgentConfig` with the selected model, workspace and
  Antigravity authentication environment.
- Map one Cursor conversation and workspace to one persistent AGY session.
- Send only the latest non-empty Cursor user prompt to AGY.
- Stream only final assistant text as `ModelEventKindTextDelta`.
- Consume AGY-internal tool activity without exposing it as Cursor tool calls.
- Support cancellation, turn serialization, bridge restart and session resume.
- Advertise AGY models only when the SDK runtime is available and configured.

## Configuration contract

```json
{
  "type": "agy",
  "displayName": "Antigravity Gemini",
  "modelID": "gemini-3.6-flash",
  "active": true
}
```

`baseURL`, `apiKey`, OpenAI/Anthropic endpoint fields and custom headers are
ignored for AGY and normalized away. Authentication remains in the AGY SDK
environment: for example `GEMINI_API_KEY`, Vertex/ADC configuration, or a
future explicitly supported runtime profile. BYOK must never read, exchange or
return provider credentials.

Model identifiers must be supplied by the configured Antigravity runtime or
explicit user configuration; the adapter must not infer model names. A model
change creates a new session mapping rather than changing the model of a live
session.

The model editor must show that AGY owns tools and that Cursor receives
text-only output. `reasoningEffort`, provider extra parameters and Cursor tool
configuration are not part of the MVP contract.

## Runtime setup

1. Locate Python from the configured runtime profile or `PATH` and run a
   version check.
2. Import `google.antigravity` and verify that the platform wheel's compiled
   `localharness` binary is available. The SDK source checkout alone is not
   sufficient to run the runtime.
3. Check authentication without logging secrets. Classify only ready,
   unauthenticated, missing package/binary and runtime error states.
4. Start the bridge with a clean, profile-specific environment and workspace
   configuration.
5. Initialize or resume the AGY session before sending the first turn.

The bridge owns an `Agent` session and uses `agent.chat(prompt)`. It iterates
the response's text stream. Internal `thoughts` and `tool_calls` streams are
consumed or logged only as redacted operational status; they are never mapped
to Cursor provider events.

## Bridge protocol subset

The bridge uses newline-delimited JSON over stdin/stdout. Logs go to stderr and
must be redacted. The Go runtime manager owns process lifecycle, request IDs,
timeouts and cancellation.

Requests:

- `initialize`: runtime version, profile and capability handshake
- `session/start`: AGY session ID, model, workspace and resume information
- `turn/start`: latest user prompt only
- `turn/cancel`: cancel the active SDK response and internal tool work
- `session/close`: release the session and exit cleanly

Events:

- `text_delta`: final assistant text fragment
- `turn_finished`: final status and optional usage metadata
- `runtime_status`: non-secret progress such as internal tool activity
- `error`: sanitized runtime/provider error

There is deliberately no `tool_call`, `tool_result`, `reasoning_delta` or
Cursor approval message in this protocol. AGY's own tool loop must finish
before `turn_finished` is emitted.

## Request and persistence rules

The Go adapter selects the latest meaningful Cursor user message. System
messages, previous assistant messages, Cursor tool definitions and Cursor tool
results are not replayed into AGY. AGY's persisted conversation is the source
of truth for its own prior turns.

The mapping is stored beside the conversation history as
`history/<conversationId>/agy.json`:

```json
{
  "provider": "agy",
  "modelID": "gemini-3.6-flash",
  "agyConversationID": "agy_...",
  "workspace": "D:\\workspace",
  "runtimeProfile": "default",
  "sdkVersion": "0.1.9"
}
```

Reuse requires matching model, workspace and runtime profile. The adapter must
serialize turns per AGY session; concurrent `Agent.chat()` calls on one
conversation are not allowed. A failed resume starts an isolated new session
and does not mix its history with the old session.

The bridge may use the SDK's `conversation_id`, `save_dir` and session resume
features. The mapping file is local recovery metadata only and is never added
to model-visible prompt history.

## Verification

- `go build .` and targeted `go build ./internal/...` packages.
- `npm run build` in `frontend/` after adding the `agy` configuration surface.
- Manual smoke checks: missing Python/package/binary, unauthenticated runtime,
  normal text streaming, an AGY-internal file/tool action, follow-up resume,
  cancellation during tool execution, bridge restart, workspace mismatch and
  model switch.
- Confirm Cursor receives no tool-call or reasoning events and does not create
  Cursor-owned pending tool executions for AGY turns.
- Inspect redacted bridge stderr and runtime artifacts; no API key, OAuth token
  or credential payload may appear in UI, JSONL or logs.

No repository test files are added for this MVP because the current repository
policy forbids adding tests.

## Deferred work

Cursor/AGY tool delegation, interactive approval bridging, dynamic model
discovery, multiple runtime profiles in one session, remote bridge transports,
model-specific reasoning controls, native multimodal input mapping and
cross-conversation session sharing remain post-MVP.
