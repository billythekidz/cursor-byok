# AGY adapter architecture

## Component diagram

```mermaid
flowchart LR
    Cursor[Cursor Client] -->|latest user prompt| Proxy[Cursor BYOK Forwarder]
    Proxy --> Router[Model Adapter Router]
    Router --> AgyAdapter[AGY Model Adapter]
    AgyAdapter --> Manager[AGY Runtime Manager]
    Manager -->|JSONL stdio| Bridge[Python AGY Bridge]
    Bridge --> SDK[google.antigravity Agent]
    SDK --> Harness[localharness runtime]
    Harness --> Tools[AGY shell/file/MCP tools]
    Harness --> Workspace[User Workspace]
    SDK --> Auth[SDK authentication environment]
    AgyAdapter -->|text deltas only| Proxy
```

The Go adapter remains responsible for Cursor's provider lifecycle. The Python
bridge is responsible for translating the narrow prompt/text contract into the
Antigravity SDK's async agent API. AGY remains the only owner of its tools and
agent loop.

## Request sequence

```mermaid
sequenceDiagram
    participant C as Cursor
    participant F as Forwarder
    participant A as AgyAdapter
    participant M as RuntimeManager
    participant B as Python Bridge
    participant G as AGY SDK
    participant W as Workspace

    C->>F: Stream model request with Cursor transcript
    F->>A: Stream(StreamRequest)
    A->>A: select latest user prompt
    A->>M: get or create session(conversation, model, workspace)
    M->>B: initialize / session/start or resume
    A->>B: turn/start(prompt)
    B->>G: agent.chat(prompt)
    G->>W: execute AGY-owned tools
    G-->>B: final text stream
    B-->>A: text_delta
    A-->>F: ModelEvent text_delta
    F-->>C: Cursor-compatible text SSE
    G-->>B: turn complete
    B-->>A: turn_finished
    A-->>F: TurnFinished
    F-->>C: Finish
```

Tool calls, tool results and internal thoughts stay between the SDK and AGY
runtime. They must not enter Cursor's `context.json`, provider replay or
pending-tool state.

## Runtime state machine

```mermaid
stateDiagram-v2
    [*] --> Unknown
    Unknown --> NotReady: runtime check
    Unknown --> Ready: Python/package/binary/auth ready
    NotReady --> Ready: setup/profile fixed
    Ready --> Starting: bridge start
    Starting --> Idle: handshake and session ready
    Starting --> Error: process/auth failure
    Idle --> Running: turn/start
    Running --> Idle: turn_finished
    Running --> Canceling: client cancellation
    Canceling --> Idle: cancellation acknowledged
    Running --> Error: bridge/provider failure
    Idle --> Error: bridge exit
    Error --> Starting: restart or isolated resume
```

The runtime manager serializes turns for each session. A bridge process exit
must fail the active turn, close the current provider pass and prevent late
text events from being attached to a later Cursor turn.

## Conversation/session mapping

```mermaid
flowchart TD
    Conversation[Cursor Conversation ID]
    Conversation --> Mapping[history/<id>/agy.json]
    Mapping --> Session[AGY Conversation ID]
    Session --> Workspace[Workspace Path]
    Session --> Model[AGY Model ID]
    Session --> Profile[Runtime Profile]
    Profile --> Python[Python Runtime]
    Profile --> SDK[google-antigravity SDK]
    SDK --> Saved[SDK save_dir/session state]
```

The mapping is conversation-owned. The adapter must pass a stable AGY session
identity to the bridge and use the same workspace on resume. If the current
Cursor request does not provide a usable workspace or conversation identity,
the adapter must fail clearly or use an explicitly configured fallback; it must
not silently merge unrelated conversations.

## Data boundaries

```mermaid
flowchart LR
    CursorTranscript[Cursor transcript] -->|latest user prompt only| AgyTurn[AGY turn/start]
    AgyTurn --> AgyHistory[AGY SDK conversation history]
    AgyHistory -->|internal| AgyTools[AGY tools]
    AgyHistory -->|final text delta| CursorStream[Cursor stream lifecycle]
    AgyMapping[agy.json mapping] -. recovery metadata .-> AgyTurn
    Credentials[SDK auth environment] -. never copied .-> Cursor
```

`state.json` and `context.json` remain the Cursor forwarder's recovery truth,
but AGY tool activity is not appended there as Cursor tool activity. The
`agy.json` mapping only connects the two session identities and does not become
part of the model prompt.

## Failure and cancellation boundaries

- A provider error from the SDK becomes one sanitized `ProviderError`; raw
  traceback and credentials stay on redacted stderr.
- Cancellation propagates from the Go request context to `turn/cancel`, then to
  the SDK response and its internal tool work.
- Text arriving after cancellation, `[DONE]` or a failed provider pass is
  discarded by pass/session identity.
- A resume failure creates a new AGY session mapping; it never reuses an
  uncertain session with a different workspace or model.
- AGY-owned tool failures remain inside the AGY turn unless the SDK terminates
  the turn with an error. Cursor should see only the resulting text or final
  provider error.

## MVP boundary

```mermaid
flowchart LR
    AgyTools[AGY-owned shell/file/MCP tools] --> MVP[Included in MVP]
    CursorTools[Cursor-owned tool calls] -. not forwarded .-> Deferred[Deferred]
    Thoughts[AGY reasoning/thought stream] -. not forwarded .-> Deferred
    Multimodal[Native multimodal payload mapping] -. deferred .-> V2[Post-MVP]
```

The key invariant is one owner for the agent loop: AGY receives the prompt and
performs its own work; Cursor displays the resulting text stream and does not
attempt to execute or approve AGY tools.
