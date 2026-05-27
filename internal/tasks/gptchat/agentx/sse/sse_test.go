package sse

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
)

// updateGolden lets `go test -update` regenerate the U17 golden file
// from the recorded run.
var updateGolden = flag.Bool("update", false, "regenerate golden files in testdata/")

// recordedEmit is a single (kind, requestID, text) tuple captured by
// the test recorder.
type recordedEmit struct {
	Kind      EmitKind
	RequestID string
	Text      string
}

// recorder is the in-memory EmitFunc used by tests. It is goroutine-safe
// only when Consume runs in a single goroutine, which is the contract.
type recorder struct {
	calls []recordedEmit
	// errOn, when non-zero, is the (1-based) call index that returns an
	// error instead of recording. Used by error-propagation tests.
	errOn int
}

func (r *recorder) Emit(kind EmitKind, requestID, text string) error {
	r.calls = append(r.calls, recordedEmit{Kind: kind, RequestID: requestID, Text: text})
	if r.errOn != 0 && len(r.calls) == r.errOn {
		return errors.New("test: forced emit failure")
	}
	return nil
}

// newWriter constructs a Writer wired to a fresh recorder, returning
// both for convenience.
func newWriter(reqID string) (*Writer, *recorder) {
	r := &recorder{}
	return NewWriter(r.Emit, reqID), r
}

// makeBase fills in a deterministic BaseEvent header — the sse package
// does not rely on event IDs for ordering, so a fixed stamp keeps tests
// readable without affecting the assertions.
func makeBase(kind string) session.BaseEvent {
	return session.BaseEvent{
		ID:        kind + "-id",
		EventKind: kind,
		At:        time.Unix(0, 0),
	}
}

// -----------------------------------------------------------------------------
// ConsumeOne — per-event mapping unit tests
// -----------------------------------------------------------------------------

func TestConsumeOne_RunStarted_EmitsHeaderLine(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-1")
	err := w.ConsumeOne(session.RunStarted{
		BaseEvent:    makeBase(session.KindRunStarted),
		RunID:        "run-1",
		ModelID:      "gpt-5",
		IterationCap: 20,
	})
	require.NoError(t, err)
	require.Equal(t, []recordedEmit{{
		Kind:      EmitReasoning,
		RequestID: "rid-1",
		Text:      "[[TOOLS]] agent run started (model=gpt-5, iter_cap=20)\n",
	}}, r.calls)
}

func TestConsumeOne_StepStarted_EmitsStepLine(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-2")
	err := w.ConsumeOne(session.StepStarted{
		BaseEvent:      makeBase(session.KindStepStarted),
		StepID:         "step-3",
		IterationIndex: 3,
	})
	require.NoError(t, err)
	require.Equal(t, []recordedEmit{{
		Kind:      EmitReasoning,
		RequestID: "rid-2",
		Text:      "[[TOOLS]] -- step 3 --\n",
	}}, r.calls)
}

func TestConsumeOne_AssistantReasoningDelta_PassesThrough(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-3")
	err := w.ConsumeOne(session.AssistantReasoningDelta{
		BaseEvent: makeBase(session.KindAssistantReasoningDelta),
		Delta:     "model thinking…",
	})
	require.NoError(t, err)
	require.Equal(t, []recordedEmit{{
		Kind:      EmitReasoning,
		RequestID: "rid-3",
		Text:      "model thinking…",
	}}, r.calls)
}

func TestConsumeOne_AssistantTextDelta_RoutesToReasoning(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-4")
	err := w.ConsumeOne(session.AssistantTextDelta{
		BaseEvent: makeBase(session.KindAssistantTextDelta),
		Delta:     "interim out-loud prose",
	})
	require.NoError(t, err)
	require.Len(t, r.calls, 1)
	require.Equal(t, EmitReasoning, r.calls[0].Kind,
		"AssistantTextDelta must route to reasoning, not content")
	require.Equal(t, "interim out-loud prose", r.calls[0].Text)
}

