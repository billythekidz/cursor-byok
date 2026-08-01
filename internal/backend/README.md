# Backend Architecture

`internal/backend` currently supports local assistant mode and direct upstream mode.

For the phase-1 research document on the backend agent's "minimum fact set", see [`../../docs/backend-agent-minimum-facts-phase1.md`](../../docs/backend-agent-minimum-facts-phase1.md).

Core boundaries:

- `server`
  - Local HTTP/Connect entry layer
  - Handles routing, middleware, error encoding, and a small amount of local mocking
- `forwarder`
  - Local protocol compatibility and LLM forwarding core
  - Handles `BidiAppend`, `RunSSE`, history JSON, prompt compilation, provider streaming calls, and broadcasting
- `host`
  - The single assembly point
  - Responsible for wiring up `server/config.Manager`, `forwarder.Module`, and the root routes

The current implementation no longer supports:

- Pro / `cursor-byok`
- HTTP/protocol trace debug UI
- DB-backed store, conversation index, and searchable conversation memory

## Directory Structure

```text
internal/backend/
  README.md
  host.go

  server/
    context.go
    errors.go
    local.go
    middleware.go
    policy.go
    route.go
    url.go

    config/
      manager.go
      store.go
      types.go
      legacy_runtime.go
      resolver.go

    upstream/
      action.go
      mocks.go
      types.go

  forwarder/
    artifacts.go
    broker.go
    compiler.go
    events.go
    file_store.go
    legacy_stream.go
    module.go
    projector.go
    provider.go
    reminders.go
    service.go
    tool_catalog.go
    types.go

  agent/
    bridge/
      exec/
        bridge.go
      interaction/
        bridge.go

    core/
      types.go

    model/
      router.go
      openai.go
      anthropic.go
      artifacts.go
      provider_limits.go
      http_error.go
      types.go

    prompt/
      engine.go
      replay.go

    protocol/
      inbound.go
```

## Persistence Layout

The assistant directory is fixed at:

- `~/.cursor-local-assistant-v2/config.yaml`
- `~/.cursor-local-assistant-v2/data/ca.crt`
- `~/.cursor-local-assistant-v2/data/ads/`
- `~/.cursor-local-assistant-v2/history/`
- `~/.cursor-local-assistant-v2/logs/`

Conventions:

- `config.yaml` is the user configuration
- `data/ca.crt` is the CA certificate injected into the host
- `data/ads/` is the ad package and resource cache directory
- `history/` is the directory for conversation facts and global usage JSON, and is not part of the logs
- `logs/` only keeps the necessary text runtime logs

Current `history/` directory layout:

```text
history/
  usage.json
  <conversation_id>/
    state.json
    context.json
    conversation.lock
```

`state.json` only expresses the current loop state and persistent memory; it does not save history content that can be projected to the LLM. Current loop status semantics:

- `idle`: no loop is currently in progress.
- `running`: the current turn's input or intermediate context has been persisted, and model advancement is being awaited/initiated.
- `waiting_tool`: the full tool call has been persisted and we are waiting for the tool result.
- `completed`: the current turn finished normally.
- `canceled`: the current turn was canceled and produces no assistant output.
- `provider_error`: the provider/LLM call failed; the error is recorded as a context tag.
- `failed`: local internal failure, e.g., projection, persistence, usage JSON write, or bridge intake failure; it is not the same as a provider error.

## Request Flow

1. Requests enter the backend root route.
2. `PolicyMiddleware` selects the local or upstream branch based on `routing.mode` and `X-Server-Upstream-URL`.
3. `BidiAppend` / `RunSSE` enters the `forwarder`.
4. The `forwarder` first writes the current loop state to `state.json`, then appends the semantic events that occurred to `context.json`.
5. The prompt sent to the LLM is projected only from `context.json`; `state.json` does not save projectable history.
6. Provider usage/cache and aggregate statistics are written to `history/usage.json`, not scanned on the fly from conversation files.
7. `checkpoint` only represents live state within the same backend process.

## Model Channels

- Users fill in `displayName`, `baseURL`, `apiKey`, and `modelID` in the config
- The runtime channel's unique ID is no longer determined by `modelID`
- The current unique ID is a short `SHA-256` hash of `url + modelID + key + name` (first 16 hex characters)
- `modelID` only denotes the provider model