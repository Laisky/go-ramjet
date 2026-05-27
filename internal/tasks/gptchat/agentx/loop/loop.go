// Package loop drives the ReAct agent loop. The Run entrypoint consumes a
// session, dispatches hooks, streams model output, fans out tool calls via
// the bounded parallel executor, and emits the typed events the SSE writer
// later converts to wire chunks (proposal §4.1).
package loop

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	gerrors "github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// SendToUserToolName is the well-known tool the model calls to terminate the
// loop with a final answer. The loop short-circuits on this exact name and
// emits Final{Origin: "send_to_user"}.
const SendToUserToolName = "send_to_user"

// PromptRenderer returns the system-prompt text to inject into the per-round
// OnContext hook chain. May be nil — in that case the loop doesn't inject a
// system prompt itself (the canonical injection point is the hook bus; this
// is here only for test ergonomics in Phase 1B-1).
type PromptRenderer interface {
	// Render returns the prompt for the given round (0-indexed) and the
	// number of iterations the loop has remaining (including the current
	// round). The renderer may return an empty string to skip injection.
	Render(round int, capsRemaining int) string
}

// RunDeps bundles the dependencies Run needs. Splitting these into a struct
// keeps the call site readable and lets test harnesses build deps directly.
type RunDeps struct {
	// Bus is the hook bus; nil is tolerated (an empty bus is created).
	Bus *hook.Bus
	// Registry is the per-session tool registry (already subsetted to the
	// curated belt by the handler). Required.
	Registry tool.Registry
	// Model is the LLM client. Required.
	Model model.Client
	// Caps holds the loop budgets. Zero-valued fields default per §4.2.
	Caps Caps
	// Prompt renders the system prompt. May be nil — see PromptRenderer.
	Prompt PromptRenderer
	// UserPrompt is the OpUserTurn text. Propagated into SessionEndEvent
	// so memory hooks can persist (prompt, final) without re-reading the
	// transcript.
	UserPrompt string
	// SessionID is the session identifier shipped to session-start /
	// session-end hooks. Phase 1 handler typically uses the gin request id.
	SessionID string
	// Input is the pre-populated conversation history. The handler may
	// pass past user/assistant messages here; the loop appends the current
	// OpUserTurn (UserPrompt) and grows the slice round by round.
	Input []model.InputItem
	// ModelID overrides the model name passed to the upstream client. When
	// empty, the loop uses an empty Model field on model.Request and lets
	// the client default.
	ModelID string
	// MaxOutputTokens is forwarded into every model.Request. Zero means
	// "client default".
	MaxOutputTokens uint
	// Reasoning is forwarded into every model.Request. Nil means "off".
	Reasoning *model.Reasoning
	// Temperature, TopP forwarded into every model.Request.
	Temperature float64
	TopP        float64
	// Logger is used for termination / debug lines. Nil tolerated.
	Logger glog.Logger
}

// sendToUserArgs is the shape send_to_user expects. The schema check itself
// lives in the send_to_user tool (Phase 1B-2); the loop only peeks at
// final_answer when it short-circuits on the tool name.
type sendToUserArgs struct {
	FinalAnswer string             `json:"final_answer"`
	Citations   []session.Citation `json:"citations,omitempty"`
}