func TestConsumeOne_ToolCallStart_WithArgs(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-5")
	err := w.ConsumeOne(session.ToolCallStart{
		BaseEvent:   makeBase(session.KindToolCallStart),
		CallID:      "01HTJZ1F5JEDQRD2MNGNH9V0WB",
		ToolName:    "web_search",
		ArgsPreview: `{"q":"x"}`,
	})
	require.NoError(t, err)
	require.Equal(t, []recordedEmit{
		{Kind: EmitReasoning, RequestID: "rid-5", Text: "[[TOOLS]] [01HTJZ] tool_call: web_search\n"},
		{Kind: EmitReasoning, RequestID: "rid-5", Text: "[[TOOLS]] [01HTJZ] args: {\"q\":\"x\"}\n"},
	}, r.calls)
}

func TestConsumeOne_ToolCallStart_EmptyArgsOmitsArgsLine(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-6")
	err := w.ConsumeOne(session.ToolCallStart{
		BaseEvent:   makeBase(session.KindToolCallStart),
		CallID:      "01HTJZ1F5JEDQRD2MNGNH9V0WB",
		ToolName:    "send_to_user",
		ArgsPreview: "",
	})
	require.NoError(t, err)
	require.Equal(t, []recordedEmit{{
		Kind:      EmitReasoning,
		RequestID: "rid-6",
		Text:      "[[TOOLS]] [01HTJZ] tool_call: send_to_user\n",
	}}, r.calls)
}

func TestConsumeOne_ToolCallStart_WhitespaceArgsOmitsArgsLine(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-6b")
	err := w.ConsumeOne(session.ToolCallStart{
		BaseEvent:   makeBase(session.KindToolCallStart),
		CallID:      "01HTJZ1F5JEDQRD2MNGNH9V0WB",
		ToolName:    "ping",
		ArgsPreview: "   \n  ",
	})
	require.NoError(t, err)
	require.Len(t, r.calls, 1)
	require.NotContains(t, r.calls[0].Text, "args:")
}

func TestConsumeOne_ToolCallEnd_NoEmit(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-7")
	err := w.ConsumeOne(session.ToolCallEnd{
		BaseEvent:  makeBase(session.KindToolCallEnd),
		CallID:     "01HTJZ1F5JEDQRD2MNGNH9V0WB",
		DurationMS: 250,
	})
	require.NoError(t, err)
	require.Empty(t, r.calls)
}

func TestConsumeOne_ToolResult_OkEmitsByteCount(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-8")
	err := w.ConsumeOne(session.ToolResult{
		BaseEvent:  makeBase(session.KindToolResult),
		CallID:     "01HTJZ1F5JEDQRD2MNGNH9V0WB",
		BytesTotal: 12345,
		IsError:    false,
	})
	require.NoError(t, err)
	require.Equal(t, []recordedEmit{{
		Kind:      EmitReasoning,
		RequestID: "rid-8",
		Text:      "[[TOOLS]] [01HTJZ] tool ok (12345B)\n",
	}}, r.calls)
}

func TestConsumeOne_ToolResult_ErrorEmitsMessage(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-9")
	err := w.ConsumeOne(session.ToolResult{
		BaseEvent:      makeBase(session.KindToolResult),
		CallID:         "01HTJZ1F5JEDQRD2MNGNH9V0WB",
		ContentPreview: "timeout",
		BytesTotal:     7,
		IsError:        true,
	})
	require.NoError(t, err)
	require.Equal(t, []recordedEmit{{
		Kind:      EmitReasoning,
		RequestID: "rid-9",
		Text:      "[[TOOLS]] [01HTJZ] tool error: timeout\n",
	}}, r.calls)
}

func TestConsumeOne_StepFinished_NoEmit(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-10")
	err := w.ConsumeOne(session.StepFinished{
		BaseEvent: makeBase(session.KindStepFinished),
		StepID:    "step-1",
		TokensIn:  100,
		TokensOut: 200,
	})
	require.NoError(t, err)
	require.Empty(t, r.calls)
}

