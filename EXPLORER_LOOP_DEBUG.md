# Explorer loop trong cursor-byok

Ngày điều tra: 2026-08-07  
Phạm vi: forwarder/model bridge, runtime history/logs, và so sánh với `submodules/codex`.

Cập nhật tham khảo: 2026-08-08 — thêm và đọc submodule `submodules/claude-cli` tại commit `5208f28e37695776e342b5ec7429a7b3022a5c9c`.

## Kết luận ngắn

Model không bị thiếu tool và local-mode bridge không bị kẹt ACK. Vấn đề chính là orchestration cho phép model tiếp tục gọi các tool thăm dò (`Read`, `Grep`, `Ls`, `Shell`, `WebSearch`) mà không có giới hạn “no progress” hoặc cổng bắt buộc chuyển sang hành động (`Write`, `PatchEdit`, chạy kiểm chứng có tác động).

Vấn đề bị khuếch đại ở subagent: runtime đang tạo child conversation với contract kiểu investigator, trong khi yêu cầu của parent có thể là implementation. Vì vậy DeepSeek có thể hoàn thành hàng chục provider pass chỉ bằng đọc/grep/shell.

WebSearch là lỗi riêng: OpenSERP nhiều lần gặp captcha, 429, 403, 503 và circuit breaker. Các lỗi này được trả lại cho model rồi turn tự resume, làm vòng explorer dài hơn.

## Bằng chứng từ cursor-byok

### Tool đã được gửi đến provider

Forwarder đưa toàn bộ tool đã compile vào request và đặt `tool_choice: "auto"` tại [service.go](D:/GitHub/cursor-byok/internal/backend/forwarder/service.go:1381). OpenAI-compatible adapter cũng truyền lựa chọn này ở cả Chat Completions và Responses tại [openai.go](D:/GitHub/cursor-byok/internal/backend/agent/model/openai.go:482) và [openai.go](D:/GitHub/cursor-byok/internal/backend/agent/model/openai.go:949).

Catalog agent có `Write` và `PatchEdit` tại [tool_catalog.go](D:/GitHub/cursor-byok/internal/backend/forwarder/tool_catalog.go:51) và prompt asset tương ứng tại [tools.json](D:/GitHub/cursor-byok/prompt/agent/tools.json:437), [tools.json](D:/GitHub/cursor-byok/prompt/agent/tools.json:641). Do đó đây không phải lỗi “model không thấy tool edit”.

Catalog đã lọc tool theo mode; plan mode cố ý không có `Write/Delete/PatchEdit` tại [tool_catalog.go](D:/GitHub/cursor-byok/internal/backend/forwarder/tool_catalog.go:143). Tuy nhiên child conversation được ép dùng asset agent tại [tool_catalog.go](D:/GitHub/cursor-byok/internal/backend/forwarder/tool_catalog.go:232), nên explorer/worker hiện vẫn nhận cùng nhóm agent tools; khác biệt hành vi chủ yếu đến từ prompt/role contract.

### Runtime đang thực sự chạy tool

Các trace bidi/RunSSE có cả request execute và client result/control message. Vì vậy tool bridge đang hoạt động; model đang chủ động chọn tool đọc thay vì tool thay đổi.

Trace DeepSeek child [provider.jsonl](C:/Users/Admin/.cursor-local-assistant-v2/history/163ed542-6ebe-4ff8-a4ab-3740d0f93d11/debug/provider.jsonl) có 44 provider pass, đều kết thúc bằng `tool_calls`, nhưng chuỗi tool quan sát được chỉ xoay quanh `Read/Grep/Ls/Shell`, chưa có `Write/PatchEdit`. Trong cùng request đầu tiên vẫn có `tool_count` khoảng 20 và `tool_choice: auto`.

Đây không phải giới hạn cố hữu của mọi adapter: trace GLM có `write_args` nhiều lần tại [provider.jsonl](C:/Users/Admin/.cursor-local-assistant-v2/history/a92d81a0-165a-4459-85c1-46514b0825c5/debug/provider.jsonl); trace GPT cũng có `write_args` trong các phiên dài. Kết luận phù hợp nhất là contract/orchestration làm DeepSeek child đi sai vai trò, không phải endpoint không hỗ trợ write.

### Vòng resume không có progress gate

Khi provider pass đã có bất kỳ tool invocation nào, forwarder checkpoint và resume provider tại [actor.go](D:/GitHub/cursor-byok/internal/backend/forwarder/actor.go:703) và [actor.go](D:/GitHub/cursor-byok/internal/backend/forwarder/actor.go:808). `completionDispositionForExternalResults` cũng chỉ phân biệt “có tool hay không”, không phân biệt read-only progress với mutation progress, tại [actor.go](D:/GitHub/cursor-byok/internal/backend/forwarder/actor.go:1140).

