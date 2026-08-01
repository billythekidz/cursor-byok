# Installed Client Read-Only Inspection & Validation

Primary Rule:

- Do NOT modify installed Cursor client code, bundle, signature, or app copies.
- Reading, searching, comparing, and analyzing client bundles, logs, ports, and history state is allowed and recommended.
- The goal is to isolate discrepancies, gather evidence, and assign ownership, NOT to patch the client.

## 1. Confirm Active Running App Copy

Verify running processes using non-interactive commands first:

```bash
pgrep -fal 'Cursor Hooked|Cursor Patched|/Contents/MacOS/Cursor'
ps -axo pid,ppid,command | rg 'Cursor(.app)?/Contents/MacOS/Cursor|extension-host'
```

Do not draw conclusions or modify client files before confirming which app copy is actually running.

## 2. Read-Only Localization of Target Bundles & Key Files

Locate and read these files without modifying them:

```bash
ls -l "/absolute/path/Target.app/Contents/Resources/app/extensions"
shasum -a 256 "/absolute/path/Target.app/Contents/Resources/app/extensions/cursor-always-local/dist/main.js"
```

Focus areas:

- `out/vs/workbench/workbench.desktop.main.js`
- `out/vs/workbench/api/node/extensionHostProcess.js`
- `cursor-always-local/dist/main.js`
- `cursor-always-local/dist/gitWorker.js`
- `cursor-agent-exec/dist/main.js`
- `cursor-agent-exec/dist/*.js`
- `cursor-agent-worker/dist/main.js`

In current installations, legacy path `cursor-agent/dist/main.js` usually does not exist; verify actual extension names and `dist/` files under `extensions/` first.

## 3. Read-Only Inspection of Bundle Content

Common search keywords:

```bash
rg -n 'BidiTransport|ExecClientMessage|InteractionResponse|conversation_checkpoint_update' "/absolute/path/Target.app/Contents/Resources/app/extensions/cursor-always-local/dist/main.js"
rg -n 'CursorAgentProvider|AnthropicProxy|ANTHROPIC_BASE_URL|InteractionUpdate|checkpoint|agent window' "/absolute/path/Target.app/Contents/Resources/app/extensions/cursor-agent-exec/dist/main.js" "/absolute/path/Target.app/Contents/Resources/app/extensions/cursor-agent-exec/dist"/*.js "/absolute/path/Target.app/Contents/Resources/app/extensions/cursor-agent-worker/dist/main.js"
rg -n 'agent window|open_agent_window|NameAgent|UpdateConversationMetadata|shouldShowAgentWindowTitleHelperText' "/absolute/path/Target.app/Contents/Resources/app/out/vs/workbench/workbench.desktop.main.js" "/absolute/path/Target.app/Contents/Resources/app/extensions/cursor-agent-exec/dist"/*.js
```

Use file viewing tools to inspect target code fragments as needed without modifying bundles.

When comparing against repository implementation, open side by side:

- `proto/agent_v1.proto`
- `proto/aiserver_v1.proto`
- `internal/backend/...`
- `internal/runtime/local_runtime.go`

## 4. Validate Whether Behavior Hits Target Copy

Verify at least two of the following:

- Process path matches target app
- Target extension host process is active
- Local listening port exists
- `~/.cursor-local-assistant-v2/logs/app.log` updates
- `~/.cursor-local-assistant-v2/history/<conversationId>/state.json` / `context.json` updates
- Request/protocol events cross the bundle files being analyzed

Common verification commands:

```bash
pgrep -fal '/absolute/path/Target.app/Contents/MacOS/Cursor'
lsof -nP -iTCP -sTCP:LISTEN | rg 'Cursor|127.0.0.1'
```

## 5. Record Evidence & Output Attribution

If a discrepancy is confirmed between installed app behavior and repository logic, document:

1. Active running app path
2. Matched bundle file paths and key symbols
3. Corresponding logs, ports, `history/state.json`, `history/context.json`, `usage.json` evidence
4. Location of corresponding implementation in repository

If the conclusion attributes the issue to the client side, output analysis and attribution without attempting patches, re-signing, file replacements, or write-based validations.