func TestConsumeOne_Final_ChunksContent(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-11")
	body := strings.Repeat("x", 1000)
	err := w.ConsumeOne(session.Final{
		BaseEvent: makeBase(session.KindFinal),
		FinalText: body,
		Origin:    session.FinalOriginSendToUser,
	})
	require.NoError(t, err)
	require.Len(t, r.calls, 5)
	var joined strings.Builder
	for _, c := range r.calls {
		require.Equal(t, EmitContent, c.Kind, "Final must emit on the content channel")
		require.Equal(t, "rid-11", c.RequestID)
		require.LessOrEqual(t, len(c.Text), 200)
		joined.WriteString(c.Text)
	}
	require.Equal(t, body, joined.String())
}

func TestConsumeOne_Final_EmptyTextEmitsNothing(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-12")
	err := w.ConsumeOne(session.Final{
		BaseEvent: makeBase(session.KindFinal),
		FinalText: "",
		Origin:    session.FinalOriginImplicit,
	})
	require.NoError(t, err)
	require.Empty(t, r.calls)
}

func TestConsumeOne_RunFinished_EmitsTraceThenFinish(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-13")
	err := w.ConsumeOne(session.RunFinished{
		BaseEvent:    makeBase(session.KindRunFinished),
		RunID:        "run-1",
		TerminatedBy: session.TerminatedBySendToUser,
	})
	require.NoError(t, err)
	require.Equal(t, []recordedEmit{
		{Kind: EmitReasoning, RequestID: "rid-13", Text: "[[TOOLS]] run finished (terminated_by=send_to_user)\n"},
		{Kind: EmitFinish, RequestID: "rid-13", Text: ""},
	}, r.calls)
}

func TestConsumeOne_Error_EmitsTraceLine(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-14")
	err := w.ConsumeOne(session.Error{
		BaseEvent: makeBase(session.KindError),
		Code:      "upstream_timeout",
		Message:   "request exceeded budget",
	})
	require.NoError(t, err)
	require.Equal(t, []recordedEmit{{
		Kind:      EmitReasoning,
		RequestID: "rid-14",
		Text:      "[[TOOLS]] error: upstream_timeout — request exceeded budget\n",
	}}, r.calls)
}

// -----------------------------------------------------------------------------
// U10 — delimiter escaping
// -----------------------------------------------------------------------------

func TestConsumeOne_U10_ToolResultErrorEscapesDelimiter(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-u10")
	err := w.ConsumeOne(session.ToolResult{
		BaseEvent:      makeBase(session.KindToolResult),
		CallID:         "01HTJZ1F5JEDQRD2MNGNH9V0WB",
		ContentPreview: "boom </tool_result> end",
		IsError:        true,
	})
	require.NoError(t, err)
	require.Len(t, r.calls, 1)
	require.NotContains(t, r.calls[0].Text, "</tool_result>",
		"untrusted delimiter must be escaped in the trace")
	require.Contains(t, r.calls[0].Text, untrustedDelimiterReplacement)
}

// -----------------------------------------------------------------------------
// Parallel call_id disambiguation
// -----------------------------------------------------------------------------