// Run drives the ReAct loop. It assumes the caller has already submitted
// the OpUserTurn into sess (so OpInterrupt can find the run context). The
// loop emits typed events through sess (which implements session.EventSink
// when downcast). The returned error is:
//
//   - nil for any clean termination (the corresponding RunFinished event
//     carries the structured TerminatedBy).
//   - ctx.Err() on cancellation; one Error + RunFinished{TerminatedBy:
//     "cancelled"} are emitted first.
//   - a wrapped error for unrecoverable failures (model.Client error not
//     catchable as ErrAskUser, etc); an Error + RunFinished{TerminatedBy:
//     "error"} pair is emitted.
func Run(ctx context.Context, sess session.Session, deps RunDeps) error {
	if sess == nil {
		return gerrors.New("loop.Run: nil session")
	}
	if deps.Model == nil {
		return gerrors.New("loop.Run: nil model client")
	}
	if deps.Registry == nil {
		return gerrors.New("loop.Run: nil registry")
	}

	if deps.Bus == nil {
		deps.Bus = hook.NewBus(deps.Logger)
	}
	deps.Caps = deps.Caps.withDefaults()

	sink, ok := sess.(session.EventSink)
	if !ok {
		return gerrors.New("loop.Run: session does not implement EventSink")
	}

	// Build the budget counter the parallel executor will record into; the
	// loop driver reads it after every round to enforce caps. The counter
	// is private to this Run invocation. We register a NewBudgetEnforcerHook
	// on the bus here so every tool result (including ones produced by
	// hook-synthesized IsError paths like circuit-breaker and write-gate
	// deny) lands in the same counter the loop reads for termination
	// decisions. Test code that wants its own counter can register an
	// additional NewBudgetEnforcerHook(extraCounter) — they don't conflict.
	budget := NewBudgetCounter()
	deps.Bus.OnAfterToolCall(NewBudgetEnforcerHook(budget))
	executor := NewParallelExecutor(deps.Bus, deps.Registry, sink, deps.Caps, budget)

	// Wall-clock deadline. We derive it once and check it after every model
	// call + parallel batch. The sub-context is propagated into the model
	// client and into ExecuteAll so deep work also notices the deadline.
	deadline := time.Now().Add(deps.Caps.WallClock)
	loopCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	toolNames := deps.Registry.Names()
	descriptors := buildDescriptors(deps.Registry)

	// 1) RunStarted.
	runStarted := session.RunStarted{
		BaseEvent:    session.NewBaseEvent(session.KindRunStarted, ""),
		RunID:        session.NewEventID(),
		ModelID:      deps.ModelID,
		ToolNames:    toolNames,
		IterationCap: deps.Caps.MaxIterations,
	}
	if err := sink.Emit(runStarted); err != nil {
		return gerrors.Wrap(err, "emit RunStarted")
	}

	// 2) OnSessionStart.
	if _, err := deps.Bus.DispatchSessionStart(loopCtx, hook.SessionStartEvent{
		SessionID: deps.SessionID,
		Caps:      deps.Caps.toHookCaps(),
	}); err != nil {
		// session-start failures don't terminate the run (we don't want
		// a buggy memory injection to poison the whole turn) but we do
		// log them; this matches §10's open-question resolution.
		logDebug(deps.Logger, "session_start hook error", zap.Error(err))
	}

	// 3) Seed inputItems. The caller's Input is the prior conversation; we
	// append the current OpUserTurn as a user message so the model sees it
	// regardless of what the caller put in Input.
	inputItems := append([]model.InputItem{}, deps.Input...)
	if deps.UserPrompt != "" {
		inputItems = append(inputItems, userMessage(deps.UserPrompt))
	}

	caps := deps.Model.Capabilities()
	parallelToolCalls := caps.SupportsParallelToolCalls && deps.Caps.MaxParallelToolCalls > 1

	var (
		finalText      string
		finalCitations []session.Citation
		finalOrigin    string
		terminatedBy   string
		runErr         error
		iterationsDone int
		finalStepID    string
	)

	// Helper to emit a Final + RunFinished pair under a given step.
	emitFinal := func(stepID string, text string, citations []session.Citation, origin, termBy string) error {
		finalEvent := session.Final{
			BaseEvent: session.NewBaseEvent(session.KindFinal, stepID),
			FinalText: text,
			Citations: citations,
			Origin:    origin,
		}
		if err := sink.Emit(finalEvent); err != nil {
			return gerrors.Wrap(err, "emit Final")
		}
		runFinished := session.RunFinished{
			BaseEvent:    session.NewBaseEvent(session.KindRunFinished, runStarted.EventID()),
			RunID:        runStarted.RunID,
			TerminatedBy: termBy,
			TotalUsage: session.TotalUsage{
				ToolCalls:  int(budget.ToolCalls()),
				Iterations: iterationsDone,
			},
		}
		return sink.Emit(runFinished)
	}

	// emitError emits Error + RunFinished{TerminatedBy: termBy} so the
	// transcript carries a structured failure for any non-Final exit.
	emitErrorAndRunFinished := func(code, msg, termBy string) error {
		errEvent := session.Error{
			BaseEvent: session.NewBaseEvent(session.KindError, runStarted.EventID()),
			Code:      code,
			Message:   msg,
		}
		if err := sink.Emit(errEvent); err != nil {
			return gerrors.Wrap(err, "emit Error")
		}
		runFinished := session.RunFinished{
			BaseEvent:    session.NewBaseEvent(session.KindRunFinished, runStarted.EventID()),
			RunID:        runStarted.RunID,
			TerminatedBy: termBy,
			TotalUsage: session.TotalUsage{
				ToolCalls:  int(budget.ToolCalls()),
				Iterations: iterationsDone,
			},
		}
		return sink.Emit(runFinished)
	}

	// Always emit OnSessionEnd at the end, regardless of how we leave the
	// loop. Errors from the session-end chain are logged but not returned.
	defer func() {
		_, hookErr := deps.Bus.DispatchSessionEnd(context.Background(), hook.SessionEndEvent{
			SessionID:    deps.SessionID,
			TerminatedBy: terminatedBy,
			FinalText:    finalText,
			UserPrompt:   deps.UserPrompt,
		})
		if hookErr != nil {
			logDebug(deps.Logger, "session_end hook error", zap.Error(hookErr))
		}
		logTermination(deps.Logger, terminatedBy, finalOrigin)
	}()

	// 4) Iteration loop.
	for round := 0; round < deps.Caps.MaxIterations; round++ {
		iterationsDone = round + 1
		stepID := session.NewEventID()
		finalStepID = stepID

		// 4.l: synthetic "summarize now" message at the last allowed round.
		// We inject it into inputItems before OnContext so hooks can still
		// see and transform it. Note: round is 0-based and the loop runs at
		// most MaxIterations times, so the last actually-executed round is
		// MaxIterations-1.
		if round == deps.Caps.MaxIterations-1 {
			inputItems = append(inputItems,
				systemMessage(lastRoundSummarizeHint()))
		}

		// 4.a: per-step deadline check.
		if loopCtx.Err() != nil {
			runErr = loopCtx.Err()
			terminatedBy = session.TerminatedByCancelled
			if errors.Is(runErr, context.DeadlineExceeded) {
				terminatedBy = session.TerminatedByTimeout
				_ = emitErrorAndRunFinished("timeout", "wall-clock budget exhausted", session.TerminatedByTimeout)
				return nil
			}
			_ = emitErrorAndRunFinished("cancelled", runErr.Error(), session.TerminatedByCancelled)
			return runErr
		}

		// StepStarted.
		stepEvent := session.StepStarted{
			BaseEvent:      session.BaseEvent{ID: stepID, ParentID: runStarted.EventID(), EventKind: session.KindStepStarted, At: time.Now()},
			StepID:         stepID,
			IterationIndex: round,
		}
		if err := sink.Emit(stepEvent); err != nil {
			return gerrors.Wrap(err, "emit StepStarted")
		}

		// 4.b: OnContext.
		ctxEvent := hook.ContextEvent{Input: append([]model.InputItem{}, inputItems...)}
		ctxEvent, ctxErr := deps.Bus.DispatchContext(loopCtx, ctxEvent)
		if ctxErr != nil {
			var ask *hook.ErrAskUser
			if errors.As(ctxErr, &ask) {
				// OnContext returning ErrAskUser exits the loop with
				// the ask-user prompt as the Final.
				finalText = ask.Message
				finalOrigin = session.FinalOriginAskUser
				terminatedBy = session.TerminatedByAskUser
				if err := emitFinal(stepID, ask.Message, nil, session.FinalOriginAskUser, session.TerminatedByAskUser); err != nil {
					return err
				}
				return nil
			}
			// Generic OnContext error -> abort with structured error.
			runErr = ctxErr
			terminatedBy = session.TerminatedByError
			_ = emitErrorAndRunFinished("context_hook_error", ctxErr.Error(), session.TerminatedByError)
			return gerrors.Wrap(ctxErr, "context hook")
		}
		modelInput := ctxEvent.Input

		// 4.c: build Request.
		req := model.Request{
			Model:             deps.ModelID,
			Input:             modelInput,
			Tools:             descriptors,
			ToolChoice:        "auto",
			MaxOutputTokens:   deps.MaxOutputTokens,
			Reasoning:         deps.Reasoning,
			Stream:            true,
			Temperature:       deps.Temperature,
			TopP:              deps.TopP,
			ParallelToolCalls: parallelToolCalls,
		}

		// 4.d: stream the model.
		streamCh, err := deps.Model.Stream(loopCtx, req)
		if err != nil {
			// Distinguish ctx cancellation from genuine upstream errors.
			if loopCtx.Err() != nil {
				if errors.Is(loopCtx.Err(), context.DeadlineExceeded) {
					terminatedBy = session.TerminatedByTimeout
					_ = emitErrorAndRunFinished("timeout", "wall-clock budget exhausted", session.TerminatedByTimeout)
					return nil
				}
				terminatedBy = session.TerminatedByCancelled
				_ = emitErrorAndRunFinished("cancelled", loopCtx.Err().Error(), session.TerminatedByCancelled)
				return loopCtx.Err()
			}
			runErr = err
			terminatedBy = session.TerminatedByError
			_ = emitErrorAndRunFinished("model_stream_error", err.Error(), session.TerminatedByError)
			return gerrors.Wrap(err, "model.Stream")
		}

		var (
			roundCalls     []model.FunctionCall
			roundText      strings.Builder
			roundStreamErr error
			roundUsage     *model.Usage
		)

		// Consume chunks.
		for chunk := range streamCh {
			switch chunk.Kind {
			case model.ChunkText:
				roundText.WriteString(chunk.Text)
				_ = sink.Emit(session.AssistantTextDelta{
					BaseEvent: session.NewBaseEvent(session.KindAssistantTextDelta, stepID),
					StepID:    stepID,
					Delta:     chunk.Text,
				})
			case model.ChunkReasoning:
				_ = sink.Emit(session.AssistantReasoningDelta{
					BaseEvent: session.NewBaseEvent(session.KindAssistantReasoningDelta, stepID),
					StepID:    stepID,
					Delta:     chunk.Text,
				})
			case model.ChunkFunction:
				if chunk.FunctionCall != nil {
					roundCalls = append(roundCalls, *chunk.FunctionCall)
				}
			case model.ChunkUsage:
				if chunk.Usage != nil {
					u := *chunk.Usage
					roundUsage = &u
				}
			case model.ChunkDone:
				// channel will close after this; just continue draining.
			case model.ChunkError:
				if chunk.Err != nil {
					roundStreamErr = chunk.Err
				} else if chunk.Text != "" {
					roundStreamErr = gerrors.New(chunk.Text)
				}
			}
		}

		if roundStreamErr != nil {
			// Treat as fatal: we have no way to recover an incomplete
			// stream into a coherent round.
			runErr = roundStreamErr
			terminatedBy = session.TerminatedByError
			_ = emitErrorAndRunFinished("model_stream_error", roundStreamErr.Error(), session.TerminatedByError)
			return gerrors.Wrap(roundStreamErr, "model stream chunk error")
		}

		// emitStepFinished must fire as the LAST event of the step per
		// the U17 golden order (proposal §6.1):
		//   StepStarted -> AssistantTextDelta* -> ToolCallStart -> ToolCallEnd -> ToolResult -> StepFinished
		// We build the StepFinished payload now (with usage data) and
		// emit it at every exit point below.
		stepFinished := session.StepFinished{
			BaseEvent: session.NewBaseEvent(session.KindStepFinished, stepID),
			StepID:    stepID,
		}
		if roundUsage != nil {
			stepFinished.TokensIn = roundUsage.InputTokens
			stepFinished.TokensOut = roundUsage.OutputTokens
		}

		// 4.f: implicit final. No tool calls -> no executor; emit
		// StepFinished now, then Final.
		if len(roundCalls) == 0 {
			_ = sink.Emit(stepFinished)
			finalText = roundText.String()
			finalOrigin = session.FinalOriginImplicit
			terminatedBy = session.TerminatedByImplicitFinal
			if err := emitFinal(stepID, finalText, nil, session.FinalOriginImplicit, session.TerminatedByImplicitFinal); err != nil {
				return err
			}
			return nil
		}

		// 4.g: send_to_user wins if present. send_to_user is the exit
		// signal: it does NOT go through the executor (no
		// tool_call_start / tool_result events for the exit tool — the
		// Final event IS the exit signal). Emit StepFinished, then Final.
		if sendIdx := indexOfSendToUser(roundCalls); sendIdx >= 0 {
			args, parseErr := parseSendToUser(roundCalls[sendIdx].Arguments)
			if parseErr != nil {
				// Malformed send_to_user is treated as a tool error per
				// U9 — fold it into the input transcript and let the
				// model retry next round. To keep the loop semantics
				// uniform we still synthesize a FunctionCallOutput here
				// rather than going through the executor.
				inputItems = appendFunctionCallAndOutput(inputItems,
					roundCalls[sendIdx],
					model.FunctionCallOutput{
						CallID: roundCalls[sendIdx].CallID,
						Output: fmt.Sprintf("send_to_user error: %v", parseErr),
					},
				)
				budget.RecordToolCall()
				budget.RecordError()
				_ = sink.Emit(stepFinished)
				// Discard sibling calls in the same round per the
				// proposal — send_to_user is supposed to be terminal.
				if budget.Errors() > int64(deps.Caps.ErrorBudget) {
					terminatedBy = session.TerminatedByErrorBudget
					_ = emitErrorAndRunFinished("error_budget", "tool error budget exhausted", session.TerminatedByErrorBudget)
					return nil
				}
				continue
			}
			_ = sink.Emit(stepFinished)
			finalText = args.FinalAnswer
			finalCitations = args.Citations
			finalOrigin = session.FinalOriginSendToUser
			terminatedBy = session.TerminatedBySendToUser
			if err := emitFinal(stepID, finalText, finalCitations, session.FinalOriginSendToUser, session.TerminatedBySendToUser); err != nil {
				return err
			}
			return nil
		}

		// 4.h: dispatch parallel batch.
		executor.SetStepParent(stepID)
		outputs, execErr := executor.ExecuteAll(loopCtx, roundCalls)
		if execErr != nil {
			var ask *hook.ErrAskUser
			if errors.As(execErr, &ask) {
				finalText = ask.Message
				finalOrigin = session.FinalOriginAskUser
				terminatedBy = session.TerminatedByAskUser
				if err := emitFinal(stepID, ask.Message, nil, session.FinalOriginAskUser, session.TerminatedByAskUser); err != nil {
					return err
				}
				return nil
			}
			if errors.Is(execErr, context.DeadlineExceeded) {
				terminatedBy = session.TerminatedByTimeout
				_ = emitErrorAndRunFinished("timeout", "wall-clock budget exhausted", session.TerminatedByTimeout)
				return nil
			}
			if errors.Is(execErr, context.Canceled) {
				terminatedBy = session.TerminatedByCancelled
				_ = emitErrorAndRunFinished("cancelled", execErr.Error(), session.TerminatedByCancelled)
				return execErr
			}
			runErr = execErr
			terminatedBy = session.TerminatedByError
			_ = emitErrorAndRunFinished("parallel_executor_error", execErr.Error(), session.TerminatedByError)
			return gerrors.Wrap(execErr, "executor.ExecuteAll")
		}

		// Step is done — emit StepFinished now so the U17 ordering holds:
		// tool_call_* events from the executor have already drained.
		_ = sink.Emit(stepFinished)

		// 4.i: feed results back into the input for the next round.
		// Preserve the model's free assistant text (the ReAct "Thought") so
		// it carries into the next round as plan continuity. Standalone
		// function_call items are a separate transcript shape per the
		// Responses API; text rides in its own assistant-role message,
		// emitted BEFORE the function_call/function_call_output pairs that
		// it preceded on the wire.
		if text := roundText.String(); text != "" {
			inputItems = append(inputItems, assistantMessage(text))
		}
		for i, call := range roundCalls {
			inputItems = appendFunctionCallAndOutput(inputItems, call, outputs[i])
		}

		// 4.j: enforce budgets.
		if budget.Errors() > int64(deps.Caps.ErrorBudget) {
			terminatedBy = session.TerminatedByErrorBudget
			_ = emitErrorAndRunFinished("error_budget", "tool error budget exhausted", session.TerminatedByErrorBudget)
			return nil
		}
		if budget.ToolCalls() > int64(deps.Caps.MaxToolCalls) {
			terminatedBy = session.TerminatedByErrorBudget
			// Tool budget shares the error-budget termination bucket per
			// the proposal §7 — there's no separate enum value for
			// "tool_budget" in agentx/session events. We log the
			// distinction so observability can split the two.
			logDebug(deps.Logger, "tool_budget exhausted",
				zap.Int64("tool_calls", budget.ToolCalls()),
				zap.Int("max", deps.Caps.MaxToolCalls),
			)
			_ = emitErrorAndRunFinished("tool_budget", "tool call budget exhausted", session.TerminatedByErrorBudget)
			return nil
		}

		// 4.k: wall-clock check (in addition to ctx deadline propagation
		// — we want a deterministic termination event even if the next
		// model.Stream wouldn't itself error out yet).
		if time.Now().After(deadline) {
			terminatedBy = session.TerminatedByTimeout
			_ = emitErrorAndRunFinished("timeout", "wall-clock budget exhausted", session.TerminatedByTimeout)
			return nil
		}
	}

	// 5) Iteration cap. Use finalStepID (the last step's id) as the parent
	// of the iteration-cap Error event so the trace remains structured.
	terminatedBy = session.TerminatedByIterationCap
	if finalStepID == "" {
		finalStepID = runStarted.EventID()
	}
	errEvent := session.Error{
		BaseEvent: session.NewBaseEvent(session.KindError, finalStepID),
		Code:      "iteration_cap",
		Message:   fmt.Sprintf("loop reached iteration cap %d without send_to_user", deps.Caps.MaxIterations),
	}
	if err := sink.Emit(errEvent); err != nil {
		return gerrors.Wrap(err, "emit iteration-cap Error")
	}
	runFinished := session.RunFinished{
		BaseEvent:    session.NewBaseEvent(session.KindRunFinished, runStarted.EventID()),
		RunID:        runStarted.RunID,
		TerminatedBy: session.TerminatedByIterationCap,
		TotalUsage: session.TotalUsage{
			ToolCalls:  int(budget.ToolCalls()),
			Iterations: iterationsDone,
		},
	}
	if err := sink.Emit(runFinished); err != nil {
		return gerrors.Wrap(err, "emit RunFinished")
	}
	return runErr
}

