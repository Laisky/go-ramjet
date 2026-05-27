# Proposal: Server-Side ReAct Agent Loop for gptchat

- **Status:** Draft (v2 — reframed around extensibility primitives from Codex & pi-agent)
- **Author:** Laisky (with Claude Code)
- **Created:** 2026-05-26
- **Affected services:** `internal/tasks/gptchat` (backend), `web/` (frontend)
- **Risk class:** Additive feature, opt-in per request. Existing proxy path is untouched.

---

## 1. Background

### 1.1 Current state

Today the gptchat backend operates as a **stateless transport** between the
frontend and an upstream OpenAI-compatible LLM. The "tool loop" already
present in
[responses_chat_handler.go:175-279](internal/tasks/gptchat/http/responses_chat_handler.go#L175-L279)
is a **relay loop**, not an agent loop:

- The backend forwards `Tools` and `Messages` to the upstream.
- The upstream LLM decides whether to call a tool.
- The backend executes the tool (local registry or remote MCP) and feeds the
  result back.
- The loop ends when the upstream returns an assistant message with no further
  tool calls.

All *reasoning* and *planning* lives in the upstream LLM. The backend holds no
ReAct policy, no system prompt, no termination condition of its own. This
proxy mode is stable and battle-tested; we explicitly do **not** want to
change it.

### 1.2 Motivation

We want an opt-in **"Agent mode"** in which the gptchat backend hosts a true
ReAct loop: the user submits a single goal-shaped prompt, the backend runs a
server-side reasoning loop with a curated tool belt (`web_search`,
`web_fetch`, `file_io/*`, `memory_*`) plus a terminal `send_to_user` tool, and
only returns to the user when the model invokes the exit tool.

The first shipping feature is intentionally small. **But the architecture is
not.** We design the foundations to host the agent system we'll want in
12 months — sub-agents, hooks, multi-provider routing, branching transcripts,
sandboxing — without rewriting the loop. Section 3 specifies those
primitives; section 4 specifies the small Phase 1 wiring on top of them.

### 1.3 Reference implementations we are stealing from

- **OpenAI Codex CLI** (`github.com/openai/codex`, Rust). We steal:
  submit/event split, unified `ToolRegistry`/`ToolRouter` covering
  local + MCP tools, JSON-RPC streaming protocol with typed events
  (their event taxonomy informs ours). We deliberately **diverge** on
  approval semantics (§3.7) — Codex retries with escalation inside the
  loop, we exit the loop and let the next user turn be the approval
  gate, because mid-stream confirmation contradicts §4.5's seamless-SSE
  contract. Codex events: `TurnStarted`, `ExecCommandBegin/End`,
  `PatchApplied`, `TokensUsed`, `TurnFinished`. Compaction in
  [`codex-rs/core/src/codex/compact.rs`](https://github.com/openai/codex/blob/main/codex-rs/core/src/codex/compact.rs).
- **pi-agent** (`earendil-works/pi`, TypeScript). Three-package monorepo
  (`pi-ai` / `pi-agent-core` / `pi-coding-agent`). The killer features for
  extensibility: an explicit `ExtensionAPI` with named hook points
  (`context`, `tool_call`, `session_before_compact`, `before_agent_start`,
  `session_start`, `session_switch`); typed `StreamEvent` union as the wire
  protocol; JSONL append-only transcripts with tree-shaped IDs (every entry
  has `id` + `parentId`) supporting branching; **sub-agent as a tool**
  (`pi-subagents` extension) with explicit per-child tool allowlists, depth
  guards, default-exclude-self, and file-mode output for large artifacts.

### 1.4 2026 design references

In addition to the two implementation references:

- **Loop caps**: 15-20 iterations, 30-50 tool calls per turn, 5-10 min wall
  clock, circuit-break on 2-3 consecutive identical tool calls
  ([Cordum on circuit breakers](https://cordum.io/blog/ai-agent-circuit-breaker-pattern)).
- **Exit-tool pattern**: explicit `send_to_user` with an "assistant message
  with no tool calls = implicit final" fallback
  ([Anthropic *Building Effective Agents*](https://www.anthropic.com/research/building-effective-agents),
  [Letta v1 agent loop](https://www.letta.com/blog/letta-v1-agent)).
- **Streaming UX**: SSE with typed events (`run_started`, `step_*`,
  `tool_call_*`, `tool_result`, `assistant_message_delta`, `run_finished`)
  influenced by AG-UI.
- **Tool-output truncation**: hard cap with clean-boundary truncation,
  ~25 k tokens for `web_fetch`
  ([Morph *MCP Output Too Large*](https://www.morphllm.com/mcp-output-too-large)).
- **Prompt injection defense**: wrap untrusted tool output in
  `<tool_result trust="untrusted">` delimiters; least-privilege sandbox for
  `file_io`; refuse new-path writes triggered by web content
  ([Anthropic *prompt-injection defenses*](https://www.anthropic.com/research/prompt-injection-defenses)).
- **Error handling**: feed errors back as `tool_result` with `is_error: true`;
  global error budget of 5-7 per loop, then abort.

---

## 2. Goals & Non-goals

### 2.1 Goals

- **Phase 1 capability**: per-request `agent_mode` switch from a new frontend
  toggle, a curated server-side tool belt, `send_to_user` exit, typed SSE
  trace.
- **Architectural foundations (must land in Phase 1, even if mostly unused)**:
  - `Session` with submit/event split.
  - Unified `Tool` interface and `ToolRegistry` covering local + MCP, with
    per-session tool **allowlists** so subset construction is a one-liner.
  - Append-only `Event` transcript with stable IDs and a `parentId` field
    (so future branching is a non-event).
  - `ModelClient` interface that wraps the upstream call, with the existing
    OneAPI/Responses path as the first implementation.
  - **Named `HookBus`** at six points (`session_start`, `context`,
    `before_tool_call`, `after_tool_call`, `before_compact`,
    `session_end`); memory integration is rewritten as two registered
    hooks rather than inline calls.
  - **`SubAgentTool` interface defined** (no implementation in Phase 1) so
    delegation lands without an API break later.
  - **`ErrAskUser` sentinel** — any hook can terminate the loop with a
    user-facing message instead of executing the in-flight tool call.
    The next user chat turn is the natural approval/redirect signal; no
    mid-stream confirmation protocol (§3.7).
  - **Bounded parallel tool execution** — when the model returns multiple
    `function_call` items in one round, the loop fans them out via a
    bounded executor (default `max_parallel_tool_calls=8`), preserves
    result order for the upstream, and cancels siblings on the first
    `ErrAskUser` (§3.8).
  - Loop caps, circuit breaker, error budget, write-gate — all enforced by
    hooks, not by core-loop conditionals.
- **Proxy invariance**: with `agent_mode=false`, the handler's externally
  observable behavior is byte-identical to the pre-change baseline.

### 2.2 Non-goals (for this change)

- Multi-agent orchestration / sub-agent **delegation execution** (interface
  defined now, implementation later).
- Persistent agent state across HTTP requests (each request = fresh
  `Session`; in-memory transcript only).
- Process sandboxing for tools (we rely on the MCP server's per-project
  namespace isolation and our existing `capToolOutput` byte caps).
- Replacing or deprecating the proxy path.
- New auth or billing tier — agent loop runs under the caller's existing
  token/quota.
- Configurable user-supplied tools beyond the curated belt (future work via
  the same `Tool` interface).

---

## 3. Architectural foundations (extensibility primitives)

Section 4 describes the small Phase 1 loop. **This section describes the
seams** — the interfaces that let that loop grow without being rewritten.
Every primitive here is named after the abstraction we are deliberately
porting from Codex or pi.

All primitives live in a new package tree:

```
internal/tasks/gptchat/agentx/
├── session/        // Session, Op, Event, transcript
├── tool/           // Tool interface, ToolRegistry, subset/allowlist
├── model/          // ModelClient interface, OneAPI implementation
├── hook/           // HookBus, named hook points
├── loop/           // The ReAct loop using session/tool/model/hook
├── tools/          // Concrete tool wrappers (mcp, send_to_user, future subagent)
└── prompt/         // System prompts (versioned constants)
```

### 3.1 Session: submit/event split *(from Codex)*

A `Session` is the per-request agent instance. The loop driver pushes
**Ops** in and subscribers pull **Events** out. This decouples the loop
from the HTTP/SSE writer, the trace renderer, and any future TUI/CLI.

```go
package session

type Op interface{ isOp() }
type OpUserTurn   struct{ Text string; Attachments []Blob }
type OpInterrupt  struct{}
type OpShutdown   struct{}

type Event interface {
    isEvent()
    // EventID is stable, ULID-shaped. ParentEventID is "" for root.
    EventID() string
    ParentEventID() string
}

type Session interface {
    Submit(ctx context.Context, op Op) error
    Events() <-chan Event       // multi-subscriber via internal fan-out
    Transcript() Transcript     // append-only view; safe to read concurrently
    Close() error
}
```

Why now: even though Phase 1 has exactly one in-flight `Op` per request
(`OpUserTurn`), routing through `Submit` keeps Interrupt and future
`OpRewind` / `OpBranch` from looking exotic later.

### 3.2 Tool: unified interface for local + MCP *(from both)*

Codex and pi both put local and MCP tools behind one interface. We do the
same so Phase 1's MCP-only belt is the same shape as future local tools.

```go
package tool

type Tool interface {
    Name() string
    Description() string
    Schema() jsonschema.Schema     // typed parameter schema (JSON Schema)
    Execute(ctx context.Context, call Call, sink EventSink) (Result, error)
}

type Call struct {
    CallID string
    Args   json.RawMessage
}

type Result struct {
    // Content is the LLM-facing rendering (string), already capped.
    Content string
    // Details is the structured payload for the UI/trace (optional, typed).
    Details json.RawMessage
    // IsError signals that this was a tool-level failure; the model sees the
    // error message but the loop's error budget is incremented.
    IsError bool
}

type Registry interface {
    Register(Tool)
    Get(name string) (Tool, bool)
    Names() []string
    // Subset returns a view restricted to the named tools. Returns an error
    // if any name is unknown. This is the primitive used by Subagent
    // allowlists and by the per-session belt assembly.
    Subset(names []string) (Registry, error)
}
```

Phase 1 implementation: a process-global `Registry` populated at startup
with MCP tools (discovered from the configured MCP server) and the
synthetic `send_to_user` tool. Each request constructs a per-session
`Subset` to enforce the curated belt. Future local-only tools, sandboxed
tools, and user-supplied MCP tools all register through the same path.

**Deterministic resolution rule (invariant, not an open question).**
The model is **never** shown two tools with the same name. The registry
maintains a single fixed source-priority order:

```
1. Local / synthesized tools  (e.g. send_to_user)
2. Curated MCP belt           (configured server in openai.agent_loop)
3. Future user-supplied MCP   (frontendReq.MCPServers, not used in Phase 1)
```

Within a given source, tools are ordered by `(server_id, tool_name)`
lexicographically. On registration, if a new tool's name already exists,
**the higher-priority tool wins; the lower-priority registration is
silently skipped** with a single structured warning log
(`agent_tool_shadowed`, fields: `name`, `kept_source`, `dropped_source`).
The same input always produces the same belt — there is no
non-determinism (no map iteration order, no first-seen-wins). `Names()`
returns the de-duplicated, sorted list; `Get(name)` returns exactly one
tool. This is enforced by unit test (U19 was extended; see §6.1).

We deliberately do **not** invent rename/disambiguate mechanics now — if
a future user-supplied tool collides with a curated one, the curated one
wins; if that becomes a pain point we revisit. The point of the rule is
reproducibility, not policy.

**Wrapping existing dispatch.** The existing `executeToolCall()` at
[responses_chat_handler.go:429-493](internal/tasks/gptchat/http/responses_chat_handler.go#L429-L493)
is wrapped as `tool.fromLegacyDispatch(name)` so Phase 1 reuses the local
+ MCP + `capToolOutput` path with zero risk to the proxy.

### 3.3 Append-only Event transcript with tree IDs *(from pi)*

The transcript is **never mutated**. Compaction produces *new* events
labelled `compacted`; branching creates child events with a different
`parentEventID`. Phase 1 keeps the transcript in memory only.

```go
type Transcript interface {
    Append(Event) error             // returns error if EventID already exists
    Events() []Event                // snapshot, in insertion order
    Tree() *TranscriptTree          // parent → children index
    Branch(fromEventID string) (Transcript, error)
    JSONL(w io.Writer) error        // future: persist to disk
}
```

Why now: Phase 1 doesn't branch, but if events are mutable arrays today,
adding branching is a refactor. Append-only with `parentEventID` is free
to implement and pays off the first time we want "rewind and try
differently."

Event types defined in Phase 1:

| Event                 | Carries                                                |
|-----------------------|---------------------------------------------------------|
| `RunStarted`          | RunID, ModelID, ToolNames, IterationCap                |
| `StepStarted`         | StepID, IterationIndex                                  |
| `AssistantTextDelta`  | StepID, Delta                                           |
| `AssistantReasoningDelta` | StepID, Delta (model reasoning, redacted by hook)  |
| `ToolCallStart`       | CallID, ToolName, ArgsPreview                           |
| `ToolCallEnd`         | CallID, DurationMS                                      |
| `ToolResult`          | CallID, ContentPreview, BytesTotal, IsError             |
| `StepFinished`        | StepID, TokensIn, TokensOut                             |
| `Final`               | FinalText, Citations, Origin (`send_to_user` \| `implicit` \| `ask_user`) |
| `RunFinished`         | RunID, TerminatedBy, TotalUsage                         |
| `Error`               | Code, Message                                           |

### 3.4 ModelClient abstraction *(from both)*

The loop only knows about an interface; the upstream call is one
implementation. Phase 1 ships exactly one (`OneAPIResponses`), but the
seam is in place for Anthropic-native, Gemini-native, or local-model
backends.

```go
package model

type Client interface {
    // Stream invokes the model and returns a typed event stream. The
    // implementation is responsible for SSE parsing and event typing.
    Stream(ctx context.Context, req Request) (<-chan Event, error)
    Capabilities() Capabilities  // supports_parallel_tool_calls, max_context, etc.
}

type Request struct {
    Model         string
    Input         []InputItem        // message + function_call + function_call_output items
    Tools         []ToolDescriptor   // names + schemas
    ToolChoice    any                // "auto" | "required" | {tool_name}
    MaxOutputTokens uint
    Reasoning     *Reasoning
    Stream        bool
}
```

Phase 1 `OneAPIResponses` implementation wraps the existing
`callUpstreamResponses` so the upstream path, headers, and rate-limit
behavior are identical to today.

### 3.5 Named HookBus *(from pi `ExtensionAPI`)*

The single highest-leverage primitive. Every future cross-cutting concern
(memory, redaction, telemetry, budget enforcement, audit, write-gate,
PII scrubbing, prompt-injection scanning) lands as a hook. The core loop
stays small.

```go
package hook

type Point string
const (
    PointSessionStart   Point = "session_start"
    PointContext        Point = "context"             // rewrite input before model call
    PointBeforeToolCall Point = "before_tool_call"    // gate / rewrite args; return error to deny
    PointAfterToolCall  Point = "after_tool_call"     // observe / mutate result
    PointBeforeCompact  Point = "before_compact"
    PointSessionEnd     Point = "session_end"
)

type ContextEvent struct {
    Input []model.InputItem  // hooks return a (possibly modified) copy
}

type ToolCallEvent struct {
    ToolName string
    CallID   string
    Args     json.RawMessage
    Result   *tool.Result  // nil for "before", populated for "after"
}

type Bus struct{ /* registry keyed by Point */ }

func (b *Bus) OnContext(h func(context.Context, ContextEvent) (ContextEvent, error))
func (b *Bus) OnBeforeToolCall(h func(context.Context, ToolCallEvent) (ToolCallEvent, error))
func (b *Bus) OnAfterToolCall(h func(context.Context, ToolCallEvent) (ToolCallEvent, error))
// … one method per point
```

**Phase 1 hooks installed:**

| Hook                  | Implementation                                                          | Source            |
|-----------------------|-------------------------------------------------------------------------|-------------------|
| `OnContext`           | Inject ReAct system prompt and per-round budget hint                     | `agentx/prompt`   |
| `OnContext`           | Run existing `memoryx.BeforeTurnHook` (replaces inline call)             | `agentx/tools/memoryhook` |
| `OnSessionEnd`        | Run existing `memoryx.AfterTurnHook` with only `(prompt, final)`         | `agentx/tools/memoryhook` |
| `OnBeforeToolCall`    | Circuit breaker: hash `(tool_name, normalized_args)`, deny on 3-repeat   | `agentx/loop/circuit` |
| `OnBeforeToolCall`    | Write-gate enforcement (`file_write`/`file_delete`/`file_rename`)        | `agentx/loop/writegate` |
| `OnAfterToolCall`     | Output cap via existing `capToolOutput` (wrapper)                         | `agentx/tools/cap` |
| `OnAfterToolCall`     | Wrap output in `<tool_result trust="untrusted">…</tool_result>`           | `agentx/loop/wrap` |
| `OnAfterToolCall`     | Increment error budget on `IsError`; emit termination if exceeded         | `agentx/loop/budget` |

Each hook is independently unit-testable. Adding "PII scrubber" later is a
new hook file, not a core change.

### 3.6 Sub-agent as a Tool *(from `pi-subagents`)*

We do **not** implement subagent execution in Phase 1, but we define the
interface and reserve the tool name so Phase ≥ 2 is non-breaking. Locking
this design now lets us also pre-wire the parent → child event forwarding
in the streaming protocol.

```go
package tools

type SubAgentArgs struct {
    Profile     string   `json:"profile"`            // e.g. "researcher" | "coder"
    Task        string   `json:"task"`
    AllowTools  []string `json:"allow_tools"`        // subset of parent registry
    OutputMode  string   `json:"output_mode"`        // "inline" | "file" | "none"
}

type SubAgentTool struct {
    Registry      tool.Registry
    Models        model.Registry
    HookBus       *hook.Bus
    MaxDepth      int     // default 2
    DefaultBudget int     // iteration cap for child
}
```

Properties we lock in from day one (copied verbatim from `pi-subagents`):

- **Explicit allowlist** — child sees only `AllowTools`, never inherit-all.
- **Depth guard** — refuses spawn when `parentDepth(ctx) >= MaxDepth`.
- **Default-exclude-self** — child never gets `SubAgentTool` in its
  allowlist unless explicitly granted, preventing recursive escape.
- **File-mode output** — for outputs larger than N tokens, return a path
  reference via `file_write` rather than blowing up the parent transcript.
- **Event forwarding** — child events flow into the parent's `Events()`
  channel with `ParentEventID` pointing at the spawning `ToolCallStart`.

In Phase 1, `SubAgentTool.Execute` returns
`errors.New("subagent execution not enabled in this build")` and the tool
is **not** registered. The type exists so callers can compile against it.

### 3.7 Approval = loop exit, not mid-stream confirmation

We deliberately **diverge** from Codex's mid-stream retry-with-escalation
pattern. Mid-loop user confirmation requires synchronous client
round-trips, new ops (`OpApproval`), bidirectional SSE, and stateful
streaming — all of which contradict the "seamless integration"
contract from §4.5. Instead, we use the **conversation turn boundary**
as the natural approval gate: any hook that wants user approval just
**terminates the loop early** with a structured prompt to the user, and
the user's next chat message becomes the approval (or correction, or
redirection). No intermediate states, no special ops, no protocol
extensions.

Mechanism: a typed sentinel error returned from any hook causes the loop
to skip the in-flight tool call and emit a hook-driven `Final` event
whose text is the hook's user-facing message.

```go
package hook // lives in agentx/hook/errors.go to avoid the loop→hook→loop cycle

// ErrAskUser, returned from any hook, exits the loop and produces a
// Final event whose text is `Message`. Equivalent to the model calling
// send_to_user with that message — same wire format, same UX. The loop
// detects it via errors.As(err, &hook.ErrAskUser{}).
type ErrAskUser struct {
    Code    string // structured code for telemetry: "write_gate" | "circuit_breaker" | ...
    Message string // user-facing prompt; rendered as the assistant message
    Details map[string]any // optional structured context (proposed tool call, args, etc.)
}

func (e *ErrAskUser) Error() string { return e.Code + ": " + e.Message }
```

The `Result` struct stays minimal — no `NeedsEscalation` field, no
`EscalationCode`:

```go
type Result struct {
    Content string
    Details json.RawMessage
    IsError bool
}
```

#### How hooks use it

A hook decides "this needs user approval" and returns `ErrAskUser`
instead of a `Result`. The loop catches it in `RunFinished` with
`TerminatedBy="ask_user"` and emits:

1. `emitThinkingDelta` — one `[[TOOLS]] ask_user(code=write_gate): blocked by policy\n` line for the trace.
2. `emitTextDelta` — the `Message` text, chunked, lands in `delta.content`.
3. Finish chunk with `finish_reason: "stop"`.

The user sees a normal assistant message. They reply naturally:

- "Yes, proceed with that file write." → next loop run reads the prior turn's `ask_user` Final as context and the model retries (now with implicit human approval in the transcript).
- "No, do it differently." → model picks a new strategy.
- "Actually, write to `/tmp/foo` instead." → model uses the corrected target.

No special user UI, no buttons, no out-of-band approval channel. The
conversation **is** the approval mechanism.

#### Phase 1 hook that uses this

Write-gate (`OnBeforeToolCall` for `file_write` / `file_delete` /
`file_rename`) with default mode `ask`:

```go
func writeGateAsk(ctx context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
    if !isWriteTool(ev.ToolName) { return ev, nil }
    return ev, &loop.ErrAskUser{
        Code:    "write_gate",
        Message: fmt.Sprintf(
            "I want to call `%s` with arguments:\n\n```json\n%s\n```\n\nShould I proceed? Reply 'yes' to confirm, or tell me a different approach.",
            ev.ToolName, prettyJSON(ev.Args)),
        Details: map[string]any{"tool": ev.ToolName, "args": ev.Args},
    }
}
```

#### Other future hooks that can use it

- **Untrusted-origin write** — `web_fetch` result contains a path that
  the model now wants to `file_write` to without the user naming it.
- **Sensitive-content scanner** — a future PII / secret-detector hook
  finds API keys in the tool output and refuses to feed it back.
- **Budget-aware advisor** — a hook that fires when iteration count is
  high and tool calls are still escalating; surfaces "I've used 18/20
  steps. Should I stop and summarize what I have?"

All of these use the same `ErrAskUser` mechanism — no new code paths.

#### What we lose vs. Codex's pattern

In Codex's retry-with-escalation, the model can sometimes recover
*within* the same loop without bothering the user (re-issuing with
justification that satisfies a heuristic gate). We don't get that. We
accept the cost because: (a) the alternative is protocol complexity
we'd carry forever, (b) for high-stakes operations (file writes), one
extra user turn is the right friction, (c) low-stakes recovery still
works because the *model* can recover by choosing a different tool —
the gate only fires on the high-stakes ones we configured.

### 3.8 Parallel tool execution within a round

The OpenAI Responses API can return multiple `function_call` items in a
single `required_action`. Modern frontier models actively exploit this
to dispatch independent reads (e.g., three parallel `web_fetch`s on
different URLs) and the latency win is substantial. We support it from
Phase 1.

```go
package loop

type parallelExecutor struct {
    bus      *hook.Bus
    registry tool.Registry
    sink     session.EventSink
    maxConc  int           // bounded — default 8, from agent_loop.max_parallel_tool_calls
    budget   *budget.Counter // global error/call counter, atomic
}

// ExecuteAll dispatches the calls concurrently, preserves input order
// in the returned outputs (so the upstream sees a stable mapping), and
// returns ErrAskUser if any sibling hook requested user approval.
func (p *parallelExecutor) ExecuteAll(
    ctx context.Context,
    calls []model.FunctionCall,
) ([]model.FunctionCallOutput, error) { … }
```

#### Invariants

1. **Bounded concurrency.** A semaphore of size `max_parallel_tool_calls`
   limits in-flight goroutines. Default 8. Set to `1` to force the
   sequential execution path (useful for repro / debugging /
   conservatively-configured deployments). Single source of truth for
   the cap is `openai.agent_loop.max_parallel_tool_calls`.
2. **Stable upstream order.** Results are written into a pre-allocated
   `[]FunctionCallOutput` slice keyed by the call's input index. The
   upstream LLM sees outputs in the same order it emitted the calls,
   regardless of which goroutine finished first. This is what keeps
   model behavior deterministic from one rerun to the next.
3. **First-ask-user wins; siblings cancelled.** If any concurrent call's
   `OnBeforeToolCall` or `OnAfterToolCall` hook returns `ErrAskUser`,
   the executor immediately cancels the shared context, drains
   outstanding goroutines (best effort — they observe `ctx.Err()` and
   return), and surfaces that single `ErrAskUser` upward. Already-
   completed sibling results are discarded — they do **not** appear in
   the transcript, because the loop is about to exit with the
   ask-user prompt as the Final. This avoids a half-applied tool call
   showing up as historical context if the user later declines.
4. **Concurrency-safe primitives.**
   - Circuit-breaker state: `sync.Mutex` around the
     `(tool_name, normalized_args)` repeat counter.
   - Error budget: `atomic.Int64`.
   - HookBus: hooks are registered once at startup (read-only at
     runtime); each call's hook chain runs through its own goroutine
     reading the immutable hook slice — no locks needed on the bus
     itself.
   - SSE writer: already serialized by `gmw.CtxLock` on the gin
     context, so writes from parallel goroutines interleave safely at
     the chunk boundary.
5. **Trace disambiguation in the existing SSE channel.** Per §4.5, tool
   trace lines stream as `[[TOOLS]] …\n` deltas on
   `delta.reasoning_content`. With concurrent calls, lines from
   different tools interleave. The Phase 1 formatter prefixes every
   line with the 6-char short call ID:

   ```
   [[TOOLS]] [a1b2c3] tool_call: web_search
   [[TOOLS]] [d4e5f6] tool_call: web_fetch
   [[TOOLS]] [a1b2c3] args: {"query":"…"}
   [[TOOLS]] [d4e5f6] args: {"url":"…"}
   [[TOOLS]] [d4e5f6] tool ok (8421B)
   [[TOOLS]] [a1b2c3] tool ok (12345B)
   ```

   Readable to humans (pair by ID), and trivially threadable by the
   Phase 2 typed-event UI which gets the full call_id on every event.
6. **Model capability gate.** `ModelClient.Capabilities()` reports
   `SupportsParallelToolCalls`. When the upstream is configured against
   a model that does not (older Anthropic 3.x, some local Llamas), the
   loop sets `parallel_tool_calls: false` on the request and the
   executor's input is naturally a single-element slice — fan-out
   becomes a no-op without code-path divergence.
7. **Per-call hooks fire normally.** Each parallel call runs the full
   `OnBeforeToolCall` → execute → `OnAfterToolCall` chain
   independently. No "batch hook" notion. Hook authors do not need to
   know about parallelism.

#### Interaction with existing freetier rate limiter

The existing `expensiveModelRateLimiter` at
[responses_chat_handler.go:461-471](internal/tasks/gptchat/http/responses_chat_handler.go#L461-L471)
is global and per-call. Concurrent calls from a single freetier session
will naturally serialize through it — bounded parallelism interacts
correctly with the limiter without changes. We do **not** loosen the
limiter for agent mode; if a freetier user trips it mid-batch, the
affected call gets an `IsError` result and the model sees it on the
next round.

#### Why default = 8, not unbounded

A bounded executor with size 8 captures ~95% of the practical
latency win (most parallel batches are 2-5 calls) while preventing a
runaway model that emits 30 `function_call`s in one round from
saturating the MCP backend or the freetier limiter. The cap is
configurable so deployments with beefier MCP fanout can raise it.

---

## 4. High-level Phase 1 wiring

### 4.1 Request flow

```
Frontend (Agent toggle ON)
  │
  ▼
POST /api  { laisky_extra.chat_switch.agent_mode = true }
  │
  ▼
ChatHandler (existing)
  │
  ├── agent_mode? ── no ──▶ existing proxy path (UNCHANGED)
  │
  └── yes
        ▼
   agentx.NewSession(SessionConfig{
       User, Registry, ModelClient, HookBus,
       Caps{ MaxIter, MaxToolCalls, WallClock, ErrorBudget },
   })
        │
        ├── session.Submit(OpUserTurn{Text})
        │
        ▼
   loop.Run(session)
        │
        ├── hook.PointSessionStart fires
        ├── for iteration in 0..MaxIter:
        │     ├── hook.PointContext fires (memory inject + sys prompt + budget hint)
        │     ├── model.Stream(ctx, request)  → consume stream → emit events
        │     ├── extract function_calls
        │     ├── if send_to_user → emit Final, break
        │     ├── if no tool_calls → implicit-final fallback, break
        │     ├── parallelExecutor.ExecuteAll(calls) — bounded fan-out
        │     │     concurrently for each call:
        │     │     ├── hook.PointBeforeToolCall (circuit, write-gate)
        │     │     ├── tool.Execute (wraps existing executeToolCall)
        │     │     ├── hook.PointAfterToolCall (cap, wrap, budget)
        │     │     └── store function_call_output[i] (preserves order)
        │     ├── append all outputs to next iteration's input
        │     └── if any hook returned ErrAskUser: emit Final{Message}, break
        │
        ├── hook.PointSessionEnd fires (memory persist)
        └── emit RunFinished{TerminatedBy: …}
```

### 4.2 Key Phase 1 decisions

1. **Branching, not a new route.** `agent_mode` is a request flag, not a
   new endpoint. Authentication, rate limits, and observability continue
   unchanged. The branch is one `if` at the top of `ChatHandler`.
2. **Reuse `executeToolCall` via wrapper.** Existing local + MCP + cap
   logic stays the source of truth. `tool.fromLegacyDispatch(name)`
   produces a `Tool` that delegates to it. Zero-risk path for the proxy.
3. **Force-enable MCP inside the loop.** Loop constructs an internal
   *copy* of the request with `EnableMCP=true` so the curated belt can
   execute even if the user left the MCP toggle off. The caller's
   request object is not mutated.
4. **Explicit `send_to_user` + implicit fallback.** Model is told to call
   `send_to_user(final_answer, citations?)` to finish. An assistant
   message with no tool calls is treated as implicit final.
5. **Caps default to 2026 conservative values.**
   `max_iterations=20`, `max_tool_calls=40`, `wall_clock_seconds=480`,
   `circuit_breaker_repeats=3`, `error_budget=6`. All configurable.
6. **Streaming uses only the existing SSE channel.** No new event types,
   no SSE parser changes on the frontend, no new wire protocol in
   Phase 1. The agent loop emits via the already-shipping
   `emitTextDelta` (→ `delta.content`) and `emitThinkingDelta`
   (→ `delta.reasoning_content`) functions, exactly as the current
   proxy tool loop does today. See §4.5 for the full mapping.
7. **Memory hooks** live in `agentx/tools/memoryhook` and register on
   the bus. The inline calls at
   [responses_chat_handler.go:739](internal/tasks/gptchat/http/responses_chat_handler.go#L739)
   and [:309](internal/tasks/gptchat/http/responses_chat_handler.go#L309)
   are **not** modified; the agent path goes through the hook bus
   instead. Same `BeforeTurnHook` / `AfterTurnHook` functions are
   invoked, just from a different call site.

### 4.3 The Phase 1 curated tool belt

| Tool name                                   | Provenance             | Notes |
|---------------------------------------------|------------------------|-------|
| `web_search`                                 | MCP (`laisky`)         | Output capped. |
| `web_fetch`                                  | MCP (`laisky`)         | Output capped to 25 k tokens. |
| `file_list`, `file_stat`, `file_read`        | MCP (`laisky`)         | Default project `go-ramjet`. |
| `file_search`                                | MCP (`laisky`)         | Hybrid retrieval. |
| `file_write`, `file_delete`, `file_rename`   | MCP (`laisky`)         | Gated by `write_gate` (default `ask` — exits loop with a confirmation prompt; see §3.7). |
| `send_to_user`                               | Local (synthesized)    | Exit tool. Args: `{ final_answer: string, citations?: Citation[] }`. |

Schemas are discovered once at startup, cached, and re-emitted into the
upstream `tools` array for every agent request.

### 4.4 System prompt

A short version-controlled system prompt is prepended via the
`OnContext` hook. It contains:

- The ReAct directive.
- The exit-tool contract.
- The untrusted-content delimiter guard
  (`<tool_result trust="untrusted">`).
- A budget hint re-injected each iteration ("you have at most N steps
  remaining") so the model paces itself.

Stored at `internal/tasks/gptchat/agentx/prompt/react.go` as a single
versioned constant with `Version int` and templated per-round.

### 4.5 Streaming compatibility (the seamless-integration contract)

The user-visible answer to "does this integrate seamlessly with current
gptchat and stream?" is **yes, and Phase 1 needs zero changes to the
frontend SSE parser or message renderer.** This subsection nails down
exactly why.

#### 4.5.1 The existing wire format we are reusing

The proxy path already streams *three* kinds of bytes to the browser:

| Direction                 | Backend emitter                                              | Wire (`OpenaiCompletionStreamResp`) | Frontend handler                                   |
|---------------------------|--------------------------------------------------------------|--------------------------------------|----------------------------------------------------|
| Final assistant text      | `emitTextDelta` ([handler:588](internal/tasks/gptchat/http/responses_chat_handler.go#L588-L603))     | `choices[0].delta.content`           | `delta.content` → `onContent` → message body       |
| Reasoning / trace text    | `emitThinkingDelta` ([handler:571](internal/tasks/gptchat/http/responses_chat_handler.go#L571-L586)) | `choices[0].delta.reasoning_content` | `delta.reasoning_content` → `onReasoning` ([api.ts:348-352](web/src/utils/api.ts#L348-L352)) → reasoning panel |
| Final completion / finish | `writeChatCompletionChunk`                                   | `choices[0].finish_reason`           | `finish_reason` → `onFinish` / `onDone`            |

The existing proxy tool loop **already** streams tool call markers via
this channel today — at
[responses_chat_handler.go:233-267](internal/tasks/gptchat/http/responses_chat_handler.go#L233-L267)
it emits things like:

```
[[TOOLS]] Upstream tool_call: web_search
[[TOOLS]] args: {"query": "anthropic claude blog"}
[[TOOLS]] tool ok
```

…each one written through `emitThinkingDelta` and rendered live in the
frontend's reasoning panel. This is the foundation we extend.

#### 4.5.2 Mapping from internal `Event`s to wire chunks

The typed `session.Event` stream from §3.3 is **internal** — used for
testing, future branching/persistence, and a possible Phase 2 typed
channel. For Phase 1 the SSE encoder (`agentx/sse.go`) reduces every
internal event to one of the existing emitters:

| Internal event              | Phase 1 wire mapping                                                                                  |
|-----------------------------|--------------------------------------------------------------------------------------------------------|
| `RunStarted`                | `emitThinkingDelta("[[TOOLS]] agent run started (model=…, iter_cap=…)\n")`                              |
| `StepStarted{i}`            | `emitThinkingDelta("[[TOOLS]] -- step " + i + " --\n")`                                                 |
| `AssistantReasoningDelta`   | `emitThinkingDelta(delta)` — the model's own reasoning, streamed as-is                                  |
| `AssistantTextDelta`        | **dropped during loop iterations** (model's intermediate prose is not the final answer); see note      |
| `ToolCallStart{call_id, name, args}` | `emitThinkingDelta("[[TOOLS]] [" + short(call_id) + "] tool_call: " + name + "\n[[TOOLS]] [" + short(call_id) + "] args: " + args + "\n")` — 6-char ID prefix disambiguates parallel calls |
| `ToolCallEnd{duration}`     | (no emit — folded into `ToolResult`)                                                                    |
| `ToolResult{call_id, ok}`   | `emitThinkingDelta("[[TOOLS]] [" + short(call_id) + "] tool ok (" + bytes + "B)\n" or "tool error: " + msg + "\n")` |
| `StepFinished`              | (no emit in Phase 1 — only logged)                                                                      |
| `Final{text}`               | `emitTextDelta(chunk)` — token-by-token over a small `chunkString` window, lands in `delta.content`     |
| `RunFinished`               | One final `writeChatCompletionChunk` with `finish_reason="stop"`                                        |
| `Error{code,msg}`           | `emitThinkingDelta("[[TOOLS]] error: " + code + " — " + msg + "\n")` then finish-reason `"stop"`        |

Note on intermediate `AssistantTextDelta`: during a multi-step loop, the
model often produces interim "thinking out loud" text *before* deciding
to call a tool. Streaming that to `delta.content` would pollute the
final answer in the UI. We route it to `reasoning_content` instead so
the user sees it in the reasoning panel; only the `send_to_user` payload
reaches `delta.content`.

#### 4.5.3 What the frontend sees in Phase 1

For a normal multi-tool agent run, the browser receives the same
sequence shape it gets today for any tool-using upstream model:

```text
data: {"choices":[{"delta":{"reasoning_content":"[[TOOLS]] agent run started …\n"}}]}
data: {"choices":[{"delta":{"reasoning_content":"[[TOOLS]] tool_call: web_search\n"}}]}
data: {"choices":[{"delta":{"reasoning_content":"[[TOOLS]] args: {\"query\":\"…\"}\n"}}]}
data: {"choices":[{"delta":{"reasoning_content":"[[TOOLS]] tool ok (12345B)\n"}}]}
data: {"choices":[{"delta":{"reasoning_content":"[[TOOLS]] tool_call: web_fetch\n"}}]}
…
data: {"choices":[{"delta":{"content":"The latest Anthropic blog post …"}}]}
data: {"choices":[{"delta":{"content":" introduces …"}}]}
data: {"choices":[{"finish_reason":"stop"}]}
data: [DONE]
```

The existing `onReasoning` callback renders every `[[TOOLS]] …` line in
the reasoning panel; `onContent` renders the final answer in the
message body. **Tool calls are visible without any frontend code
change.**

#### 4.5.4 Backpressure, flushing, locking

We reuse `writeChatCompletionChunk` ([handler:621](internal/tasks/gptchat/http/responses_chat_handler.go#L621))
verbatim. It already:

- Acquires the per-request gin write lock via `gmw.CtxLock`.
- Writes the `data: ` prefix and the JSON payload.
- Flushes after each chunk so SSE intermediaries don't buffer.

The agent loop's `sse.go` writer holds a reference to the gin context
and emits in the same goroutine as the loop driver, so write ordering
matches event ordering naturally — no separate fan-out worker needed
in Phase 1.

#### 4.5.5 Why this is "seamless"

A client opened against the dev server today, with no frontend rebuild,
will already render every Phase 1 agent run correctly — tool calls,
reasoning, and the final answer — *provided* it can submit the
`agent_mode: true` flag. The flag is the only mandatory frontend
change. The "Agent" toggle button (§5.3) is added in the same PR for
ergonomics, but a developer can already trigger agent mode today by
hand-crafting a request and watch it render.

#### 4.5.6 Phase 2 enrichment (not blocking Phase 1)

When we later want richer UI (collapsible tool cards, structured
citations, copy-buttons on tool outputs), we add a second SSE channel
using `event: agent` named frames carrying the typed `Event` JSON.
Phase 1 clients ignore unknown SSE event types per the SSE spec, so
deployment is non-breaking. The internal typed `Event` stream from §3.3
becomes the source of truth for that channel without code refactors.

---

## 5. Change list

### 5.1 New backend packages and files

| Path | Purpose |
|---|---|
| `internal/tasks/gptchat/agentx/session/session.go` | `Session`, `Op`, `Event`, `Transcript`. |
| `internal/tasks/gptchat/agentx/session/transcript.go` | Append-only event log with `parentEventID` tree. |
| `internal/tasks/gptchat/agentx/tool/tool.go` | `Tool` interface, `Result`, `Call`. |
| `internal/tasks/gptchat/agentx/tool/registry.go` | `Registry` with `Subset` allowlist. |
| `internal/tasks/gptchat/agentx/tool/legacy.go` | `fromLegacyDispatch` wrapper over `executeToolCall`. |
| `internal/tasks/gptchat/agentx/model/client.go` | `model.Client` interface, `Request`, streaming `Event`. |
| `internal/tasks/gptchat/agentx/model/oneapi.go` | First implementation, wraps `callUpstreamResponses`. |
| `internal/tasks/gptchat/agentx/hook/bus.go` | `Bus`, `Point`, event types per point. |
| `internal/tasks/gptchat/agentx/loop/loop.go` | `Run(session) error`. The actual ReAct loop. |
| `internal/tasks/gptchat/agentx/loop/circuit.go` | Circuit-breaker hook. |
| `internal/tasks/gptchat/agentx/loop/budget.go` | Iteration / call / wall-clock / error counters (atomic-safe for parallel use). |
| `internal/tasks/gptchat/agentx/loop/parallel.go` | Bounded parallel executor (§3.8). Fan-out with semaphore, order-preserving fan-in, first-ask-wins cancellation. |
| `internal/tasks/gptchat/agentx/loop/writegate.go` | Write-class tool denial hook. |
| `internal/tasks/gptchat/agentx/loop/wrap.go` | Untrusted-content delimiter hook. |
| `internal/tasks/gptchat/agentx/tools/send_to_user.go` | Synthetic exit tool. |
| `internal/tasks/gptchat/agentx/tools/memoryhook.go` | `OnContext` and `OnSessionEnd` hooks calling `memoryx.*`. |
| `internal/tasks/gptchat/agentx/tools/subagent.go` | `SubAgentTool` type (Phase 1: unimplemented `Execute`). |
| `internal/tasks/gptchat/agentx/prompt/react.go` | Versioned system prompt + per-round template. |
| `internal/tasks/gptchat/agentx/sse.go` | SSE writer: default channel for token deltas, `event: agent` channel for typed events. |
| `internal/tasks/gptchat/agentx/handler.go` | `HandleAgent(ctx *gin.Context, req *FrontendReq, user *UserConfig) error` — top-level entry called from `ChatHandler`. |
| `docs/proposals/2026-05-26-gptchat-react-agent-mode.md` | This document. |

Unit tests next to each file. See §6.

### 5.2 Modified files (backend)

| Path | Change |
|---|---|
| `internal/tasks/gptchat/http/dto.go` (lines 76-86) | Add `AgentMode *bool` to `LaiskyExtra.ChatSwitch`. Pointer ⇒ absent ≡ false. |
| `internal/tasks/gptchat/http/responses_chat_handler.go` (top of `ChatHandler`) | Single conditional: if `AgentMode != nil && *AgentMode`, delegate to `agentx.HandleAgent` and return. Otherwise fall through unchanged. |
| `internal/tasks/gptchat/config/config.go` | Add `AgentLoop` block (see §5.4). |
| `internal/tasks/gptchat/http/common.go` | Export `getRawUserToken` and `executeToolCall` (rename/visibility only) so `agentx/tool/legacy.go` can call them. No behavior change. |
| `internal/tasks/gptchat/http/mcp_client.go` | Add `discoverTools(server)` helper returning `[]ToolDescriptor` so the agent registry can populate at startup. Pure addition; no existing call paths touched. |

### 5.3 Modified files (frontend) — Phase 1 minimal set

Phase 1 reuses the existing `delta.reasoning_content` / `delta.content`
SSE channels (see §4.5), so the SSE parser, message renderer, and
reasoning panel need **no changes**. Only three files touched:

| Path | Change |
|---|---|
| `web/src/pages/gptchat/types.ts` (`ChatSwitch` at lines 73-80) | Add `agent_mode: boolean`. |
| `web/src/pages/gptchat/components/chat-input.tsx` (toggle row, lines 418-490) | Add `<ToggleButton>` labeled "Agent" after the Memory toggle. Robot icon from lucide-react. Default OFF. |
| `web/src/pages/gptchat/hooks/chat-streaming.ts` (around line 365) | Add `agent_mode: config.chat_switch.agent_mode` inside `laisky_extra.chat_switch`. |

Files **not** modified in Phase 1 (kept for Phase 2 enrichment):

- `web/src/utils/api.ts` — SSE parser stays untouched.
- `web/src/pages/gptchat/hooks/use-chat.ts` — message state shape unchanged.
- No new `agent-trace.tsx` component.

In Phase 2 (typed `event: agent` channel), we will add an
`onAgentEvent` SSE handler, an `agentTrace` field on assistant messages,
and a collapsible `agent-trace.tsx` component. None of those block
Phase 1.

### 5.4 Configuration

```yaml
openai:
  agent_loop:
    enabled: true                  # global kill-switch
    max_iterations: 20
    max_tool_calls: 40
    max_parallel_tool_calls: 8     # bounded fan-out within a round (§3.8)
                                   # set to 1 to force sequential execution
    wall_clock_seconds: 480
    circuit_breaker_repeats: 3
    error_budget: 6
    write_gate: ask                # one of: ask | allow | deny
                                   # ask  — exits loop with a confirmation prompt; user's
                                   #         next chat message is the approval (default)
                                   # allow — write tools execute unconditionally
                                   # deny  — write tools always return IsError to the model
                                   #         (loop continues, model picks another approach)
    mcp_server: laisky             # alias of an entry in openai.mcp_servers
    web_fetch_max_tokens: 25000
    default_file_project: go-ramjet
    subagent:
      enabled: false               # interface defined, execution off
      max_depth: 2
```

When the block is absent, defaults apply but `agent_mode` requests return
HTTP 409 `agent_mode_disabled` (a recoverable state surfaced in the UI).

---

## 6. Test matrix

### 6.1 Unit tests

| ID | Subject | Setup | Expected |
|----|---------|-------|----------|
| U1 | Happy path | Mock `model.Client` returns one `send_to_user` call | `Final` + `RunFinished{TerminatedBy: send_to_user}`; `AfterTurnHook` called once with `(prompt, final)`. |
| U2 | Multi-round | Mock returns `web_search` → `web_fetch` → `send_to_user` | Three `StepStarted`/`StepFinished`, two `ToolResult`, final equals round-3 answer. |
| U3 | Iteration cap | Mock never calls `send_to_user` | `RunFinished{TerminatedBy: iteration_cap}` at round 20; the loop-cap hook injects the "summarize now" tool output before final iteration. |
| U4 | Wall-clock cap | `wall_clock_seconds=1`, tool sleeps 2 s | `RunFinished{TerminatedBy: timeout}`. |
| U5 | Circuit breaker | Mock returns identical `web_search` 3× in a row | Third call denied by `OnBeforeToolCall` with a synthetic `IsError` result (`Content="repeated tool call detected"`); loop continues; if pattern persists, eventual `error_budget` termination. |
| U6 | Tool error recovery | Round 1 tool errors; round 2 different tool; round 3 `send_to_user` | Loop succeeds; error budget at 1. |
| U7 | Error budget | 7 consecutive errored tools | `RunFinished{TerminatedBy: error_budget}`. |
| U8 | Implicit final | Mock returns assistant message, no tool calls | Loop emits `Final` with that text; `TerminatedBy: implicit_final`. |
| U9 | `send_to_user` malformed | Args fail schema validation | Tool returns `IsError=true`; error budget +1; not treated as final. |
| U10 | Delimiter escaping | Tool output contains literal `</tool_result>` | Encoder escapes; system prompt's guard remains parseable. |
| U11 | Output cap | `web_fetch` returns 100 k tokens | Stored output ≤ `web_fetch_max_tokens`; truncation marker present. |
| U12 | Tool belt construction | Discovery returns 15 tools | Per-session `Registry.Subset` contains exactly the curated belt + `send_to_user`. |
| U13 | EnableMCP isolation | Request `enable_mcp=false`, `agent_mode=true` | Loop's internal copy has `EnableMCP=true`; original `frontendReq` untouched after loop returns. |
| U14 | Memory disabled | `EnableMemory=false` | `OnContext` memory hook is a no-op; loop runs. |
| U15 | Memory enabled | Paid tier, `EnableMemory=true` | `OnSessionEnd` calls `AfterTurnHook` once with `(user_prompt, final_answer)` only; intermediate tool turns NOT in the payload. |
| U16 | Write-gate `ask` exits loop | `write_gate=ask`, model calls `file_write` | Hook returns `ErrAskUser{Code: "write_gate"}`; loop emits Final with the confirmation prompt as message body, then `RunFinished{TerminatedBy: "ask_user"}`. The `file_write` tool is never invoked. SSE wire format matches `send_to_user` exactly (delta.content + finish_reason=stop). |
| U16b | Write-gate `deny` (alternate mode) | `write_gate=deny`, model calls `file_write` | Hook returns a synthetic `Result{IsError: true, Content: "write tools are disabled in this session"}`; loop continues; error budget +1; model picks another path. |
| U16c | Write-gate `allow` (alternate mode) | `write_gate=allow`, model calls `file_write` | No gate fires; tool executes normally. |
| U17 | Streaming order | Five-round happy path | Strict event order: `RunStarted → (StepStarted → AssistantTextDelta* → ToolCallStart → ToolResult → StepFinished)* → Final → RunFinished`. Verified by golden file. |
| U18 | Transcript append-only | Direct test of `Transcript.Append` | Second append with same `EventID` returns error; no events are removed across loop lifecycle. |
| U19 | Registry `Subset` & deterministic resolution | (a) Subset with unknown tool name. (b) Register two tools named `web_search` from different sources, repeat 100 times with shuffled input order. | (a) Returns error; startup fails-loud if any curated name is missing. (b) `Get("web_search")` returns the higher-priority tool on every run; `Names()` returns the same de-duplicated sorted list every run; the shadow warning log fires exactly once per duplicate. |
| U20 | `SubAgentTool` reservation | Resolve tool by name `spawn_agent` | Tool exists in registry only when `subagent.enabled=true`. With default config, name resolves to `(nil, false)`. |
| U21 | HookBus ordering | Register two `OnContext` hooks A then B | B receives A's output; output passed to model is B's transformation. |
| U22 | HookBus deny | `OnBeforeToolCall` returns an error | Tool not executed; loop receives a synthetic `Result{IsError: true}`; error budget +1. |
| U23 | Parallel fan-out happy path | Mock model returns 4 parallel `function_call` items in one round; tools sleep [200ms, 50ms, 150ms, 100ms] | All four `Execute` calls run concurrently (verified by start-time delta < 30ms); upstream sees outputs in original input order; total round latency ≈ 200ms (not 500ms). |
| U24 | Parallel bounded concurrency | `max_parallel_tool_calls=2`, 6 calls in one round | At most 2 goroutines in-flight at any instant (verified by atomic counter peak); all 6 outputs returned in input order. |
| U25 | Parallel first-ask-wins | 3 parallel calls; 2nd hook returns `ErrAskUser` after a 100ms delay; others would take 300ms | Loop emits Final with the `ErrAskUser` message; surviving siblings observe `ctx.Err()` and return; their already-completed outputs are NOT appended to the transcript; `RunFinished{TerminatedBy: ask_user}`. |
| U26 | Parallel deterministic result order | Run U23 100 times with randomized tool sleep durations | The upstream receives outputs in the same input order every run, regardless of completion order. |
| U27 | Parallel capability gate | `ModelClient.Capabilities().SupportsParallelToolCalls=false` | Loop sets `parallel_tool_calls: false` on the request; mock model returns only one call per round; executor still runs correctly with effective parallelism=1. |

### 6.2 Integration tests

| ID | Subject | Setup | Expected |
|----|---------|-------|----------|
| I1 | Proxy invariance (byte-diff) | Real handler, `agent_mode` absent, stubbed upstream | SSE bytes identical to pre-change golden. |
| I2 | End-to-end agent run | Stub upstream returning a realistic three-round trace ending in `send_to_user` | Trace card populated; final text matches; memory persisted once. |
| I3 | Agent disabled | `agent_loop.enabled=false`, `agent_mode=true` request | HTTP 409 `agent_mode_disabled`. |
| I4 | Freetier rate limit | Freetier token, multiple MCP calls in one loop | Per-existing limiter throttles; loop continues with `Result{IsError: true}`. |
| I5 | Cancellation | Client disconnects mid-loop | Context cancellation propagates; structured log `agent_loop_cancelled`; no goroutine leak (verified by goroutine count delta). |
| I6 | Hook composition | Custom test hook added at `OnAfterToolCall` redacting a regex | Redaction applied; existing hooks still run; tool result reaches model in redacted form. |

### 6.3 Manual / UAT

| ID | Subject | Pass criteria |
|----|---------|---------------|
| M1 | Toggle visibility | "Agent" toggle present below input, default OFF, tooltip explains. |
| M2 | Persistence | Toggle remembers state across page refresh. |
| M3 | Live run | "Find the latest Anthropic Claude blog post and summarize" — trace card shows `web_search` → `web_fetch` → final; final renders as a normal assistant message. |
| M4 | Proxy untouched | Toggle off → behavior identical to current production. |
| M5 | Error rendering | Trigger U7 → UI shows a friendly error. |
| M6 | Memory check | Toggle on, complete a run, open a new session and ask a recall question — only final answer is recalled, not the tool trail. |

### 6.4 Performance smoke

- 10 concurrent agent loops against the stub upstream; `go test -race`
  clean; goroutine count returns to baseline within 5 s.
- `wall_clock_seconds` terminates a stuck loop within ±2 s.

---

## 7. Acceptance criteria

Merge gates — all must hold:

1. **Proxy invariance.** I1's SSE byte-diff test passes in CI.
2. **All unit tests** in §6.1 pass under `go test -race`.
3. **All integration tests** in §6.2 pass against the stub upstream in CI.
4. **Manual UAT** M1-M6 confirmed on the dev server.
5. **Structured termination logs.** Every loop emits one line with
   `agent_loop_terminated_by={send_to_user|implicit_final|ask_user|iteration_cap|timeout|circuit_breaker|error_budget|cancelled|error}`.
6. **Streaming protocol end-to-end.** The trace card in the UI renders
   real tool-call cards for at least one real prompt.
7. **`SubAgentTool` interface compiles, ships unregistered.** A unit test
   asserts that `spawn_agent` does NOT appear in `Registry.Names()` under
   the default config.
8. **HookBus ordering is deterministic.** Hooks fire in registration
   order; verified by U21.
9. **No new lint / vet warnings.**
10. **No secrets in logs.** API keys, tokens, MCP URLs containing keys
    are redacted.
11. **Backwards-compatible config.** Server boots with the existing
    settings file unmodified; agent loop defaults to disabled if the
    `agent_loop` block is absent.
12. **Memory hygiene.** U15 + M6 confirm only `(prompt, final_answer)`
    persist.
13. **Documentation merged** alongside the code; CLAUDE.md / AGENTS.md
    updated with a paragraph naming the `agentx/` package as the agent
    entrypoint.

---

## 8. Rollout plan

1. **Phase 0 — proposal review.** Merge this doc only. Sign-off on the
   tool-belt scope and write-gate default.
2. **Phase 1A — foundations.** Land `agentx/{session,tool,model,hook}` +
   the legacy `executeToolCall` wrapper. No handler changes yet. All
   packages compile and have unit tests, but nothing calls them in
   production.
3. **Phase 1B — loop & handler.** Land `agentx/loop`, `agentx/sse.go`
   (event → existing `emitThinkingDelta`/`emitTextDelta` mapping per
   §4.5), and `agentx/handler.go`; wire the conditional in
   `ChatHandler`. Feature flag `agent_loop.enabled=false` in production
   config. Backend now works end-to-end via the existing SSE channel; a
   hand-crafted `curl` with `agent_mode: true` already streams correctly
   to any current client.
4. **Phase 1C — frontend toggle.** Ship the three-file frontend change
   (§5.3): toggle button, ChatSwitch field, payload pass-through.
   Default OFF. SSE parser and message renderer untouched.
5. **Phase 1D — production enablement.** Flip
   `agent_loop.enabled=true`. Monitor `agent_loop_terminated_by`
   distribution and tool-error rate for a week.
6. **Phase 2A — sub-agent execution.** Implement `SubAgentTool.Execute`
   behind `subagent.enabled=true`. Add `spawn_agent` to the belt with
   profile-defined defaults.
7. **Phase 2B — typed `event: agent` SSE channel.** Add a second SSE
   channel emitting the typed `Event` JSON for rich trace UI. Add
   `agent-trace.tsx`, `onAgentEvent` callback, `agentTrace` field on
   messages. The Phase 1 `reasoning_content` channel continues to fire
   in parallel so older clients keep working — strictly additive, no
   protocol break.
8. **Phase 3 — branching / persistence.** Persist transcripts to JSONL;
   add `OpBranch` / `OpRewind`.

---

## 9. Risks & mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Prompt injection from `web_fetch` triggers destructive `file_write` | Medium | High | Untrusted delimiter; `write_gate=ask` default surfaces the proposed write to the human for explicit confirmation; refuse new-path writes whose target wasn't named by the user. |
| Cost blowup from runaway loops | Medium | Medium | Iteration + tool + wall-clock + error budgets; structured termination logs. |
| Model fails to call `send_to_user` and produces garbage final | Low | Low | Implicit-final fallback; trace surfaces the issue. |
| MCP discovery flaky at startup | Low | Medium | Cache the curated belt with TTL; on cache miss, fall back to a hard-coded minimal belt (`web_search`, `web_fetch`, `file_read`, `send_to_user`). |
| Agent loop bug regresses proxy path | Low | High | I1 SSE-byte-diff test; branching is a single conditional. |
| Foundation packages over-engineer for nothing | Medium | Low | Phase 1A's foundations cost ~600 LOC of interfaces + wrappers; the alternative is a rewrite at Phase 2. We accept the cost. |
| HookBus order-dependency surprises | Medium | Medium | Registration order is the firing order; documented; U21 enforces it. Hooks must be pure-ish (idempotent results expected). |

---

## 10. Open questions

1. **Trace persistence.** Should the trace live in chat history?
   **Proposal:** yes for the current session (`agentTrace` field on the
   message), no for long-term memory.
2. **HookBus error semantics.** Should a hook returning a non-nil error
   abort the run or just the current step?
   **Proposal:** abort current step (synthesize `IsError` result), not
   the run, **except** for the typed `ErrAskUser` sentinel (§3.7)
   which is the documented way to terminate the loop with a user-facing
   prompt. Avoids one buggy hook from killing a session.

---

## 11. References

### Implementation references

- OpenAI Codex CLI: <https://github.com/openai/codex>
  - Submit/event model: `codex-rs/core/src/codex/` (`Op`, `EventMsg`,
    `Session`, `ThreadManager`).
  - Compaction: `codex-rs/core/src/codex/compact.rs`.
  - Protocol schema: `codex-rs/app-server-protocol/schema/typescript/`.
  - Architecture write-up: <https://codex.danielvaughan.com/2026/03/28/codex-rs-rust-rewrite-architecture/>.
- pi-agent (`earendil-works/pi`): <https://github.com/earendil-works/pi>
  - Core: `packages/agent/` (`Agent` class, `streamFn`).
  - Streaming events: `pi-ai` `streamSimple` / typed `StreamEvent` union.
  - Extensions: `pi-coding-agent` `ExtensionAPI` (named hook points).
- `pi-subagents`: <https://github.com/nicobailon/pi-subagents>
  (subagent-as-tool, allowlist, depth guard, output modes).

### Design references

- Anthropic, *Building Effective Agents*:
  <https://www.anthropic.com/research/building-effective-agents>
- Anthropic, *Writing tools for agents*:
  <https://www.anthropic.com/engineering/writing-tools-for-agents>
- Anthropic, *Effective context engineering*:
  <https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents>
- Anthropic, *Mitigating prompt injection in browser use*:
  <https://www.anthropic.com/research/prompt-injection-defenses>
- OpenAI, *Running agents (Agents SDK)*:
  <https://openai.github.io/openai-agents-python/running_agents/>
- Letta, *Rearchitecting Letta's agent loop*:
  <https://www.letta.com/blog/letta-v1-agent>
- LangChain, *Context management for Deep Agents*:
  <https://www.langchain.com/blog/context-management-for-deepagents>
- Morph, *MCP Output Too Large*:
  <https://www.morphllm.com/mcp-output-too-large>
- Microsoft DevBlogs, *AG-UI multi-agent workflow demo*:
  <https://devblogs.microsoft.com/agent-framework/ag-ui-multi-agent-workflow-demo/>
- arXiv 2511.17006, *Budget-Aware Tool-Use*:
  <https://arxiv.org/html/2511.17006v1>
- Cordum, *AI Agent Circuit Breaker Pattern*:
  <https://cordum.io/blog/ai-agent-circuit-breaker-pattern>