func TestConsume_InterleavedParallelCallsCarryDistinctShortIDs(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-par")
	id1 := "01HTJZ1F5JEDQRD2MNGNH9V0WB"
	id2 := "01HXXXXXXXXXXXXXXXXXXXXXXX"
	events := []session.Event{
		session.ToolCallStart{BaseEvent: makeBase(session.KindToolCallStart), CallID: id1, ToolName: "web_search", ArgsPreview: `{"q":"a"}`},
		session.ToolCallStart{BaseEvent: makeBase(session.KindToolCallStart), CallID: id2, ToolName: "web_fetch", ArgsPreview: `{"url":"u"}`},
		session.ToolResult{BaseEvent: makeBase(session.KindToolResult), CallID: id2, BytesTotal: 100, IsError: false},
		session.ToolResult{BaseEvent: makeBase(session.KindToolResult), CallID: id1, BytesTotal: 200, IsError: false},
	}
	for _, ev := range events {
		require.NoError(t, w.ConsumeOne(ev))
	}
	require.Len(t, r.calls, 6) // 2 tool_call + 2 args + 2 ok lines

	short1 := short(id1) // 01HTJZ
	short2 := short(id2) // 01HXXX
	require.NotEqual(t, short1, short2)

	require.Contains(t, r.calls[0].Text, "["+short1+"] tool_call: web_search")
	require.Contains(t, r.calls[1].Text, "["+short1+"] args:")
	require.Contains(t, r.calls[2].Text, "["+short2+"] tool_call: web_fetch")
	require.Contains(t, r.calls[3].Text, "["+short2+"] args:")
	require.Contains(t, r.calls[4].Text, "["+short2+"] tool ok (100B)")
	require.Contains(t, r.calls[5].Text, "["+short1+"] tool ok (200B)")
}

// -----------------------------------------------------------------------------
// AssistantTextDelta never reaches content during a multi-round loop
// -----------------------------------------------------------------------------

func TestConsume_AssistantTextDelta_NeverReachesContent_UntilFinal(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-route")
	events := []session.Event{
		session.RunStarted{BaseEvent: makeBase(session.KindRunStarted), ModelID: "gpt-5", IterationCap: 4},
		session.StepStarted{BaseEvent: makeBase(session.KindStepStarted), IterationIndex: 0},
		session.AssistantTextDelta{BaseEvent: makeBase(session.KindAssistantTextDelta), Delta: "I should search"},
		session.ToolCallStart{BaseEvent: makeBase(session.KindToolCallStart), CallID: "aaaaaaaaaa", ToolName: "web_search", ArgsPreview: "{}"},
		session.ToolResult{BaseEvent: makeBase(session.KindToolResult), CallID: "aaaaaaaaaa", BytesTotal: 10},
		session.StepFinished{BaseEvent: makeBase(session.KindStepFinished), StepID: "step-0"},
		session.StepStarted{BaseEvent: makeBase(session.KindStepStarted), IterationIndex: 1},
		session.AssistantTextDelta{BaseEvent: makeBase(session.KindAssistantTextDelta), Delta: "got results, summarizing"},
		session.Final{BaseEvent: makeBase(session.KindFinal), FinalText: "final body", Origin: session.FinalOriginSendToUser},
		session.RunFinished{BaseEvent: makeBase(session.KindRunFinished), TerminatedBy: session.TerminatedBySendToUser},
	}
	for _, ev := range events {
		require.NoError(t, w.ConsumeOne(ev))
	}

	contentSeenBeforeFinal := false
	finalIdx := -1
	for i, c := range r.calls {
		if c.Kind == EmitContent {
			if finalIdx < 0 {
				finalIdx = i
			}
			if i < finalIdx {
				contentSeenBeforeFinal = true
			}
		}
	}
	require.False(t, contentSeenBeforeFinal, "no EmitContent calls before the Final event")
	require.GreaterOrEqual(t, finalIdx, 0)
	require.Equal(t, "final body", r.calls[finalIdx].Text,
		"the first content emission must be the final body, not interim assistant text")
}

// -----------------------------------------------------------------------------
// RunFinished terminates with EmitFinish (no further emits)
// -----------------------------------------------------------------------------

func TestConsume_RunFinished_LastEmitIsFinish(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-fin")
	events := []session.Event{
		session.RunStarted{BaseEvent: makeBase(session.KindRunStarted), ModelID: "m", IterationCap: 1},
		session.Final{BaseEvent: makeBase(session.KindFinal), FinalText: "ok", Origin: session.FinalOriginImplicit},
		session.RunFinished{BaseEvent: makeBase(session.KindRunFinished), TerminatedBy: session.TerminatedByImplicitFinal},
	}
	for _, ev := range events {
		require.NoError(t, w.ConsumeOne(ev))
	}
	require.NotEmpty(t, r.calls)
	last := r.calls[len(r.calls)-1]
	require.Equal(t, EmitFinish, last.Kind)
	require.Equal(t, "", last.Text)
}