Vì vậy chuỗi sau là hợp lệ mãi mãi:

```text
Read/Grep/Ls -> tool result -> auto resume -> Read/Grep/Ls -> ...
```

Lời nhắc agent có câu “continue until the action is complete” tại [reminders.go](D:/GitHub/cursor-byok/internal/backend/forwarder/reminders.go:197), nhưng đây chỉ là instruction cho model, không phải invariant được runtime kiểm tra.

### Child contract đang kéo model về investigator

Trong provider input của trace DeepSeek child có system reminder yêu cầu child “work as an investigator for the parent agent” và trả về investigation result ngắn. Điều này mâu thuẫn với yêu cầu parent muốn child triển khai code. Đây là nguyên nhân trực tiếp cần sửa ở lớp spawn/subagent role.

### WebSearch đang tạo thêm vòng lặp

Khi người dùng approve WebSearch, bridge thực thi OpenSERP và trả lỗi trực tiếp vào tool result tại [bridge.go](D:/GitHub/cursor-byok/internal/backend/agent/bridge/interaction/bridge.go:424) và [bridge.go](D:/GitHub/cursor-byok/internal/backend/agent/bridge/interaction/bridge.go:586).

OpenSERP query bốn engine chính và cố gắng thay thế engine lỗi tại [client.go](D:/GitHub/cursor-byok/internal/search/openserp/client.go:103), [client.go](D:/GitHub/cursor-byok/internal/search/openserp/client.go:119), [client.go](D:/GitHub/cursor-byok/internal/search/openserp/client.go:192). Runtime log ghi lặp lại `OpenSERP returned no replacement for engine=yandex` tại [app.log](C:/Users/Admin/.cursor-local-assistant-v2/logs/app.log:7120), còn OpenSERP log có 503/403 ở [openserp.log](C:/Users/Admin/.cursor-local-assistant-v2/logs/openserp.log:199). Sau tool error, actor vẫn có thể resume model, nên model tiếp tục search/inspect thay vì kết thúc với lỗi rõ ràng.

## So sánh với `submodules/codex`

Snapshot được đọc ở commit `ee0247f95` (`Extract exec-server request dispatching`). Submodule đã có thay đổi local trong các file `exec-server`; không có file nào bị chỉnh sửa trong quá trình tham khảo.

### 1. Codex tách rõ explorer và worker

Codex có built-in role `explorer` cho câu hỏi codebase và `worker` cho execution/production work, với mô tả khác nhau tại [role.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/agent/role.rs:379). Spawn handler nhận `agent_type`, áp dụng role config trước khi tạo child tại [spawn.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/tools/handlers/multi_agents/spawn.rs:57) và [spawn.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/tools/handlers/multi_agents/spawn.rs:105).

Codex cũng có đường riêng để giữ developer instructions của caller khi áp dụng role tại [role.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/agent/role.rs:50) và [role.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/agent/role.rs:185). Đây là điểm cần áp dụng trực tiếp cho lỗi `generalPurpose` hiện tại: task implementation phải vào worker contract; task investigation mới vào explorer contract.

### 2. Codex dùng trạng thái follow-up có ý nghĩa hơn một boolean request-level

Kết quả xử lý output của Codex có `needs_follow_up` và `tool_future` tại [stream_events_utils.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/stream_events_utils.rs:195). Tool call mới set `needs_follow_up` tại [stream_events_utils.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/stream_events_utils.rs:296); output không có tool thì không tự tạo follow-up. Outer turn chỉ tiếp tục nếu `needs_follow_up` hoặc có pending input tại [turn.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/session/turn.rs:383) và kết thúc khi không còn follow-up tại [turn.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/session/turn.rs:460).

Codex chờ toàn bộ tool future hoàn tất trước khi quyết định bước tiếp theo tại [turn.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/session/turn.rs:2106) và [turn.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/session/turn.rs:2691). Điều này không tự giải quyết semantic “đã làm đủ chưa”, nhưng làm ranh giới tool result/follow-up rõ và tránh resume dựa trên trạng thái tool mơ hồ.

### 3. Codex dựng tool router theo từng step/mode

Codex không chỉ gửi một static catalog; nó dựng `ToolRouter`, lọc exposure và tạo model-visible specs theo `TurnContext` tại [spec_plan.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/tools/spec_plan.rs:236), [spec_plan.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/tools/spec_plan.rs:294), [spec_plan.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/tools/spec_plan.rs:326). Apply-patch chỉ được register khi environment và model capability phù hợp tại [spec_plan.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/tools/spec_plan.rs:859).