// indexOfSendToUser returns the index of the first send_to_user call in
// calls, or -1 if absent.
func indexOfSendToUser(calls []model.FunctionCall) int {
	for i, c := range calls {
		if c.Name == SendToUserToolName {
			return i
		}
	}
	return -1
}

// parseSendToUser unmarshals the args payload, returning a descriptive
// error if the shape is wrong.
func parseSendToUser(raw stdjson.RawMessage) (sendToUserArgs, error) {
	var args sendToUserArgs
	if len(raw) == 0 {
		return args, gerrors.New("send_to_user args are empty")
	}
	if err := stdjson.Unmarshal(raw, &args); err != nil {
		return args, gerrors.Wrap(err, "unmarshal send_to_user args")
	}
	if strings.TrimSpace(args.FinalAnswer) == "" {
		return args, gerrors.New("send_to_user missing final_answer")
	}
	return args, nil
}

// userMessage builds a Responses-API style input message. We use map[string]any
// (rather than depending on the concrete http types) so the loop has no
// runtime coupling to the http package shapes — the model.OneAPI adapter
// already accepts these map shapes via its InputItem boundary parsing.
func userMessage(text string) model.InputItem {
	return map[string]any{
		"role":    "user",
		"content": text,
	}
}

// systemMessage builds a system-role input item used for the §6.1 U3
// "summarize now" hint and for prompt injection.
func systemMessage(text string) model.InputItem {
	return map[string]any{
		"role":    "system",
		"content": text,
	}
}

