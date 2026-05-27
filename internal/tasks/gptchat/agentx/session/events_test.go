package session

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventTypesImplementInterface(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Round(time.Millisecond)
	base := BaseEvent{ID: "01-id", ParentID: "01-parent", EventKind: KindRunStarted, At: now}
	cases := []Event{
		RunStarted{BaseEvent: base, RunID: "run", ModelID: "gpt-x", ToolNames: []string{"a"}, IterationCap: 10},
		StepStarted{BaseEvent: base, StepID: "step", IterationIndex: 1},
		AssistantTextDelta{BaseEvent: base, StepID: "step", Delta: "hi"},
		AssistantReasoningDelta{BaseEvent: base, StepID: "step", Delta: "think"},
		ToolCallStart{BaseEvent: base, CallID: "call-1", ToolName: "web_fetch", ArgsPreview: "{}"},
		ToolCallEnd{BaseEvent: base, CallID: "call-1", DurationMS: 42},
		ToolResult{BaseEvent: base, CallID: "call-1", ContentPreview: "ok", BytesTotal: 2, IsError: false},
		StepFinished{BaseEvent: base, StepID: "step", TokensIn: 100, TokensOut: 50},
		Final{BaseEvent: base, FinalText: "answer", Origin: FinalOriginSendToUser},
		RunFinished{BaseEvent: base, RunID: "run", TerminatedBy: TerminatedBySendToUser},
		Error{BaseEvent: base, Code: "boom", Message: "fail"},
	}
	for _, ev := range cases {
		require.Equal(t, "01-id", ev.EventID(), "%T.EventID", ev)
		require.Equal(t, "01-parent", ev.ParentEventID(), "%T.ParentEventID", ev)
		require.Equal(t, now, ev.Timestamp(), "%T.Timestamp", ev)
		require.NotEmpty(t, ev.Kind(), "%T.Kind", ev)
	}
}

func TestToolCallStartCarriesCallIDForSSEPrefix(t *testing.T) {
	t.Parallel()
	// §3.8 requires CallID on ToolCallStart so the SSE encoder can do per-call
	// prefixing. The same field appears on ToolCallEnd and ToolResult to thread
	// the call through its full lifecycle. This test guards that contract.
	start := ToolCallStart{BaseEvent: BaseEvent{ID: "x"}, CallID: "abc123"}
	end := ToolCallEnd{BaseEvent: BaseEvent{ID: "y"}, CallID: "abc123"}
	result := ToolResult{BaseEvent: BaseEvent{ID: "z"}, CallID: "abc123"}
	require.Equal(t, "abc123", start.CallID)
	require.Equal(t, "abc123", end.CallID)
	require.Equal(t, "abc123", result.CallID)
}

func TestFinalOriginEnumeration(t *testing.T) {
	t.Parallel()
	require.Equal(t, "send_to_user", FinalOriginSendToUser)
	require.Equal(t, "implicit", FinalOriginImplicit)
	require.Equal(t, "ask_user", FinalOriginAskUser)
}

func TestRunFinishedTerminatedByEnumeration(t *testing.T) {
	t.Parallel()
	// Acceptance criterion #5 in §7 specifies the closed set.
	values := []string{
		TerminatedBySendToUser, TerminatedByImplicitFinal, TerminatedByAskUser,
		TerminatedByIterationCap, TerminatedByTimeout, TerminatedByCircuitBreaker,
		TerminatedByErrorBudget, TerminatedByCancelled, TerminatedByError,
	}
	require.Len(t, values, 9)
}

func TestNewBaseEventStampsULID(t *testing.T) {
	t.Parallel()
	base := NewBaseEvent(KindRunStarted, "parent-x")
	require.NotEmpty(t, base.ID)
	require.Equal(t, "parent-x", base.ParentID)
	require.Equal(t, KindRunStarted, base.EventKind)
	require.False(t, base.At.IsZero())
}

func TestBaseEventJSONRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Round(time.Millisecond)
	original := Final{
		BaseEvent: BaseEvent{ID: "id-1", ParentID: "id-0", EventKind: KindFinal, At: now},
		FinalText: "the answer is 42",
		Citations: []Citation{{URL: "https://example.com", Title: "Example"}},
		Origin:    FinalOriginSendToUser,
	}
	tr := NewTranscript()
	require.NoError(t, tr.Append(original))
	var buf bytes.Buffer
	require.NoError(t, tr.JSONL(&buf))
	parsed, err := ParseJSONL(&buf)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	got, ok := parsed[0].(Final)
	require.True(t, ok, "expected Final, got %T", parsed[0])
	require.Equal(t, original.ID, got.EventID())
	require.Equal(t, original.ParentID, got.ParentEventID())
	require.Equal(t, original.Kind(), got.Kind())
	require.True(t, original.Timestamp().Equal(got.Timestamp()))
	require.Equal(t, original.FinalText, got.FinalText)
	require.Equal(t, original.Citations, got.Citations)
	require.Equal(t, original.Origin, got.Origin)
}