Tương đương gần nhất của cursor-byok là `ToolCatalog.Load(mode, subagentTypeName)`, nên hướng cải thiện không phải bỏ catalog hiện tại mà là dùng `subagentTypeName` để phân biệt rõ explorer/worker và nhất quán giữa tool exposure, prompt và resume policy.

### 4. Codex có hard boundary về ngân sách

Codex có shared rollout budget theo token; khi hết budget, session trả lỗi `SessionBudgetExceeded` tại [rollout_budget.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/session/rollout_budget.rs:25), và nhắc model số token còn lại tại [rollout_budget.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/rollout_budget.rs:43). Đây là hard stop theo chi phí/context, không phải progress gate theo loại tool.

Vì vậy không nên copy nguyên xi chỉ một token budget để chữa explorer loop. cursor-byok cần thêm một budget riêng cho số lượt read-only liên tiếp hoặc số provider pass không tạo mutation/verification progress.

### 5. Codex có completion hook có thể chặn kết thúc

Khi model không cần follow-up, Codex chạy stop hooks; hook có thể inject continuation prompt hoặc chặn completion tại [turn.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/session/turn.rs:460) và [hook_runtime.rs](D:/GitHub/cursor-byok/submodules/codex/codex-rs/core/src/hook_runtime.rs:298). Đây là điểm mở phù hợp để thêm “implementation completion check”, nhưng cần tránh biến mọi câu hỏi read-only thành bắt buộc edit.

## Tham khảo `submodules/claude-cli`

Submodule được thêm bằng URL `https://github.com/billythekidz/claude-cli.git` và pin tại commit `5208f28e37695776e342b5ec7429a7b3022a5c9c`. README của repo này mô tả đây là research mirror của Claude Code, không phải mã nguồn chính thức của Anthropic; vì vậy các kết luận dưới đây là pattern tham khảo từ implementation snapshot, không phải specification upstream.

### 1. Tách role thành contract thực thi thật sự

Claude CLI không dùng một prompt “explorer” chung cho mọi child:

- `Explore` có system prompt bắt buộc read-only và disallow `Write`, `Edit`, `NotebookEdit`, `ExitPlanMode` và spawn agent tại [exploreAgent.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/built-in/exploreAgent.ts:31).
- `Plan` cũng read-only nhưng contract là thiết kế implementation plan tại [planAgent.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/built-in/planAgent.ts:19).
- `general-purpose` được mô tả là phải dùng tool để hoàn thành task đầy đủ và nhận `tools: ['*']` tại [generalPurposeAgent.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/built-in/generalPurposeAgent.ts:3) và [generalPurposeAgent.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/built-in/generalPurposeAgent.ts:22).

Quan trọng hơn prompt: lúc spawn, Claude CLI dựng `workerTools` từ permission mode của agent, mặc định `acceptEdits`, thay vì lấy nguyên restriction của parent tại [AgentTool.tsx](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/AgentTool.tsx:580). Sau đó `runAgent` lọc tool theo definition và có thể thay toàn bộ session allow rules bằng `allowedTools`, không để approval của parent rò sang child tại [runAgent.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/runAgent.ts:465) và [agentToolUtils.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/agentToolUtils.ts:122).

Đây là pattern cần port trực tiếp nhất vào cursor-byok: `explorer`, `planner`, `worker` phải là ba execution contract khác nhau ở spawn path; không chỉ là tên/nhắc nhở được chèn vào cùng một child conversation.

### 2. Có hard boundary cho agentic loop, nhưng phải bổ sung no-progress gate riêng

Query API của Claude CLI có `maxTurns` và `taskBudget` ngay trong params tại [query.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/query.ts:181). Trước khi quay lại model sau một batch tool, nó tăng `turnCount`, kiểm tra `maxTurns`, phát `max_turns_reached` và return terminal tại [query.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/query.ts:1678). Agent definition cũng có thể khai báo `maxTurns` tại [loadAgentsDir.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/loadAgentsDir.ts:118).

Repo này còn có token-budget continuation với ngưỡng diminishing returns tại [tokenBudget.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/query/tokenBudget.ts:45). Tuy nhiên helper này chủ động bỏ qua `agentId` ở [tokenBudget.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/query/tokenBudget.ts:51), nên không thể copy nguyên xi rồi kỳ vọng nó chặn child explorer. Kết luận: cursor-byok cần hai lớp độc lập:

