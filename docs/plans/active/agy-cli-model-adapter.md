# Execution Plan: Integrate AGY CLI As Cursor Model

Date: 2026-08-07

## Status

Active

## Outcome

Expose the installed Antigravity CLI (`agy.exe` 1.1.11) as a local Cursor model adapter with a verified headless JSONL contract. AGY owns authentication, conversation history, permissions and tool execution; Cursor receives assistant text, final status and read-only provider progress only.

## Context

- Repository workflow: `AGENTS.md` and `docs/WORKFLOW.md`.
- Adapter contract: `internal/backend/agent/model/types.go`.
- Existing provider pattern: Codex runtime, router, forwarder, config and UI.
- Verified executable: `C:\Users\Admin\AppData\Local\agy\bin\agy.exe`.
- Verified AGY app data: `C:\Users\Admin\.gemini\antigravity-cli`.

## Scope

In scope:

- Implement a Go AGY process manager using `--print --output-format stream-json` over ordinary pipes.
- Parse the verified `init`, `step_update`, `result` JSONL records strictly enough to reject unsupported shapes.
- Forward assistant `text_delta`, AGY-owned tool progress and final status without creating Cursor pending tools.
- Support bounded context cancellation, process cleanup, exact native conversation resume and compatibility checks.
- Integrate the adapter, router, config, runtime status, benchmark and minimal UI using existing Codex patterns.
- Replace the stale Python/SDK architecture and MVP documents with the CLI design.

Out of scope:

- Python SDK, localharness, PTY/ConPTY as the default transport, or a new API proxy.
- Cursor-native actionable tool calls, approvals or tool result delegation.
- Dynamic model discovery, plugin management, installation/update/auth UI.
- Repository test files; validation uses builds, runtime probes and manual E2E checks per the current project policy.

## Verified Runtime Contract

`stream-json` emitted multiple JSONL records in this order for a read-only directory probe:

1. `event=init`: `conversation_id`, `init.cwd`, `init.tools`, `init.permission_mode`.
2. `event=step_update`: user/system/agent response lifecycle records.
3. `event=step_update`, `step_type=tool`, `state=ACTIVE`: tool name and parameters.
4. `event=step_update`, `step_type=tool`, `state=DONE`: tool output.
5. `event=step_update`, `step_type=agent_response`, `text_delta`.
6. `event=result`: final `conversation_id`, `status`, `response`, duration, turns and usage.

The final `result.response` duplicates the text carried by `text_delta`. The `json` format emitted only the final result object. `--conversation <exact-id>` resumed the same native conversation successfully.

## Approach

1. Add an AGY runtime package for command construction, executable/version lookup, JSONL decoding, process lifecycle, cancellation, redaction and native resume metadata.
2. Add `AgyAdapter` and router dispatch. Select only the latest meaningful user prompt; do not replay Cursor tool definitions/results.
3. Map AGY tool lifecycle to provider-owned progress events or equivalent non-actionable status. Never emit Cursor pending tools because AGY already executes them.
4. Add `type: agy` config normalization, sanitized runtime status, benchmark and minimal model editor fields by reusing Codex surfaces.
5. Update docs, build the Go and frontend targets, then manually verify success, tool ownership, resume, timeout/cancel, workspace mismatch and credential redaction.

## Risks And Recovery

- Unsupported AGY schema: mark the runtime unavailable and return a sanitized error; do not heuristically map records.
- Process leak or late output: cancel the owned process, wait for exit and discard events after the active pass ends.
- Tool ownership mismatch: provider progress only; never create Cursor executable tool calls.
- Conversation collision: resume only when exact native ID, workspace, model and CLI version match.
- Authentication failure: expose readiness/error status and leave the adapter inactive; never read or copy credentials.

## Progress

- [x] Resolve process-visible authentication and run successful bounded `json` and `stream-json` probes.
- [x] Confirm tool event ordering, text delta/final duplication and exact conversation resume.
- [x] Select provider-owned progress as the event contract.
- [ ] Implement AGY runtime manager.
- [ ] Implement adapter, router and config integration.
- [ ] Add runtime status, benchmark and frontend configuration.
- [ ] Replace stale SDK/Python docs.
- [ ] Run focused builds and manual E2E validation.

## Decisions

- 2026-08-07: Runtime logs and stdout override stale SDK/Python design assumptions.
- 2026-08-07: Headless ordinary pipes are the default transport.
- 2026-08-07: AGY owns tool execution; Cursor receives provider-owned progress only.
- 2026-08-07: Do not use `--continue`; resume only with an exact native conversation ID and compatibility checks.
- 2026-08-07: Do not persist or modify AGY-owned cache; store only local recovery metadata if the existing history system requires it.

## Validation

- Completed: `agy.exe --help`, version 1.1.11, authenticated `stream-json` and `json` probes, tool lifecycle observation, duplicate-response observation and exact native resume.
- Pending: Go adapter build, frontend build, cancellation/timeout cleanup, workspace/model mismatch, runtime status and manual Cursor projection.

## Result

Complete after implementation and validation. Record verified limitations, especially provider progress being non-actionable and AGY remaining the sole tool executor, then move this plan to `docs/plans/completed/`.