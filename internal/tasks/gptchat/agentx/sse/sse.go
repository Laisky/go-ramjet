package sse

import (
	"context"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
)

// EmitKind tags which SSE delta channel an emission lands on.
//
// The Phase 1 wire protocol uses only three sub-channels of the existing
// chat-completion stream — see proposal §4.5.1 for the mapping table.
type EmitKind int

const (
	// EmitReasoning routes the emitted text to choices[0].delta.reasoning_content
	// so the frontend renders it inside the reasoning panel.
	EmitReasoning EmitKind = iota
	// EmitContent routes the emitted text to choices[0].delta.content so it
	// lands in the user-visible assistant message body.
	EmitContent
	// EmitFinish writes the terminal chunk with finish_reason="stop". The
	// text argument is ignored by handlers; pass "".
	EmitFinish
)

// String returns a stable label for the EmitKind, used in tests and logs.
func (k EmitKind) String() string {
	switch k {
	case EmitReasoning:
		return "reasoning"
	case EmitContent:
		return "content"
	case EmitFinish:
		return "finish"
	default:
		return "unknown"
	}
}

// EmitFunc encodes a (kind, requestID, text) tuple into one SSE
// `data: {…}\n\n` chunk and writes it to the underlying transport.
// Returning an error causes Consume to abort and surface that error.
//
// The handler in 1B-4 supplies a wrapper around the existing
// emitThinkingDelta / emitTextDelta / writeChatCompletionChunk helpers;
// tests supply an in-memory recorder.
type EmitFunc func(kind EmitKind, requestID, text string) error

// finalChunkBytes is the byte window used to fan Final.Text out into
// EmitContent calls. The value mirrors the streaming chunk size used
// elsewhere in the gptchat backend so the UI sees a familiar paint
// cadence; it is intentionally small so the user sees text appear
// incrementally rather than as a single bulk delivery.
const finalChunkBytes = 200

// Writer translates session.Event values into SSE wire chunks. A Writer
// is bound to a single agent run via NewWriter; the EmitFunc owns the
// underlying transport, so the Writer holds no goroutine of its own and
// requires no shutdown apart from the caller closing the events channel
// or cancelling the context passed to Consume.
type Writer struct {
	emit      EmitFunc
	requestID string
}

// NewWriter constructs a Writer bound to the given emit function and
// request id. The caller is responsible for the request id; the loop
// driver passes the upstream's request id when available so the chunks
// the agent emits share the same x-oneapi-request-id with proxy chunks.
func NewWriter(emit EmitFunc, requestID string) *Writer {
	return &Writer{emit: emit, requestID: requestID}
}

// Consume drains events from the channel until it is closed or the
// context cancels. Each event is mapped per the §4.5 table and emitted
// via the EmitFunc. The function returns nil on clean drain, ctx.Err()
// on cancellation, or the first EmitFunc error.
//
// Consume is intended to run in its own goroutine, started by the
// handler after it constructs the Session and before it calls loop.Run.
// The loop emits events into the session's Events() channel; Consume
// reads them and writes SSE.
func (w *Writer) Consume(ctx context.Context, events <-chan session.Event) error {
	if w == nil || w.emit == nil {
		return errors.New("sse: nil writer or emit function")
	}
	for {
		// Cancellation supersedes draining — even if events are pending we
		// return ctx.Err() immediately on cancellation per the contract.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if err := w.ConsumeOne(ev); err != nil {
				return err
			}
		}
	}
}

// ConsumeOne maps a single event to zero or more EmitFunc calls. It is
// exposed primarily for unit testing; the production driver uses
// Consume which owns the channel-drain loop.
func (w *Writer) ConsumeOne(ev session.Event) error {
	if w == nil || w.emit == nil {
		return errors.New("sse: nil writer or emit function")
	}
	if ev == nil {
		return nil
	}
	switch e := ev.(type) {
	case session.RunStarted:
		return w.emitReasoningLine(
			toolStepMarker + "agent run started (model=" + e.ModelID +
				", iter_cap=" + strconv.Itoa(e.IterationCap) + ")\n",
		)
	case session.StepStarted:
		return w.emitReasoningLine(
			toolStepMarker + "-- step " + strconv.Itoa(e.IterationIndex) + " --\n",
		)
	case session.AssistantReasoningDelta:
		if e.Delta == "" {
			return nil
		}
		return w.emit(EmitReasoning, w.requestID, e.Delta)
	case session.AssistantTextDelta:
		// Per §4.5 the model's interim "thinking out loud" text is routed
		// to the reasoning channel, not the content channel — only the
		// final send_to_user payload may reach delta.content.
		if e.Delta == "" {
			return nil
		}
		return w.emit(EmitReasoning, w.requestID, e.Delta)
	case session.ToolCallStart:
		prefix := toolStepMarker + "[" + short(e.CallID) + "] "
		if err := w.emitReasoningLine(prefix + "tool_call: " + e.ToolName + "\n"); err != nil {
			return err
		}
		// Suppress the args line entirely when arguments are empty —
		// the proxy loop does the same so the trace stays terse for
		// no-arg tools.
		if strings.TrimSpace(e.ArgsPreview) != "" {
			if err := w.emitReasoningLine(prefix + "args: " + e.ArgsPreview + "\n"); err != nil {
				return err
			}
		}
		return nil
	case session.ToolCallEnd:
		// Folded into ToolResult — no emit.
		return nil
	case session.ToolResult:
		prefix := toolStepMarker + "[" + short(e.CallID) + "] "
		if e.IsError {
			return w.emitReasoningLine(
				prefix + "tool error: " + escapeUntrustedDelimiter(e.ContentPreview) + "\n",
			)
		}
		return w.emitReasoningLine(
			prefix + "tool ok (" + strconv.Itoa(e.BytesTotal) + "B)\n",
		)
	case session.StepFinished:
		// Bookkeeping only — no emit.
		return nil
	case session.Final:
		if e.FinalText == "" {
			return nil
		}
		for _, chunk := range chunkString(e.FinalText, finalChunkBytes) {
			if err := w.emit(EmitContent, w.requestID, chunk); err != nil {
				return err
			}
		}
		return nil
	case session.RunFinished:
		if err := w.emitReasoningLine(
			toolStepMarker + "run finished (terminated_by=" + e.TerminatedBy + ")\n",
		); err != nil {
			return err
		}
		return w.emit(EmitFinish, w.requestID, "")
	case session.Error:
		return w.emitReasoningLine(
			toolStepMarker + "error: " + e.Code + " — " + e.Message + "\n",
		)
	default:
		// Unknown event kinds are silently ignored — Phase 2 may
		// introduce new types and we want backward-compatible degrade.
		return nil
	}
}

// emitReasoningLine is a tiny wrapper that funnels text through
// EmitReasoning while applying the untrusted-delimiter escape. The
// escape is idempotent on the trace markers we generate ourselves, so
// applying it uniformly here is safe and cheaper to reason about than
// scattering escape calls across each branch.
func (w *Writer) emitReasoningLine(text string) error {
	return w.emit(EmitReasoning, w.requestID, escapeUntrustedDelimiter(text))
}