1. `maxTurns`/token budget để chặn chi phí và số provider pass tuyệt đối.
2. `noProgressPasses`/`consecutiveReadOnlyCalls` để chặn đúng failure mode đang thấy: nhiều vòng Read/Grep/Shell nhưng không có mutation, verification hoặc terminal answer.

Claude CLI snapshot không có một invariant tổng quát kiểu “sau N read-only pass phải chuyển sang action”. Vì vậy submodule củng cố chẩn đoán cũ nhưng không thay thế progress gate của chúng ta.

### 3. Orchestration phân biệt read-only và mutation

`runTools` partition tool calls thành các batch: nhiều tool read-only liên tiếp có thể chạy concurrent, còn tool không read-only chạy serial tại [toolOrchestration.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/services/tools/toolOrchestration.ts:25) và [toolOrchestration.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/services/tools/toolOrchestration.ts:86). Điều này giúp deterministic hơn và làm rõ loại operation, nhưng bản thân nó chỉ là scheduling; nó không tự quyết định model đã “tiến hành” hay chưa.

Claude CLI cũng gom chuỗi Read/Search thành collapsed projection để giảm context/UI noise tại [collapseReadSearch.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/utils/collapseReadSearch.ts:757). Đây là tối ưu context và hiển thị, không phải stop condition; không nên dùng collapse để che lấp việc model đang loop.

### 4. Giữ protocol tool-result nhất quán khi lỗi hoặc fork

Nếu model đã phát `tool_use` nhưng execution bị lỗi trước khi có result, query loop tạo `tool_result` error cho từng tool use tại [query.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/query.ts:123). Khi fork child từ context có tool call dở dang, `runAgent` lọc assistant message không có paired result tại [runAgent.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/runAgent.ts:862). Đây là guard chống API state mồ côi; cần giữ tương đương khi cursor-byok chuyển provider pass hoặc resume child.

Subagent còn gọi `onQueryProgress` trên mọi message, kể cả stream delta, tại [runAgent.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/AgentTool/runAgent.ts:756). Pattern này giải quyết liveness/telemetry của một stream dài, nhưng không được nhầm với action progress: có token stream không có nghĩa là đã sửa code.

### 5. `toolChoice` không phải cách chính để chữa explorer loop

Main query của Claude CLI truyền `toolChoice: undefined` tại [query.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/query.ts:674), tương đương để model tự chọn tool. Chỉ một micro-call riêng cho WebSearch mới force `tool_choice` khi dùng Haiku, đồng thời tắt thinking và không expose tool khác tại [WebSearchTool.ts](D:/GitHub/cursor-byok/submodules/claude-cli/src/tools/WebSearchTool/WebSearchTool.ts:273).

Điều này khớp với trace cursor-byok: không nên force Write/PatchEdit trên toàn bộ request. Cần sửa role, tool exposure, resume policy và progress budget; `tool_choice` chỉ nên dùng cho call chuyên biệt có schema/action duy nhất.

### Mapping áp dụng cho cursor-byok

| Pattern từ `claude-cli` | Ý nghĩa với lỗi hiện tại | Hướng áp dụng |
|---|---|---|
| `Explore`/`Plan` disallow mutation | Explorer không thể “vô tình” nhận task implementation | Tách `subagentType`/contract ở spawn path; worker implementation không dùng investigator reminder |
| Worker tool pool + permission riêng | Parent restriction không làm child mất Write/PatchEdit | Xây child catalog và permission context theo role, không theo heuristic “mọi child = agent” |
| `maxTurns` + `taskBudget` | Không để một child chạy vô hạn | Default cap cho child và terminal state rõ khi cap chạm |
| Read-only batch vs non-read-only serial | Có classification operation để tính progress | Dùng classification cho `noProgressPasses`, không chỉ scheduling |
| Paired tool result / incomplete-call cleanup | Resume/fork không tạo protocol state mồ côi | Validate/canonicalize pair trước mỗi provider resume |
| Main `toolChoice` auto | Model vẫn có quyền tự chọn tool trong contract đúng | Không force Write toàn cục |

## Root cause được xếp hạng

1. **P0 — Sai execution contract ở child role.** `generalPurpose` implementation task đang nhận investigator-style reminder.
2. **P0 — Không có no-progress/action gate.** Actor resume sau mọi tool invocation; không có giới hạn chuỗi read-only.
3. **P1 — WebSearch backend không ổn định.** Error được đưa lại vào model và có thể tạo retry/exploration loop.
4. **P2 — Reasoning/context quá rộng.** Một số request runtime dùng `thinking_effort=max` và context window rất lớn; đây là yếu tố làm model thận trọng/lâu hơn, chưa phải root cause.

## Implementation plan và break task để fix