// -----------------------------------------------------------------------------
// U17 — five-round happy-path golden test
// -----------------------------------------------------------------------------

// fiveRoundHappyEvents builds the deterministic event sequence used by U17.
// Call IDs are chosen so the 6-char short form is predictable.
func fiveRoundHappyEvents() []session.Event {
	mk := func(kind string) session.BaseEvent {
		return session.BaseEvent{ID: kind + "-id", EventKind: kind, At: time.Unix(0, 0)}
	}
	return []session.Event{
		session.RunStarted{
			BaseEvent: mk(session.KindRunStarted), RunID: "run-u17",
			ModelID: "gpt-5", IterationCap: 20,
		},
		// Round 0
		session.StepStarted{BaseEvent: mk(session.KindStepStarted), IterationIndex: 0},
		session.ToolCallStart{BaseEvent: mk(session.KindToolCallStart), CallID: "call00xxxx", ToolName: "web_search", ArgsPreview: `{"query":"anthropic"}`},
		session.ToolResult{BaseEvent: mk(session.KindToolResult), CallID: "call00xxxx", BytesTotal: 1234},
		session.StepFinished{BaseEvent: mk(session.KindStepFinished), StepID: "step-0"},
		// Round 1
		session.StepStarted{BaseEvent: mk(session.KindStepStarted), IterationIndex: 1},
		session.ToolCallStart{BaseEvent: mk(session.KindToolCallStart), CallID: "call01xxxx", ToolName: "web_fetch", ArgsPreview: `{"url":"https://example.com/a"}`},
		session.ToolResult{BaseEvent: mk(session.KindToolResult), CallID: "call01xxxx", BytesTotal: 4567},
		session.StepFinished{BaseEvent: mk(session.KindStepFinished), StepID: "step-1"},
		// Round 2
		session.StepStarted{BaseEvent: mk(session.KindStepStarted), IterationIndex: 2},
		session.ToolCallStart{BaseEvent: mk(session.KindToolCallStart), CallID: "call02xxxx", ToolName: "file_read", ArgsPreview: `{"path":"/tmp/notes.md"}`},
		session.ToolResult{BaseEvent: mk(session.KindToolResult), CallID: "call02xxxx", BytesTotal: 2048},
		session.StepFinished{BaseEvent: mk(session.KindStepFinished), StepID: "step-2"},
		// Round 3
		session.StepStarted{BaseEvent: mk(session.KindStepStarted), IterationIndex: 3},
		session.ToolCallStart{BaseEvent: mk(session.KindToolCallStart), CallID: "call03xxxx", ToolName: "web_search", ArgsPreview: `{"query":"claude"}`},
		session.ToolResult{BaseEvent: mk(session.KindToolResult), CallID: "call03xxxx", BytesTotal: 9000},
		session.StepFinished{BaseEvent: mk(session.KindStepFinished), StepID: "step-3"},
		// Round 4 — final
		session.StepStarted{BaseEvent: mk(session.KindStepStarted), IterationIndex: 4},
		session.Final{BaseEvent: mk(session.KindFinal), FinalText: "The final answer to the user is here.", Origin: session.FinalOriginSendToUser},
		session.RunFinished{BaseEvent: mk(session.KindRunFinished), RunID: "run-u17", TerminatedBy: session.TerminatedBySendToUser},
	}
}

