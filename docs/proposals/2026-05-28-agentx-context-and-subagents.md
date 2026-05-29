# Proposal: agentx Phase 2 — Cache-Stable Prompt, Mid-Loop Compaction, Sub-Agents, Session Todos

- **Status:** Draft
- **Author:** Laisky (with Claude Code)
- **Created:** 2026-05-28
- **Affected packages:** `internal/tasks/gptchat/agentx/{prompt,loop,session,hook,tools,tool,handler}`
- **Risk class:** Additive on top of Phase 1; opt-in per capability. Existing
  `agent_mode=true` path keeps working unchanged when every new feature is
  off.
- **Predecessor:** [`2026-05-26-gptchat-react-agent-mode.md`](2026-05-26-gptchat-react-agent-mode.md)

---

## 1. Background

### 1.1 What Phase 1 shipped

The agent loop at [`agentx/loop/loop.go`](../../internal/tasks/gptchat/agentx/loop/loop.go)
is a faithful ReAct loop: per round it dispatches `OnContext`, streams the
model, collects assistant text (Thought) + `function_call` items (Actions),
either exits via `send_to_user` / implicit-final or fans the calls out
through the bounded `parallelExecutor` and appends `function_call_output`
items (Observations) back into `inputItems`. The architecture matches the
2026 mainline for conversational tool-using agents (native function-calling,
exit-tool, parallel actions, hook bus, layered budgets, untrusted-content
delimiter).

A research pass against 2025-2026 best practices (Anthropic *Effective
Context Engineering*, Sep 2025; *Building Effective Agents*, Dec 2024;
LangChain Deep Agents; OpenAI Agents SDK; Claude Code) surfaced four gaps
that already cost us real money or reliability today:

1. **Prompt cache is permanently busted.** [`ReactRenderer.Render`](../../internal/tasks/gptchat/agentx/prompt/react.go#L63-L100)
   embeds a per-round budget hint (`"round N of M, %d step(s) remaining"`)
   inside the *system* message body. The provider's prefix-cache key changes
   every round, so the ~1 KB of stable directives in front of it never gets
   a cache hit. Anthropic reports ~90% cost / ~85% latency wins from a
   stable prefix; on long agent turns this is the largest single waste.
2. **No mid-loop compaction.** `inputItems` grows linearly until
   `MaxIterations`. The Phase 1 e2e run on gpt-5.4-mini accumulated ~20
   `web_search` results + a 21 KB Environment Canada page before
   `send_to_user`. Provider auto-truncation does not help — by the time it
   kicks in, the relevant signal has been crowded out by stale tool output.
   The `OnBeforeCompact` bus point exists ([`agentx/hook/events.go:29-33`](../../internal/tasks/gptchat/agentx/hook/events.go#L29-L33))
   but nothing dispatches it.
3. **`spawn_agent` is a stub.** [`SubAgentTool.Execute`](../../internal/tasks/gptchat/agentx/tools/subagent.go#L102-L107)
   returns `IsError=true` with the canonical "subagent execution not enabled
   in this build" content. The locked-in tool name, schema, and registry
   gate (`BeltDeps.SubagentEnabled`) ship; only the executor is missing.
4. **No planning surface.** Production agent harnesses in 2026
   (Claude Code's `TodoWrite`, LangChain Deep Agents' `write_todos`)
   consistently report that giving the model an explicit todo store
   improves multi-step reliability — the model follows plans better than
   it generates them inline.

### 1.2 Motivation

These four are all *primitives* — they unlock further work but each is
small in code (≤500 LOC total) and each is gated behind a flag, so we can
ship and roll back independently. The first two are pure efficiency /
reliability wins on existing traffic; the second two enable new behaviour
the current loop cannot express (delegation, explicit planning).

### 1.3 What this proposal does NOT cover

- **Layered prompt-injection defence** beyond the existing delimiter guard
  (scanners, dependency graphs, capability gating). Worth a separate pass.
- **Persistent / cross-session memory** beyond the existing
  `(user_prompt, final_answer)` pair persisted by [`memoryhook.go`](../../internal/tasks/gptchat/agentx/tools/memoryhook.go).
- **Reflection / evaluator-optimizer** loops.
- **Frontend changes** beyond optionally surfacing todos in the trace
  panel (Phase 2C, out of scope here).

---

## 2. Goals and Non-Goals

### 2.1 Goals

- **G1 — Prompt prefix cache hits.** After this change, the upstream
  reports `cached_prompt_tokens > 0` on every round ≥ 2 of a single user
  turn under both OpenAI and Anthropic-via-OneAPI back-ends, for the
  static section of the ReAct prompt.
- **G2 — Bounded context growth.** A turn that runs to `MaxIterations`
  emits at most `compaction_threshold_tokens` of `inputItems` at every
  model call, regardless of how big individual tool outputs were.
- **G3 — Sub-agent delegation works.** `spawn_agent` with a valid
  `(profile, task, allow_tools)` payload runs a child loop with an
  isolated context, returns the child's final answer (or a file
  reference) as the parent's tool result, and is bounded by `MaxDepth`.
- **G4 — Session todos work.** A new local tool `write_todos`
  maintains an in-memory list for the lifetime of one `loop.Run`. The
  current list is injected into every round's context as a stable
  system-role message and emitted as a typed event for the SSE trace.

### 2.2 Non-Goals

- Per-tenant or cross-session todo persistence (in-memory only, dies with
  the session).
- Sub-agent recursion beyond `MaxDepth=2` by default.
- Auto-compaction triggered by *token count from a tokenizer*; Phase 2
  uses a coarse byte-count heuristic. Real token counting can land later
  without touching the trigger interface.
- Replacing `ReactVersionMarker` v1 with v2 — the static prefix keeps
  emitting `[ReAct/v1]` for back-compat with any persisted transcripts.

---

## 3. Change List

### 3.1 Prompt-cache-stable prefix (`prompt/react.go`, `loop/loop.go`)

#### Current behaviour

`ReactRenderer.Render(round, remaining)` renders the entire system prompt
as one string, with the budget hint baked into the tail:

```
[ReAct/v1]
You are an autonomous tool-using assistant ...
WHEN TO USE TOOLS: ...
EXIT CONTRACT: ...
UNTRUSTED CONTENT GUARD: ...
BUDGET HINT:
- You are on round 7 of at most 20. You have 13 step(s) remaining.   <-- changes
- Pace yourself ...
```

`AsContextHook` then *overwrites the marker-bearing system message
in place* every round. Because the changing tail is in the same message
as the stable head, the entire system prompt becomes a fresh prefix to
the upstream cache every round.

#### Target behaviour

Split into two messages:

- **Static system message** at index 0 of the model input. Carries every
  byte that does not change across rounds. Still tagged `[ReAct/v1]` so
  existing detection keeps working.
- **Dynamic budget message**, also system-role, appended **at the tail
  of `inputItems`** just before the model call. Tagged
  `[ReAct/budget-v1]` so it can be found and overwritten on each round.

The two-message split moves all mutation to the suffix, which is past the
provider's cache point. The static prefix is byte-identical every round
of a turn → prefix-cache hit on rounds 2..N.

#### Concrete changes

- `ReactRenderer` gains two methods:
  - `RenderStatic() string` — returns everything except the budget hint.
    Pure function of `r.BudgetCap` (which is stable for the turn). No
    `round` parameter.
  - `RenderBudgetHint(round, remaining int) string` — returns the
    `[ReAct/budget-v1]` marker + 2-line hint. Pure function of its
    arguments.
- `AsContextHook` is rewritten as **two** hooks, registered separately on
  the bus:
  - `AsStaticContextHook()` — first-round prepend, idempotent on later
    rounds (the static message is already in place, no-op).
  - `AsBudgetContextHook()` — finds and overwrites the
    `[ReAct/budget-v1]` system message at the tail; appends one if
    absent.
- `findReactSystemIndex` keeps working for the static marker.
  `findBudgetSystemIndex` is its analogue for the budget marker.
- `PromptRenderer` interface in `loop.go` is widened to accept both pieces
  via two methods, but Phase 2 wires the renderer through the hook bus
  (as today) — the loop itself does not call the renderer directly. The
  existing `PromptRenderer` field on `RunDeps` stays unused (Phase 1
  comment says it's "here only for test ergonomics" and we keep it).
- The handler registers both hooks at session-start with the
  static-hook first so it lands at index 0 and the budget hook last so
  it lands at the tail.

#### Caveat

OpenAI `/responses` accepts multiple `system`-role input items. Anthropic
Messages collapses them into the leading `system` parameter. Our OneAPI
adapter at [`model/oneapi.go`](../../internal/tasks/gptchat/agentx/model/oneapi.go)
must preserve both messages on the wire — verified by U22 (see §4). If
the upstream coalesces them server-side the cache key still benefits
because the *static prefix portion* is unchanged.

### 3.2 Mid-loop compaction and tool-result clearing (`loop/compact.go`, `loop/loop.go`)

#### Trigger

At the top of every round, before `OnContext` fires:

```go
if compactor != nil && compactor.ShouldCompact(inputItems) {
    inputItems = compactor.Compact(loopCtx, inputItems)
    deps.Bus.DispatchBeforeCompact(...)   // observability only in this phase
}
```

`ShouldCompact` is a coarse heuristic: total UTF-8 byte length of the
input items' content fields exceeds `Caps.CompactBytesThreshold` (default
**192 KB**, roughly ~48 K tokens at 4 chars/token, leaving headroom for a
128 K-window model with 30 K reserved for tools+response).

#### Strategy

Two cooperating mechanisms, both cheap and deterministic:

1. **Tool-result clearing (mechanical, always-on once enabled).**
   Walk `inputItems` from the head; any `function_call_output` item more
   than `Caps.ToolResultKeepRecent` rounds old (default **3**) is rewritten
   to a fixed `"[tool output redacted at compact step]"` placeholder. The
   matching `function_call` item is left intact so call/result IDs still
   pair. This recovers the bulk of the bytes (tool outputs dominate).

2. **LLM-summarised compaction (when (1) is insufficient).** If after
   step (1) the input still exceeds the threshold, replace the *contiguous
   head segment* (everything before the last `ToolResultKeepRecent`
   rounds) with a single system-role message tagged
   `[ReAct/compacted-v1]` carrying a summary produced by a one-shot model
   call. The summarisation request is a *separate* `model.Client.Stream`
   call with `tools=nil`, `max_output_tokens=2048`, and a fixed
   summarisation prompt (proposal §3.2.1 below). The same `ModelID` is
   used; we accept the cost in exchange for not maintaining a second
   client.

#### `Compactor` interface

```go
type Compactor interface {
    ShouldCompact(items []model.InputItem) bool
    Compact(ctx context.Context, items []model.InputItem) ([]model.InputItem, error)
}
```

A `DefaultCompactor` implements both steps; tests inject a fake.

#### Hook semantics

`OnBeforeCompact` fires with the **pre-compaction** `inputItems` so
observers can persist a snapshot for the trace. Phase 2A does not let
hooks mutate the slice (the event payload stays as the existing empty
`CompactEvent`). Phase 3 may add fields if a real consumer materialises.

#### Events

New session event kind `KindCompacted` with fields
`{BytesBefore, BytesAfter, Mechanism: "clear"|"summary"|"both"}`. Emitted
via the existing sink; mapped onto `delta.reasoning_content` like other
trace events in [`sse/format.go`](../../internal/tasks/gptchat/agentx/sse/format.go).

#### Caps

Two new fields on `loop.Caps`:

| Field | Default | Meaning |
|---|---|---|
| `CompactBytesThreshold` | `192 * 1024` | Trigger threshold for compaction. 0 disables. |
| `ToolResultKeepRecent` | `3` | Number of recent rounds whose `function_call_output` is kept verbatim. |

A nil/disabled compactor is the default — Phase 2A ships behind a config
flag (`openai.agent_loop.compaction_enabled`) so we can roll it out
gradually.

### 3.3 Sub-agent execution (`tools/subagent.go`, `loop/spawn.go`)

#### Current state

[`SubAgentTool.Execute`](../../internal/tasks/gptchat/agentx/tools/subagent.go#L102-L107)
returns `IsError=true` immediately. The registry already accepts a
`MaxDepth` but never reads it.

#### Target state

The tool's `Execute` runs a fresh `loop.Run` invocation against a
**subset registry**, a **fresh input slice**, the same model + caps, and
returns the child's `Final.FinalText` (or a file reference) as the
parent's `tool.Result.Content`.

#### Wiring

Because `tools/subagent.go` must not import `loop` (it would create a
cycle: loop → tools → loop), we inject a `SpawnFunc` at construction:

```go
type SpawnRequest struct {
    Profile     string
    Task        string
    AllowTools  []string
    OutputMode  string   // "inline" | "file" | "none"
    ParentDepth int
}

type SpawnResult struct {
    FinalText    string
    FileRef      string   // populated when OutputMode == "file"
    TerminatedBy string
    ToolCalls    int
    Iterations   int
}

type SpawnFunc func(ctx context.Context, req SpawnRequest, sink session.EventSink) (SpawnResult, error)
```

`NewSubAgentTool(maxDepth, spawn SpawnFunc)` becomes the constructor.
Phase 1's no-arg path stays as `NewSubAgentToolStub()` for back-compat
test code and is the no-op fallback when `spawn == nil`.

The spawn implementation lives in a new file `loop/spawn.go`:

```go
func NewSpawnFunc(parent SpawnParentDeps) SpawnFunc { ... }
```

`SpawnParentDeps` carries the parent's `model.Client`, the *parent
registry* (so we can subset), the parent `hook.Bus` (read-only — the
child re-uses parent's `BeforeToolCall` / `AfterToolCall` hooks for
shared circuit-breaker / budget enforcement; see §3.3.5), the parent's
`Caps` (the child halves `MaxIterations` and `MaxToolCalls`), and a
counter or struct holding the parent's `Depth`.

#### Tool subsetting

The child registry is built by:

1. Start from the parent registry's `Names()`.
2. Intersect with `AllowTools`. Empty list = empty registry (no tools).
3. Always remove `spawn_agent` itself (default-exclude-self per
   proposal §3.6) unless the parent's `AllowSubAgentRecursion` flag is
   set AND `ParentDepth+1 < MaxDepth`.
4. Always include `send_to_user` (the child needs an exit).

Subsetting is a new method on `tool.Registry`:
`Subset(names []string) Registry`. It returns a view, not a copy — the
underlying tools are shared (they hold no per-call state today).

#### Depth guard

The parent's `SpawnParentDeps` holds `Depth int`. The injected `SpawnFunc`
checks `Depth + 1 > MaxDepth` and returns an `IsError=true` result with
content `"sub-agent depth exhausted (max=%d)"` rather than running the
child. Default `MaxDepth=2` from the existing constant.

#### Wall-clock and budget sharing

The child receives `min(remaining_parent_wall_clock, child_caps.WallClock)`
as its deadline. Tool calls inside the child count against the parent's
budget counter via the shared `hook.Bus` — the
`NewBudgetEnforcerHook(budget)` registered in [`loop.go:131-133`](../../internal/tasks/gptchat/agentx/loop/loop.go#L131-L133)
already covers any `AfterToolCall` regardless of which loop emitted it,
so reusing the parent bus does the right thing automatically.

#### Output handling

| `OutputMode` | `Result.Content` | `Result.Details` |
|---|---|---|
| `inline` (default) | child's `FinalText`, truncated to 8 KB with `"... [truncated]"` suffix | structured `SpawnResult` as JSON |
| `file` | reference string `"<sub-agent output stored in /agentx/spawn/<call_id>.md>"` (writes via `file_io`) | same |
| `none` | `"ok"` | same |

#### Events

The child's events flow through the parent's `EventSink` wrapped by a
`childEventPrefixWrapper` that stamps every event's `ParentID` with the
parent's `StepID`. The frontend trace shows the child's tool calls
indented under the parent's `spawn_agent` step.

### 3.4 Session-scoped todo tool (`tools/todos.go`)

#### Surface

One tool, name `write_todos`, source `SourceLocal`. Args:

```json
{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id":         { "type": "string" },
          "content":    { "type": "string", "minLength": 1 },
          "activeForm": { "type": "string", "minLength": 1 },
          "status":     { "type": "string", "enum": ["pending", "in_progress", "completed"] }
        },
        "required": ["content", "activeForm", "status"],
        "additionalProperties": false
      }
    }
  },
  "required": ["todos"],
  "additionalProperties": false
}
```

`id` is optional on input; the tool assigns ULIDs to entries that omit
it. Subsequent `write_todos` calls that supply the same `id` are
treated as updates; new ids are inserts; previously-seen ids absent
from the new list are deletes. **The tool's contract is "write the full
list"** — exactly Claude Code's TodoWrite shape. No partial updates.

#### State

```go
type TodoStore struct {
    mu    sync.Mutex
    todos []Todo
}

type Todo struct {
    ID         string `json:"id"`
    Content    string `json:"content"`
    ActiveForm string `json:"activeForm"`
    Status     string `json:"status"`
}
```

One `TodoStore` per `loop.Run` invocation. Constructed by the handler
right before `loop.Run`, passed into both:

1. `NewTodoTool(store)` — registered in the belt.
2. `NewTodoContextHook(store)` — registered on the bus's `OnContext`
   chain so the model sees the live list every round.

When the run ends, the store is dropped.

#### Context injection

The `OnContext` hook appends — at the **tail** of `ev.Input`, right
before the budget hint — a system-role message tagged
`[ReAct/todos-v1]` carrying the current list rendered as:

```
[ReAct/todos-v1]
Current plan (write_todos to update):
- [x] researched Ottawa forecast — completed
- [-] fetched primary source page — in_progress
- [ ] summarise into 3-line answer — pending
```

If the list is empty the message is omitted entirely (no marker). The
hook overwrites the existing `[ReAct/todos-v1]` system message in place
the same way the budget hook does, keeping the input shape stable.

#### Concurrency

Parallel tool calls may include two `write_todos` invocations in the
same round (unlikely but legal). The mutex serialises writes; the
last-completed write wins. Since the loop builds the round's appends
from `executor.ExecuteAll`'s returned slice (stable index order), the
*deterministic* last-write-wins is the one at the highest input index.
That matches the user's likely intent (the model put its newer plan
later in the call list).

#### Observability

New event kind `KindTodoUpdated` with payload
`{ StepID, Before []Todo, After []Todo }`. Emitted by `Execute` after
the mutation. SSE writer maps it onto `delta.reasoning_content` like
the other trace events.

#### Tool description

```
Maintain a session-scoped plan. Call write_todos to replace the current
plan with a new list. Pass an empty list to clear. Use this whenever the
user task has 3+ steps so you and the user can both track progress; mark
exactly one item in_progress while you work on it. The plan is private
to this turn — it disappears when send_to_user fires.
```

#### Registration

`tools/belt.go` gains a new `BeltDeps` field `TodoStore *TodoStore`. If
nil, the tool is not registered (Phase 2A ships disabled by default; the
handler can construct a store per request and flip the flag once UI
support exists for the trace view). Tool name is added to
`CuratedBeltNames` so the fallback belt knows about it.

---

## 4. Test Matrix

Phase 2 inherits the Phase 1 invariants (U1..U22 in the predecessor).
New invariants:

| Id | Area | Invariant | Test |
|----|------|-----------|------|
| U23 | prompt cache | Static prompt body is byte-identical across rounds 1..20 of the same `Caps`. | `prompt/react_static_test.go::TestRenderStaticByteStable` — 20 calls with varying round/remaining → all equal. |
| U24 | prompt cache | Budget message is overwritten in place each round; never duplicated. | `prompt/react_budget_test.go::TestBudgetHookIdempotent` — 5 invocations, exactly one `[ReAct/budget-v1]` message in result. |
| U25 | prompt cache | Static prefix is index 0; budget is the LAST system message; no other system message moves. | `prompt/react_layout_test.go::TestLayoutOrder` — fixture with memory hook present, assert positions. |
| U26 | prompt cache | OneAPI adapter preserves both system messages on the wire. | `model/oneapi_systemsplit_test.go::TestPreservesDualSystemMessages` — assert outbound JSON contains both `system` entries verbatim. |
| U27 | compaction | `ShouldCompact` triggers at threshold ±1 byte. | `loop/compact_threshold_test.go` — table test 191 KB / 192 KB / 193 KB. |
| U28 | compaction | Tool-result clearing keeps the most recent N rounds verbatim. | `loop/compact_clear_test.go::TestKeepsRecentRounds` — 6 rounds, N=3, assert older 3 redacted, newer 3 verbatim. |
| U29 | compaction | LLM compaction emits exactly one `[ReAct/compacted-v1]` system message and removes the compacted head segment. | `loop/compact_summary_test.go` — fake model returning canned summary. |
| U30 | compaction | `KindCompacted` event emitted with bytes-before/after. | `loop/compact_events_test.go` — assert sink received event. |
| U31 | compaction | `OnBeforeCompact` hook fires with pre-compaction items. | `loop/compact_hook_test.go` — capturing hook records the slice length. |
| U32 | compaction | Compaction off (`CompactBytesThreshold=0`) is a no-op even on a 1 MB input. | `loop/compact_disabled_test.go`. |
| U33 | sub-agent | `spawn_agent` runs to a `send_to_user` final and the parent receives `FinalText` as `tool.Result.Content`. | `tools/subagent_run_test.go::TestSpawnInline` — fake spawn function. |
| U34 | sub-agent | Depth guard rejects at `MaxDepth`. | `tools/subagent_depth_test.go` — Depth=2 + MaxDepth=2 → IsError. |
| U35 | sub-agent | Child registry contains only intersected `allow_tools` + `send_to_user`, never `spawn_agent` (unless explicitly opted in). | `loop/spawn_subset_test.go`. |
| U36 | sub-agent | `OutputMode=file` writes via `file_io` and the parent's Content is a reference, not the body. | `loop/spawn_filemode_test.go` — fake file_io seam. |
| U37 | sub-agent | Child tool calls increment parent's budget counter. | `loop/spawn_budget_test.go` — assert parent `budget.ToolCalls()` after child run. |
| U38 | sub-agent | Child wall-clock is min(parent_remaining, child_cap). | `loop/spawn_walclock_test.go`. |
| U39 | sub-agent | Child events stamp `ParentID = parent_step_id`. | `loop/spawn_events_test.go`. |
| U40 | todos | `write_todos` mutation is visible in the next round's context. | `tools/todos_context_test.go` — two-round driver, assert second round's input contains the list. |
| U41 | todos | Empty list omits the `[ReAct/todos-v1]` message entirely. | `tools/todos_empty_test.go`. |
| U42 | todos | Tool emits `KindTodoUpdated` with before/after. | `tools/todos_event_test.go`. |
| U43 | todos | Parallel `write_todos` calls serialise; deterministic last-write-wins. | `tools/todos_parallel_test.go` — two calls in one round, fixed seed. |
| U44 | todos | Store dies with the session — a second `loop.Run` sees an empty list. | `tools/todos_session_scope_test.go`. |
| U45 | todos | Tool absent from registry when `BeltDeps.TodoStore == nil`. | `tools/belt_todo_gate_test.go`. |
| U46 | integration | E2E run with all four features on terminates cleanly on a 20-round task and emits all four event kinds at least once. | `agentx/handler_e2e_test.go::TestPhase2AllFeatures` — fake upstream scripted to use compaction + spawn_agent + write_todos. |
| U47 | regression | All Phase 2 features OFF behaves byte-identical to Phase 1 trace. | `agentx/handler_e2e_test.go::TestPhase2OffMatchesPhase1` — golden trace comparison. |

### 4.1 Manual / staging verification (one-time)

- **M1.** Send a 20-round prompt against production OneAPI (gpt-5.4-mini)
  with `compaction_enabled=true, todos_enabled=true`. Inspect upstream
  response usage payload for `cached_prompt_tokens > 0` on rounds 2..N.
- **M2.** Same setup with `spawn_agent` enabled, prompt the model with
  "delegate the web research to a sub-agent." Verify the trace renders
  the child's tool calls nested under the parent's `spawn_agent` step.
- **M3.** Trigger compaction by forcing a `web_fetch` of a 40 KB page on
  round 1, then 9 more rounds; assert `KindCompacted` fires and the
  next-round input no longer carries the 40 KB body.

---

## 5. Acceptance Criteria

A change set against this proposal is accepted when **all** of the
following hold. Each criterion maps to one or more tests above so
reviewers can spot-check.

### 5.1 Correctness

- **A1.** With every Phase 2 feature OFF (`compaction_enabled=false`,
  `subagent_enabled=false`, `todos_enabled=false`), the loop produces
  byte-identical SSE output to Phase 1 head-of-tree on the canonical
  e2e fixture. (U47)
- **A2.** The static ReAct system prompt is byte-identical across all
  rounds of a turn; the budget message is the only mutating system
  message; both ride the wire as separate items on OneAPI. (U23, U24,
  U25, U26)
- **A3.** With `compaction_enabled=true` and the default threshold,
  inputs to the upstream model never exceed `CompactBytesThreshold +
  one round's growth`. (U27, U28, U29, U32)
- **A4.** `spawn_agent` runs a child loop, returns the child's final
  text (or file reference), and respects `MaxDepth`, `allow_tools`
  subsetting, wall-clock min, and parent budget sharing. (U33–U39)
- **A5.** `write_todos` mutations are immediately visible in the next
  round's context as a single tail-positioned system message; an empty
  list emits no message; concurrent calls serialise deterministically.
  (U40–U45)

### 5.2 Observability

- **A6.** Every new mechanism surfaces a typed event: `KindCompacted`,
  `KindTodoUpdated`, plus the existing `ToolCallStart`/`ToolResult` for
  `spawn_agent` and `write_todos`. (U30, U42, U46)
- **A7.** Upstream usage payload reports `cached_prompt_tokens > 0` on
  round ≥ 2 of any agent turn issued to a cache-aware provider. (M1)

### 5.3 Safety / rollback

- **A8.** Each feature can be independently disabled via
  `openai.agent_loop.{compaction_enabled, subagent_enabled,
  todos_enabled}` and the loop continues to function with the others
  enabled. Verified by running U46 with each individual flag flipped
  off.
- **A9.** No new tool runs by default in production until its flag is
  flipped. The shipped settings.yml ships all three flags as `false`
  in the same patch that introduces them; a follow-up commit flips them
  on after staging soak.

### 5.4 Performance

- **A10.** Per-round handler latency does NOT regress more than 10%
  with all features ON vs Phase 1 head-of-tree on the same fixture
  (the compaction LLM call is the only material cost, and only fires
  on threshold hit). Measured via the existing
  `handler_e2e_test.go` timing assertions where present, or by manual
  M1 timing.

### 5.5 Code health

- **A11.** No new package-level cycles. `tools/subagent.go` must not
  import `loop`; the `SpawnFunc` injection seam is the boundary.
- **A12.** New code passes `go vet`, `golangci-lint`, and is covered by
  tests in §4 with line coverage ≥ 80% in `loop/compact.go`,
  `loop/spawn.go`, `tools/todos.go`. (Existing thresholds; no policy
  change.)
- **A13.** Documentation: this proposal links from
  `docs/proposals/README.md` (if one exists); the predecessor's
  Phase 1 file gets a single-line "see also" note pointing here.

---

## 6. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| OneAPI / upstream collapses dual system messages into one and we lose the cache | medium | medium | U26 verifies the *wire* payload; if a provider coalesces server-side the static prefix portion is still byte-stable, so cache key still benefits. Worst case: we add a config switch `prompt_cache_split=false` to collapse client-side. |
| LLM compaction summary drops a critical fact and downstream rounds make wrong calls | low | high | Tool-result clearing (step 1) preserves recent rounds verbatim; the LLM summary only replaces the *old* segment. Threshold is conservative (192 KB, ~48 K tokens); summary call uses the same model so quality matches the main loop. Emit `KindCompacted` so the user can spot it in trace. |
| Sub-agent recursion bug burns budgets | low | high | Depth guard hard-fails before child `loop.Run`; default-exclude-self for `spawn_agent` in the child registry; parent's budget counter is shared so a runaway child trips error-budget termination. |
| Todo tool encourages the model to plan instead of acting | medium | low | Make the tool optional, behind a flag. Description explicitly says "use this whenever the user task has 3+ steps" — does not coerce on simple tasks. |
| Parallel `write_todos` race produces non-deterministic state | low | low | Mutex serialises; result ordering follows the executor's stable input order; covered by U43. |
| Compaction trigger fires inside a single round's growth, costing an extra summary call per round | low | medium | Threshold is well above any single round's worst-case output (`web_fetch` is capped at ~25 KB upstream per the predecessor proposal §4.5); if observed, raise threshold or add a "minimum N rounds between compactions" guard in a follow-up. |

---

## 7. Phasing & Rollout

| Phase | Scope | Gating |
|---|---|---|
| 2A | (3.1) prompt split + (3.4) todos tool. Both ship with flags default-off. | One PR. Land U23–U26 + U40–U45 + U47. |
| 2B | (3.2) compaction. Ships default-off. | Second PR. Land U27–U32. |
| 2C | (3.3) sub-agent execution. Ships default-off (`subagent_enabled=false`). | Third PR. Land U33–U39. |
| 2D | Soak + flip flags. Flip in this order: prompt-split (zero behaviour risk), todos, compaction, sub-agents. One flip per week with the prior week's e2e trace as the comparison baseline. | Config-only commits; no code. |

Each phase is independently revertable: every feature lives behind a
config flag, every interface is additive (`Compactor` is new,
`SpawnFunc` is a constructor argument, `TodoStore` is a new
`BeltDeps` field, the prompt split adds new methods without removing
`Render`).

---

## 8. Open Questions

- **Q1.** Should the budget hint be a system message at the tail, or
  attached as a `user`-role "system_note" preamble that the model
  conventions tend to weight more? Default: system at tail (cheaper to
  reason about; matches existing `[ReAct/v1]` shape). Revisit if A2's
  cache numbers come back weak.
- **Q2.** Should the todo list be persisted into long-term memory at
  session end (alongside the existing `user_prompt`/`final_answer`
  pair)? Default: no — todos are intermediate scratchpad, not user-
  facing artifacts.
- **Q3.** Should sub-agent events emit a separate `event: agent_child`
  SSE channel rather than reusing the parent's `reasoning_content`
  stream? Default: no for Phase 2C; revisit when Phase 2B-typed
  channel from the predecessor proposal lands.
- **Q4.** Compaction summary prompt — should we keep it as a fixed
  string in code or move it to `prompt/compact.go` with a version
  marker like the ReAct prompt? Lean: separate file, versioned
  (`[Compact/v1]`).
- **Q5.** Should the `write_todos` tool also accept `note` text to
  attach to each item? Default: no — keep the surface as small as
  Claude Code's TodoWrite; add if the model demonstrably needs it.

---

## 9. Reference Implementations

- **Anthropic.** *Effective Context Engineering for AI Agents* (Sep
  2025) — compaction, structured note-taking, JIT context.
- **Anthropic.** *Building Effective Agents* (Dec 2024) — minimal-loop
  + workflows + sub-agent guidance.
- **Anthropic.** *How We Built Our Multi-Agent Research System* —
  isolated-context sub-agent pattern.
- **LangChain.** *Deep Agents* — `write_todos`, sub-agent `task` tool,
  virtual filesystem.
- **Claude Code.** `TodoWrite` tool semantics — full-list-replace
  contract.
- **OpenAI.** Agents SDK `Runner.run` + Responses API prompt-caching
  guide.
- **Cognition.** *Don't build multi-agents* — the contra view; informs
  why we cap depth at 2 and share budgets with the parent.
- **pi-agent (`earendil-works/pi`).** Sub-agent depth guard and
  default-exclude-self semantics borrowed verbatim into §3.3.