**Goal:** biến mỗi child conversation thành đúng loại `explorer`, `planner` hoặc `worker`, đồng thời biến provider resume từ “có tool là chạy tiếp” thành quyết định dựa trên progress có giới hạn.

**Architecture:** `tool_catalog.go` và prompt assets quyết định role/tool exposure; `actor.go` quyết định provider continuation/terminal state; một module progress nhỏ phân loại outcome của tool và giữ counter; WebSearch trả lỗi có cấu trúc để actor không retry cùng một failure vô hạn. Main request vẫn dùng `tool_choice=auto`.

**Global constraints:**

- Giữ `model-visible history` append-only; counter/progress metadata phải nằm trong runtime state hoặc bounded debug metadata, không rewrite history.
- Không để prompt nhắc “hãy dùng Write” thay thế cho invariant runtime.
- `Explore`/`Plan` không được expose mutation tool; `worker` implementation phải có Write/PatchEdit khi permission và provider capability cho phép.
- Không force `tool_choice` trên request chính; chỉ dùng forced choice cho call chuyên biệt có một action/schema rõ ràng.
- Mọi provider pass phải có terminal reason hoặc follow-up reason có thể quan sát trong debug record.
- Không triển khai full-suite hoặc thay đổi prompt asset production chỉ để đổi threshold trước khi replay baseline xác nhận failure mode.

### Dependency và thứ tự merge

```text
Task 0 baseline/contract decision
          |
          v
Task 1 role + tool exposure -----> Task 3 WebSearch terminal errors
          |                                  |
          v                                  v
Task 2 progress-aware actor resume ---> Task 4 protocol recovery
          |
          v
Task 5 observability + guarded rollout
```

Task 1 có thể review độc lập trước Task 2. Task 3 có thể làm song song sau khi thống nhất `ProgressOutcome`, nhưng Task 4 phải dùng cùng terminal/follow-up taxonomy với Task 2.

### Task 0: Chốt baseline, vocabulary và policy authority

**Files:**

- Read: [actor.go](D:/GitHub/cursor-byok/internal/backend/forwarder/actor.go:691), [tool_catalog.go](D:/GitHub/cursor-byok/internal/backend/forwarder/tool_catalog.go:22), [compiler.go](D:/GitHub/cursor-byok/internal/backend/forwarder/compiler.go:36), trace DeepSeek đã nêu ở phần bằng chứng.
- Modify: báo cáo này và debug field inventory trong [debug_recorder.go](D:/GitHub/cursor-byok/internal/backend/forwarder/debug_recorder.go:350) nếu cần bổ sung tên field trước khi code.

**Deliverable:** một bảng baseline có `conversation_id`, role/subagent type, provider pass, tool names, tool result status, và terminal reason; không tính stream delta là action progress.

- [x] Ghi lại failure signature hiện tại: DeepSeek child có 44 provider pass, liên tục `Read/Grep/Ls/Shell`, không có `Write/PatchEdit`.
- [x] Chốt role mapping: `Explore` và `Plan` là read-only; `generalPurpose`, implementation task và child không có type rõ ràng mặc định là `worker`; type lạ dùng worker contract thay vì investigator reminder.
- [x] Chốt outcome: `observation`, `mutation`, `verification`, `failure`, `duplicate/no progress`, và terminal provider/runtime.
- [x] Chốt safety defaults tập trung tại [provider_progress.go](D:/GitHub/cursor-byok/internal/backend/forwarder/provider_progress.go): `maxProviderPasses=12`, `maxNoProgressPasses=4`, một recovery nudge; WebSearch cho tối đa một retry cùng normalized query.
- [x] Ghi rõ authority: role/tool exposure ở `agent_role.go` + `tool_catalog.go`/compiler; continuation state ở `provider_progress.go` và actor; WebSearch classification ở OpenSERP/interaction bridge.

**Acceptance:** engineer có thể phân biệt “model đang sống” với “task đang tiến triển”, và mọi threshold/policy đều có một nơi sở hữu duy nhất.

### Task 1: Tách role contract và worker tool pool

**Files:**

- Modify: [tool_catalog.go](D:/GitHub/cursor-byok/internal/backend/forwarder/tool_catalog.go:22), [compiler.go](D:/GitHub/cursor-byok/internal/backend/forwarder/compiler.go:44), [reminders.go](D:/GitHub/cursor-byok/internal/backend/forwarder/reminders.go:197), [prompt.md](D:/GitHub/cursor-byok/prompt/agent/prompt.md), [tools.json](D:/GitHub/cursor-byok/prompt/agent/tools.json:437).
- Prefer create: `internal/backend/forwarder/agent_contract.go` cho role enum và resolver, để không tiếp tục phình `actor.go` hoặc `tool_catalog.go`.
- Inspect/modify only if the spawn metadata is currently lost: [file_store.go](D:/GitHub/cursor-byok/internal/backend/forwarder/file_store.go:665) và conversation creation path.