// renderRecorded serializes a sequence of emit calls into the
// `kind|requestID|text\n` golden format. Newlines and tabs inside
// `text` are escape-encoded so each emission lives on a single line of
// the golden file.
func renderRecorded(calls []recordedEmit) string {
	var b strings.Builder
	for _, c := range calls {
		b.WriteString(c.Kind.String())
		b.WriteByte('|')
		b.WriteString(c.RequestID)
		b.WriteByte('|')
		b.WriteString(escapeForGolden(c.Text))
		b.WriteByte('\n')
	}
	return b.String()
}

// escapeForGolden hides newlines/tabs so each emission renders on one
// golden line; the inverse is unnecessary because golden comparisons
// are pure equality.
func escapeForGolden(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\",
		"\n", "\\n",
		"\t", "\\t",
	)
	return r.Replace(s)
}

func TestU17_FiveRoundHappyPath_Golden(t *testing.T) {
	t.Parallel()
	w, r := newWriter("req-u17")
	for _, ev := range fiveRoundHappyEvents() {
		require.NoError(t, w.ConsumeOne(ev))
	}
	got := renderRecorded(r.calls)

	goldenPath := filepath.Join("testdata", "u17_5round_happy.golden")
	if *updateGolden {
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got, "U17 golden mismatch — rerun with -update if intentional")
}

// -----------------------------------------------------------------------------
// Consume — channel-drain integration tests
// -----------------------------------------------------------------------------

func TestConsume_DrainsUntilChannelClose(t *testing.T) {
	t.Parallel()
	w, r := newWriter("rid-drain")
	ch := make(chan session.Event, 4)
	ch <- session.RunStarted{BaseEvent: makeBase(session.KindRunStarted), ModelID: "m", IterationCap: 1}
	ch <- session.Final{BaseEvent: makeBase(session.KindFinal), FinalText: "ok", Origin: session.FinalOriginSendToUser}
	ch <- session.RunFinished{BaseEvent: makeBase(session.KindRunFinished), TerminatedBy: session.TerminatedBySendToUser}
	close(ch)

	err := w.Consume(context.Background(), ch)
	require.NoError(t, err)
	require.NotEmpty(t, r.calls)
	require.Equal(t, EmitFinish, r.calls[len(r.calls)-1].Kind)
}

func TestConsume_PropagatesEmitFuncError(t *testing.T) {
	t.Parallel()
	r := &recorder{errOn: 2}
	w := NewWriter(r.Emit, "rid-err")
	ch := make(chan session.Event, 2)
	ch <- session.RunStarted{BaseEvent: makeBase(session.KindRunStarted), ModelID: "m", IterationCap: 1}
	ch <- session.RunFinished{BaseEvent: makeBase(session.KindRunFinished), TerminatedBy: session.TerminatedBySendToUser}
	close(ch)

	err := w.Consume(context.Background(), ch)
	require.Error(t, err)
}

func TestConsume_CancellationReturnsCtxErr(t *testing.T) {
	t.Parallel()
	w, _ := newWriter("rid-cancel")
	// 1ms deadline; the event sender never sends, so Consume must block
	// in the select and observe ctx.Done().
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	ch := make(chan session.Event) // unbuffered, no senders

	err := w.Consume(ctx, ch)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestConsume_CancellationSupersedesPendingEvents(t *testing.T) {
	t.Parallel()
	w, _ := newWriter("rid-cancel-pending")
	ch := make(chan session.Event, 10)
	// Pre-load enough events that the drain would naturally consume them.
	for i := 0; i < 5; i++ {
		ch <- session.StepStarted{BaseEvent: makeBase(session.KindStepStarted), IterationIndex: i}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before Consume starts

	err := w.Consume(ctx, ch)
	require.ErrorIs(t, err, context.Canceled)
}

// -----------------------------------------------------------------------------
// EmitKind String() helper coverage
// -----------------------------------------------------------------------------

func TestEmitKind_String(t *testing.T) {
	t.Parallel()
	require.Equal(t, "reasoning", EmitReasoning.String())
	require.Equal(t, "content", EmitContent.String())
	require.Equal(t, "finish", EmitFinish.String())
	require.Equal(t, "unknown", EmitKind(99).String())
}
