# Codex adapter architecture

## Component diagram

```mermaid
flowchart LR
    Cursor[Cursor Client] -->|AI request| Proxy[Cursor BYOK Forwarder]
    Proxy --> Router[Model Adapter Router]
    Router --> CodexAdapter[Codex Model Adapter]
    CodexAdapter --> Manager[Codex Runtime Manager]
    Manager --> AppServer[codex app-server]
    AppServer --> CodexCore[Codex Core]
    CodexCore --> Workspace[User Workspace]
    CodexCore --> Auth[CODEX_HOME Auth]
    Dashboard[BYOK Dashboard] --> Setup[Codex Setup Service]
    Setup --> CLI[codex CLI]
    Setup --> Auth
```

## Request sequence

```mermaid
sequenceDiagram
    participant C as Cursor
    participant F as Forwarder
    participant A as CodexAdapter
    participant M as RuntimeManager
    participant S as Codex app-server
    participant W as Workspace

    C->>F: Stream model request
    F->>A: Stream(StreamRequest)
    A->>M: Get or create runtime
    M->>S: initialize / initialized
    A->>S: thread/start or thread/resume
    A->>S: turn/start
    S->>W: Execute Codex tools
    S-->>A: message/reasoning/tool notifications
    A-->>F: ModelEvent deltas
    F-->>C: Cursor-compatible stream
    S-->>A: turn/completed
    A-->>F: TurnFinished
    F-->>C: Finish
```

## Runtime state machine

```mermaid
stateDiagram-v2
    [*] --> Unknown
    Unknown --> NotInstalled: status check
    Unknown --> InstalledUnauthenticated: codex found
    InstalledUnauthenticated --> Ready: login success
    NotInstalled --> Installing: install
    Installing --> InstalledUnauthenticated: install success
    Installing --> Error: install failure
    InstalledUnauthenticated --> LoggingIn: OAuth start
    LoggingIn --> Ready: OAuth success
    LoggingIn --> Error: OAuth failure or cancel
    Ready --> Running: app-server start
    Running --> Ready: idle
    Running --> Error: process exit
    Error --> Unknown: retry
```

## Conversation/thread mapping

```mermaid
flowchart TD
    Conversation[Cursor Conversation ID]
    Conversation --> Mapping[Persistent codex.json mapping]
    Mapping --> Thread[Codex Thread ID]
    Thread --> Workspace[Workspace Path]
    Thread --> Profile[Codex Runtime Profile]
    Profile --> Binary[Codex Binary]
    Profile --> Home[CODEX_HOME]
```

The mapping is conversation-owned, not global model configuration. Resume is
allowed only when the model and workspace still match. The runtime manager
serializes turns on a single thread and drops late notifications by waiting on
the current thread's completion boundary.

## Data boundaries

```mermaid
flowchart LR
    CursorHistory[Cursor semantic history] -->|latest user input only| Turn[Codex turn/start]
    Turn --> CodexHistory[Codex persisted thread history]
    CodexHistory -->|delta and metadata| CursorStream[Cursor stream lifecycle]
    RuntimeState[Codex mapping and process state] -. mutable .-> CursorHistory
    Credentials[CODEX_HOME credentials] -. never copied .-> Frontend[Frontend]
```

`state.json` and `context.json` remain the forwarder's recovery truth for
semantic history and volatile state. `codex.json` is a small mutable adapter
mapping; it is not injected into model-visible prompt history.

## MVP boundary

```mermaid
flowchart LR
    CursorTools[Cursor-owned tools] -. deferred .-> DynamicBridge[dynamicTools / item/tool/call]
    CodexTools[Codex-owned shell/file tools] --> MVP[Included in MVP]
    DynamicBridge --> V2[Post-MVP]
```

Codex owns shell/file execution under `workspace-write`. BYOK does not expose
an approval UI in this stage; `approvalPolicy: never` is explicit in runtime
setup, adapter request construction, and the dashboard warning.