**Interface:**

```text
type AgentRole string
const (
  AgentRoleExplorer AgentRole = "explorer"
  AgentRolePlanner  AgentRole = "planner"
  AgentRoleWorker   AgentRole = "worker"
)

func ResolveAgentRole(mode agentv1.AgentMode, subagentTypeName string) AgentRole
func (catalog *DefaultToolCatalog) LoadForRole(mode agentv1.AgentMode, role AgentRole) (...)
```

- [x] Resolve role once from persisted `SubagentTypeName`; pass the resolved role through prompt compilation, tool catalog and reminders.
- [x] For `explorer`/`planner`, expose only read/search tools and remove `Write`, `PatchEdit`, `Delete`, `Task`, `AskQuestion` and arbitrary MCP mutation paths; `Shell` remains exposed for read-only commands under the contract.
- [x] For `worker`, use the agent tool asset plus worker contract; do not inject the old generic investigator contract.
- [x] Preserve parent task intent by adding the role contract to the stable system prompt and current reminder without rewriting history.
- [x] Keep `tool_choice=auto`; catalog filtering now determines the role-specific tool list.

**Acceptance:** an implementation child receives a worker contract and mutation tools; an explicit Explore/Plan child cannot mutate; prompt, tool list and persisted role agree for resume.

### Task 2: Replace boolean resume with progress-aware continuation

**Files:**

- Prefer create: `internal/backend/forwarder/progress.go` for bounded progress types/classification.
- Modify: [actor.go](D:/GitHub/cursor-byok/internal/backend/forwarder/actor.go:691), [types.go](D:/GitHub/cursor-byok/internal/backend/forwarder/types.go), [state_tools.go](D:/GitHub/cursor-byok/internal/backend/forwarder/state_tools.go), and the tool-result normalization path used before [completionDispositionForExternalResults](D:/GitHub/cursor-byok/internal/backend/forwarder/actor.go:1140).

**Interface:**

```text
type ProgressOutcome string
const (
  ProgressObservation ProgressOutcome = "observation"
  ProgressMutation    ProgressOutcome = "mutation"
  ProgressVerification ProgressOutcome = "verification"
  ProgressNoChange    ProgressOutcome = "no_change"
  ProgressFailure     ProgressOutcome = "failure"
)

type TurnProgress struct {
  ProviderPass int
  NoProgressPasses int
  RepeatedReadOnlyCalls int
  LastOutcome ProgressOutcome
  LastToolName string
}

func classifyToolResult(toolName string, args, result []byte) ProgressOutcome
func decideContinuation(role AgentRole, progress TurnProgress, pendingResults int) ContinuationDecision
```

- [x] Replace the semantic resume decision with a pass outcome assembled after external tool results are applied; non-child keeps legacy behavior.
- [x] Classify read/search, mutation, verification, failure and duplicate results; empty results do not count as progress.
- [x] Treat successful WebSearch as observation and structured failure payloads as failure; failed search does not reset the streak.
- [x] Reset the consecutive counter only on meaningful progress; identical tool/args/result fingerprints remain no-progress.
- [x] At four worker no-progress passes inject one bounded action nudge; a following no-progress pass ends as `blocked_no_progress`. Explorer/Plan receive a report nudge instead of a mutation demand.
- [x] Enforce `maxProviderPasses=12` for child resumes and emit `max_provider_passes` as a structured terminal reason.
- [x] Wait for pending external results and pairing repair before deciding follow-up; no resume is caused by invocation existence alone.

**Acceptance:** the known 44-pass trace stops within the configured bound with `blocked_no_progress` or `max_provider_passes`; a worker that writes or verifies resets the counter and continues normally; ordinary read-only questions still finish without an action nudge.

### Task 3: Make WebSearch failures terminal per attempt, not fuel for looping

**Files:**

- Modify: [client.go](D:/GitHub/cursor-byok/internal/search/openserp/client.go:103), [bridge.go](D:/GitHub/cursor-byok/internal/backend/agent/bridge/interaction/bridge.go:424), [interaction_tools.go](D:/GitHub/cursor-byok/internal/backend/forwarder/interaction_tools.go), and the progress classifier from Task 2.