// assistantMessage carries the model's free text (the ReAct "Thought") from
// one round into the next so the model sees its own plan continuity rather
// than re-deriving it. Phase 1 uses the same map shape as user/system
// helpers; the coercing model client converts it at the boundary.
func assistantMessage(text string) model.InputItem {
	return map[string]any{
		"role":    "assistant",
		"content": text,
	}
}

// appendFunctionCallAndOutput appends the model's function_call item plus its
// matching function_call_output item to the input transcript. Phase 1 builds
// the items as plain maps so the loop stays decoupled from the upstream
// adapter's concrete shapes — the OneAPI adapter recognises both the
// httppkg.* concrete types and these map-shaped equivalents.
//
// The function_call item carries BOTH an `id` and a `call_id`. The
// Responses API strict-validates `input[*].id` to be a non-empty string
// of `[A-Za-z0-9_\-]+`; an empty id (the prior shape) earned us a 400
// `invalid_value` from the upstream. The two fields can share the same
// value (the upstream emits them as such on output_item.added — see the
// OneAPI adapter's pendingCall accumulator). When the upstream omitted
// CallID (defensive: never observed in production) we synthesize an id
// in the `fc_<ULID>` form so the regex still matches.
func appendFunctionCallAndOutput(
	items []model.InputItem,
	call model.FunctionCall,
	out model.FunctionCallOutput,
) []model.InputItem {
	id := callIDForFunctionCall(call)
	items = append(items,
		map[string]any{
			"type":      "function_call",
			"id":        id,
			"call_id":   call.CallID,
			"name":      call.Name,
			"arguments": string(call.Arguments),
		},
		map[string]any{
			"type":    "function_call_output",
			"call_id": out.CallID,
			"output":  out.Output,
		},
	)
	return items
}

