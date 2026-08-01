# Provider Replay & Debugging

Read this reference when experiencing provider errors, SSE `event: error`, or when verifying whether outbound provider body can be reproduced independently.

This workflow reconstructs "the final request shape sent by backend to provider" and "provider's actual response". It is not semantic history, nor is it a client input source of truth.

## Evidence Boundaries

- `history/<conversationId>/debug/provider.jsonl`
  - Closest to outbound provider boundary.
  - `payload.body` in `event=llm_request` is the final provider request body.
  - Use it to run curl replay and verify if issues exist in outbound request shape.
- `history/<conversationId>/state.json`
  - Current status and recent provider call summary (`latest_request_prefix`, `last_provider_call`).
- `history/<conversationId>/context.json`
  - Replayable semantic history. Explains why prompt was formed, not for replaying HTTP requests.
- `history/<conversationId>/debug/bidi.raw.jsonl`
  - Raw client uploaded bytes evidence.
- `history/<conversationId>/debug/bidi.decoded.jsonl`
  - Decoded client upstream evidence according to known schema.
- `history/<conversationId>/debug/runtime.jsonl`
  - Backend fields attached to active request / stream.
- `history/<conversationId>/debug/runsse.jsonl`
  - Messages backend attempted to send back to client.

Do not mix evidence types: provider replay only proves final provider HTTP request and response; it does not prove what client originally uploaded, nor does it replace semantic history in `context.json`.

## Prerequisites

- `log: true` in `config.yaml` when request occurred, or existing `history/<conversationId>/debug/provider.jsonl`.
- Already obtained via ID lookup:
  - `conversationId`
  - `requestId`
  - `modelCallId`
- Confirmed target is Anthropic-compatible `/v1/messages` request.

If debug files are missing, fall back to `state.json`, `context.json`, `usage.json`, `logs/app.log` for inference, and state clearly "no direct provider body evidence".

## Minimal Workflow

1. Locate provider debug file:

```bash
ROOT="$HOME/.cursor-local-assistant-v2"
CONV="<conversationId>"
REQ="<requestId>"
MODEL_CALL="<modelCallId>"
REQUEST_LOG="$ROOT/history/$CONV/debug/provider.jsonl"
```

2. Confirm `llm_request` exists:

```bash
jq -c --arg req "$REQ" --arg mc "$MODEL_CALL" '
  select(.event == "llm_request" and .request_id == $req and .model_call_id == $mc)
  | {at, conversation_id, request_id, model_call_id, provider: .payload.provider, model: .payload.body.model}
' "$REQUEST_LOG"
```

3. Run generic replay script:

```bash
REQUEST_LOG="$REQUEST_LOG" \
REQUEST_ID="$REQ" \
MODEL_CALL_ID="$MODEL_CALL" \
CHANNEL_NAME="GLM" \
OUT_DIR="/tmp/cursor-provider-replay-$REQ" \
.agents/skills/cursor-client-e2e-debugging/scripts/provider-replay.sh
```

Or pass provider config directly to avoid reading `config.yaml`:

```bash
REQUEST_LOG="$REQUEST_LOG" \
REQUEST_ID="$REQ" \
MODEL_CALL_ID="$MODEL_CALL" \
BASE_URL="<provider-base-url>" \
API_KEY="<provider-api-key>" \
OUT_DIR="/tmp/cursor-provider-replay-$REQ" \
.agents/skills/cursor-client-e2e-debugging/scripts/provider-replay.sh
```

4. Inspect artifacts:

- `request.body.json`: Extracted final provider body.
- `response.headers`: HTTP response headers.
- `response.sse`: SSE response body.
- `replay.meta.json`: IDs and provider log paths used in replay.

## Result Analysis

- `curl_exit_code != 0`: Network, TLS, timeout, connection, or local curl issue. Check stderr and `response.headers`.
- HTTP non-2xx: Provider gateway or authentication rejected request. Inspect `response.headers` and provider error body.
- HTTP 2xx with SSE `event: error`: Provider accepted connection, but parameter validation failed or model side rejected. Compare `request.body.json` message structure, tool schema, thinking/reasoning parameters, model name, and endpoint compatibility.
- SSE normal streaming output: Outbound provider request shape is valid. If client still fails, inspect `runsse.jsonl`, forwarder state machine, or client protocol layer.

## Common Resolution Directions

On provider parameter error, check:

- `model`: Supported name by target endpoint.
- `messages`: Anthropic-compatible format compliance.
- `system`: Supported by target provider, or needs message conversion.
- `tools` / `tool_choice`: Target provider dialect compliance.
- `thinking` / `reasoning`: Supported by target provider.
- Images, files, cache_control, metadata: Supported within provider compatibility bounds.
- `max_tokens`, `temperature`, `top_p`, `stop_sequences`: Within allowed ranges.

## Sensitive Information Rules

- Do NOT write API keys into skills, references, script defaults, or commits.
- When responding to users, do NOT paste full API keys; state "using key provided by user / config key".
- Do NOT paste large sections of full `request.body.json` to users; quote only relevant field shapes.
- Do NOT embed transient `conversationId/requestId/modelCallId` into skill docs.
- Save temporary replay artifacts to `/tmp` by default; document path and purpose if retained.

## Script Parameters

`scripts/provider-replay.sh` parameters controlled via environment variables:

- Required: `REQUEST_LOG`, `REQUEST_ID`, `MODEL_CALL_ID`
- Optional: `BASE_URL`, `API_KEY`, `GLM_BASE_URL`, `GLM_API_KEY`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY`, `CONFIG_FILE` (default `~/.cursor-local-assistant-v2/config.yaml`), `CHANNEL_NAME` (default `GLM`), `OUT_DIR` (default `/tmp/cursor-provider-replay-<requestId>`), `MAX_TIME` (default `240`), `ENDPOINT_PATH` (default `/v1/messages`).

Script produces 4 stable artifacts:

- `request.body.json`
- `response.headers`
- `response.sse`
- `replay.meta.json`