- [x] Normalize OpenSERP failures into bounded classes: startup, transport, rate/block, no-results and too-many-errors.
- [x] Keep fallback engines bounded within one WebSearch invocation and pass a 45-second context deadline.
- [x] Return bounded tool-result metadata with `retryable`, `failure_class`, query fingerprint and attempted engines.
- [x] Allow one retry for a retryable identical query; a different normalized query gets its own bounded key, while terminal identical retries are blocked.
- [x] Feed WebSearch result text into the shared progress classifier as observation or failure.

**Acceptance:** the observed OpenSERP 503/403/captcha case produces at most one controlled retry for the same query, then a terminal result; successful WebSearch remains usable and does not force Write.

### Task 4: Harden tool-result pairing and resume recovery

**Files:**

- Modify: [actor.go](D:/GitHub/cursor-byok/internal/backend/forwarder/actor.go:808), [file_store.go](D:/GitHub/cursor-byok/internal/backend/forwarder/file_store.go:665), [tool_error_completion.go](D:/GitHub/cursor-byok/internal/backend/forwarder/tool_error_completion.go), and provider request assembly in [openai.go](D:/GitHub/cursor-byok/internal/backend/agent/model/openai.go:482).

- [x] Before child provider resume, assert every emitted tool invocation has exactly one corresponding result; missing results receive a structured execution-error result.
- [x] Cancellation and the 90-second interaction watchdog append structured error results for pending exec/interaction calls before stopping; late interaction responses are ignored through a bounded completion tombstone.
- [x] Keep append-only history: repair by appending a result, never delete an earlier assistant tool call.
- [x] Distinguish `missing_tool_result`, `provider_error`, `blocked_no_progress` and `max_provider_passes` in terminal code/metadata.

**Acceptance:** interrupted WebSearch/Shell/tool calls produce valid replayable history; provider resumes contain paired tool calls/results; no orphaned tool-call error is hidden by the recovery path.

### Task 5: Add bounded telemetry and guarded rollout

**Files:**

- Modify: [debug_recorder.go](D:/GitHub/cursor-byok/internal/backend/forwarder/debug_recorder.go:350), [runtime_summary.go](D:/GitHub/cursor-byok/internal/backend/forwarder/runtime_summary.go), [actor.go](D:/GitHub/cursor-byok/internal/backend/forwarder/actor.go:703), and configuration authority in [types.go](D:/GitHub/cursor-byok/internal/backend/server/config/types.go:60) only if thresholds must be user-configurable.

- [x] Record bounded role/pass/tool/outcome/no-progress/terminal fields; do not record full tool arguments or unbounded model output.
- [x] Preserve current behavior for non-child conversations by gating the watchdog on resolved child role; the progress guard can be disabled at runtime with `CURSOR_BYOK_PROVIDER_PROGRESS_GUARD=0|false|off|disabled` without changing role resolution or tool policy.
- [x] Add user-visible cap/blocked messages with the last bounded tool name and terminal reason.
- [x] Replay the DeepSeek, GLM and GPT traces as a deterministic policy replay and compare pass count, tool exposure, terminal reason and pairing evidence.
- [x] Run targeted existing package validation for forwarder, interaction bridge and OpenSERP; `git diff --check` remains a final validation step.

**Acceptance:** rollout can be enabled/disabled without changing role resolution; debug records explain every stop; the original DeepSeek loop is bounded without regressing worker execution or normal Explorer/Plan behavior.

### Review gates before implementation

1. Approve the role mapping and the initial thresholds in Task 0; these are externally observable behavior.
2. Review Task 1 independently: compare provider request tool lists and prompts for Explore, Plan and worker.
3. Review Task 2 with the DeepSeek trace before touching WebSearch; it is the core fix for the endless explorer loop.
4. Review Task 3 and Task 4 together because error classification determines whether actor can safely resume.
5. Enable Task 5 rollout only after replay proves that successful Write/PatchEdit and verification reset the no-progress counter.

## Trạng thái thay đổi

### Đã triển khai