// callIDForFunctionCall returns the `id` value to stamp on a
// function_call input item. The OpenAI Responses API requires this
// field to (a) be non-empty and (b) begin with the `fc` prefix —
// distinct from `call_id` (which uses the `call_` prefix). The two
// fields live in DIFFERENT namespaces even though some intermediate
// SSE events stream them side-by-side. Reusing `call.CallID` verbatim
// triggers a 400 `Expected an ID that begins with 'fc'.` from the
// upstream, so we always synthesize `fc_<ULID>` for the id slot. The
// `call_id` field on the function_call (and on its matching
// function_call_output) is what binds the pair together for tool-call
// dispatch; that field continues to carry `call.CallID` verbatim.
func callIDForFunctionCall(call model.FunctionCall) string {
	if id := strings.TrimSpace(call.CallID); id != "" && strings.HasPrefix(id, "fc") {
		return id
	}
	return "fc_" + session.NewEventID()
}

// buildDescriptors converts the per-session registry into the model-facing
// descriptor slice. We materialise it once per loop because the registry
// shape is stable for the lifetime of a session.
func buildDescriptors(reg tool.Registry) []model.ToolDescriptor {
	regDesc := reg.Descriptors()
	out := make([]model.ToolDescriptor, 0, len(regDesc))
	for _, d := range regDesc {
		out = append(out, model.ToolDescriptor{
			Name:        d.Name,
			Description: d.Description,
			Schema:      d.Schema,
		})
	}
	return out
}

// lastRoundSummarizeHint is the synthetic note injected on the last allowed
// round per §6.1 U3.
func lastRoundSummarizeHint() string {
	return "You have 1 step remaining. Summarize what you have so far and call send_to_user with your best final answer."
}

func logDebug(logger glog.Logger, msg string, fields ...zap.Field) {
	if logger == nil {
		return
	}
	logger.Debug(msg, fields...)
}

func logTermination(logger glog.Logger, terminatedBy, finalOrigin string) {
	if logger == nil || terminatedBy == "" {
		return
	}
	logger.Info("agent_loop_terminated_by",
		zap.String("terminated_by", terminatedBy),
		zap.String("final_origin", finalOrigin),
	)
}
