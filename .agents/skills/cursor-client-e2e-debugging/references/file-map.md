# File Map

## Installed Client Bundle

Prioritize inspecting these running client bundle files:

- `/Applications/Cursor.app/Contents/Resources/app/out/vs/workbench/workbench.desktop.main.js`
- `/Applications/Cursor.app/Contents/Resources/app/out/vs/workbench/api/node/extensionHostProcess.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-always-local/dist/main.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-always-local/dist/gitWorker.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent-exec/dist/main.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent-exec/dist/*.js`
- `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent-worker/dist/main.js`

In current installations `cursor-agent` is split into bundles; legacy path `/Applications/Cursor.app/Contents/Resources/app/extensions/cursor-agent/dist/main.js` usually does not exist. Do not reach conclusions using legacy paths; list `extensions/` first to confirm active `cursor-agent-exec`, `cursor-agent-worker`, `cursor-always-local` and their `dist/` files.

General Roles:

- `out/vs/workbench/workbench.desktop.main.js`: Main UI, agent window / titlebar, feature flags, user clickable controls, read-only / disabled states.
- `cursor-always-local/dist/main.js`: Local mode protocol, `BidiAppend`, `RunSSE`, `AgentServerMessage` / `AgentClientMessage` bridge.
- `cursor-agent-exec/dist/main.js` and numeric chunks: Agent execution side, SDK/canvas runtime, tool execution, proto message definitions and split chunks. (In current builds `411.js` may contain key agent execution fragments, but chunk numbers are unstable; search `dist/*.js` first).
- `cursor-agent-worker/dist/main.js`: Agent worker side background logic.

Other app copies may exist on user machine, e.g.:

- `~/Applications/Cursor Hooked.app`
- `/Applications/Cursor Patched.app`

Do not assume which copy is running; check running process path first.

## Current Backend / Store & History

Assistant root directory on user machine:

- `~/.cursor-local-assistant-v2/`

Key locations:

- `~/.cursor-local-assistant-v2/config.yaml`
- `~/.cursor-local-assistant-v2/data/ca.crt`
- `~/.cursor-local-assistant-v2/data/ads/`
- `~/.cursor-local-assistant-v2/history/usage.json`
- `~/.cursor-local-assistant-v2/history/<conversationId>/state.json`
- `~/.cursor-local-assistant-v2/history/<conversationId>/context.json`
- `~/.cursor-local-assistant-v2/history/<conversationId>/conversation.lock`
- `~/.cursor-local-assistant-v2/logs/app.log`

Roles:

- `state.json`: Conversation metadata, loop state, latest provider/request prefix, current todos/plans, token/compaction state.
- `context.json.items`: Append-only semantic history, source of truth for prompt replay.
- `usage.json`: Global provider call / turn usage aggregation.
- `conversation.lock`: Conversation-level file lock.
- Checkpoints represent live state within single backend process; not persistent recovery sources of truth.
- Legacy artifacts (`conversation.json`, `entries.jsonl`, `turns/`, `request.json`, `summary.json`, `sse.jsonl`, `replay.json`, `runtime.json`, `latest.json`, numeric turn directories) are cleaned automatically by history maintenance.

Generation entry points:

- `internal/appdata/paths.go`
- `internal/backend/host.go`
- `internal/backend/README.md`
- `internal/backend/forwarder/file_store.go`
- `internal/backend/forwarder/history_maintenance.go`
- `internal/backend/forwarder/usage_store.go`
- `internal/backend/forwarder/token_usage.go`
- `internal/backend/forwarder/artifacts.go`

## Protocol & Local Mode Implementation in Repository

Protocol definitions:

- `proto/agent_v1.proto`
- `proto/aiserver_v1.proto`
- `proto/from_extensions/agent_v1.proto`
- `proto/from_extensions/aiserver_v1.proto`

Extension snapshot and extraction:

- `proto/extensions-cursor-app/cursor-always-local/package.json`
- `proto/extract_extensions_proto.sh`
- `proto/ext_tool/main.go`

Local backend entry points:

- `internal/backend/host.go`
- `internal/backend/server/route.go`
- `internal/backend/server/policy.go`
- `internal/backend/server/local.go`
- `internal/backend/server/config/types.go`
- `internal/backend/server/config/manager.go`
- `internal/backend/server/config/resolver.go`

Forwarder main pipeline:

- `internal/backend/forwarder/module.go`
- `internal/backend/forwarder/service.go`
- `internal/backend/forwarder/actor.go`
- `internal/backend/forwarder/broker.go`
- `internal/backend/forwarder/events.go`
- `internal/backend/forwarder/compiler.go`
- `internal/backend/forwarder/projector.go`
- `internal/backend/forwarder/provider.go`
- `internal/backend/forwarder/checkpoint_memory.go`
- `internal/backend/forwarder/runtime_summary.go`

Protocol decoding:

- `internal/backend/agent/protocol/inbound.go`

Execution bridge / Interaction bridge:

- `internal/backend/agent/bridge/exec/bridge.go`
- `internal/backend/agent/bridge/interaction/bridge.go`

Model adaptation:

- `internal/backend/agent/model/router.go`
- `internal/backend/agent/model/openai.go`
- `internal/backend/agent/model/anthropic.go`
- `internal/backend/agent/model/artifacts.go`
- `internal/backend/agent/model/http_error.go`
- `internal/backend/agent/model/tool_call_id.go`
- `internal/modelchannel/identity.go`
- `internal/runtime/local_runtime.go`

Prompt / replay:

- `internal/backend/agent/prompt/engine.go`
- `internal/backend/agent/prompt/replay.go`
- `internal/backend/agent/prompt/content_parts.go`
- `internal/backend/forwarder/prompt_context.go`
- `internal/backend/forwarder/request_context.go`
- `internal/backend/forwarder/reminders.go`
- `internal/backend/forwarder/prompt_guard.go`

## Build References (Read-Only)

Build and code signing reference files in repository:

- `Taskfile.yml`
- `build/darwin/Taskfile.yml`
- `build/dmg-extras/FixDamagedApp.command`

Key sections:

- `codesign:adhoc` in `build/darwin/Taskfile.yml`
- `xattr -cr` in `build/dmg-extras/FixDamagedApp.command`