- [x] Task 1 — thêm resolver role trong [agent_role.go](D:/GitHub/cursor-byok/internal/backend/forwarder/agent_role.go), tách `explorer`/`planner` read-only khỏi `worker`; catalog chặn mutation/spawn cho hai role đầu, worker giữ agent tools trừ `AskQuestion`.
- [x] Task 1 — compiler thêm role contract ổn định vào system prompt và reminder; `generalPurpose`, `browser-use`, `shell` hoặc type lạ mặc định là worker. `tool_choice` vẫn là `auto`.
- [x] Task 2 — [provider_progress.go](D:/GitHub/cursor-byok/internal/backend/forwarder/provider_progress.go) phân loại observation/mutation/verification/failure/duplicate, chỉ áp dụng watchdog cho child, cap 12 provider pass, cap 4 no-progress pass và một recovery nudge.
- [x] Task 2 — terminal metadata/debug record có `agent_role`, `provider_pass`, `last_tool`, `progress`, `no_progress_passes`, `reason`; non-child giữ đường resume cũ.
- [x] Task 2/5 — provider loop có `last_outcome`, `web_search_failure_class`, `progress_guard_enabled`, cap pass/no-progress và recovery nudge bounded; không ghi args hay model output đầy đủ.
- [x] Task 3 — OpenSERP có failure class bounded; WebSearch truyền context deadline 45 giây, dùng fallback HTML hữu hạn, cho phép tối đa một lần retry cùng normalized query và trả query fingerprint/attempted engines trong tool result.
- [x] Task 4 — trước resume kiểm tra pairing; thiếu result được append error `missing_tool_result` theo append-only, orphan/duplicate pair dừng với terminal reason riêng. Cancellation và interaction timeout append error cho pending exec/interaction; response đến muộn bị tombstone bỏ qua.
- [x] Task 5 — status terminal chuyển thành message có nghĩa (`blocked_no_progress`, `max_provider_passes`, `missing_tool_result`) thay vì chỉ generic provider error; debug fields mới đều bounded và không ghi args/model output.
- [x] Task 5 — kill-switch rollout là `CURSOR_BYOK_PROVIDER_PROGRESS_GUARD`; mặc định bật, tắt chỉ watchdog progress/cap của child và giữ nguyên resolver role.

### Validation đã chạy

- `go test ./internal/backend/forwarder`
- `go test ./internal/backend/agent/bridge/interaction ./internal/search/openserp ./internal/backend/forwarder`
- `go vet ./internal/backend/forwarder ./internal/backend/agent/bridge/interaction ./internal/search/openserp`
- `git diff --check`

Kết quả targeted validation hiện tại: ba package test đều pass, `go vet` pass, và `git diff --check` không có lỗi whitespace (chỉ có cảnh báo line-ending của Git trên các file Go hiện hữu). Đã sửa hai copylock warning trong interaction bridge bằng cách marshal qua pointer và clone protobuf reference theo field, không thay đổi protocol.

### Replay tĩnh ba trace cũ

Replay đọc read-only các `debug/provider.jsonl` và `state.json` hiện có; không gọi lại provider, nên terminal dự kiến là kết quả của policy mới chứ không phải một runtime replay có network.

| Trace | Role từ state | Kết quả cũ | Tool calls cũ | Kết quả policy mới |
|---|---|---:|---|---|
| DeepSeek `163ed542-6ebe-4ff8-a4ab-3740d0f93d11` | `generalPurpose` → `worker` | 45 provider pass, 44 `tool_calls`, không có mutation | `Read=102`, `Grep=30`, `Ls=14`, `Shell=24`, `Glob=3`, `AwaitShell=2` | worker được phép mutation/verification; nếu vẫn read-only thì dừng ở pass 12 với `max_provider_passes`; `CURSOR_BYOK_PROVIDER_PROGRESS_GUARD=0` khôi phục đường resume cũ |
| GLM `a92d81a0-165a-4459-85c1-46514b0825c5` | không có child type → `main` | 12 pass, có completed | `Grep=16`, `PatchEdit=16`, `Read=4`, `ReadLints=6` | non-child giữ legacy resume, không bị watchdog chặn |
| GPT `1acdd64d-17dd-463b-84b3-8ce1062c51d6` | mode `multitask`, không có child type → `main` | 119 pass, có completed | `Read=428`, `Grep=100`, `Ls=58`, `Shell=54`, `Glob=20`, `Write=2` cùng các tool điều phối | non-child giữ legacy resume; mutation `Write` vẫn được phép |

Tool exposure sau patch được quyết định từ cùng catalog: `explorer`/`planner` chỉ có 9 tool read/search (Shell bị kiểm tra command read-only), `worker` có agent tool pool trừ `AskQuestion`, còn main giữ mode catalog. Cả ba trace cũ đều có `tool_choice=auto`; patch không force Write/PatchEdit.

`submodules/codex` được dùng làm đối chiếu cho budget/cancel/append-only recovery; `submodules/claude-cli` được dùng để đối chiếu `toolChoice=undefined`/auto và WebSearch micro-call. `submodules/claude-cli` đã được thêm và pin tại commit đã ghi ở đầu file. `submodules/codex` và installed Cursor client không bị chỉnh sửa; các thay đổi khác trong `git status` là thay đổi có sẵn hoặc thuộc phần triển khai nêu trên. Chưa chạy full-suite vì validation hiện tại đã bao phủ các package trực tiếp bị thay đổi.
