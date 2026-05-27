package session

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// U18 — append-only invariant.
func TestTranscript_AppendOnlyRejectsDuplicateID(t *testing.T) {
	t.Parallel()
	tr := NewTranscript()
	original := StepStarted{
		BaseEvent: BaseEvent{ID: "dup", EventKind: KindStepStarted, At: time.Now()},
		StepID:    "step-1",
	}
	require.NoError(t, tr.Append(original))

	dup := StepStarted{
		BaseEvent: BaseEvent{ID: "dup", EventKind: KindStepStarted, At: time.Now()},
		StepID:    "step-2-different-payload",
	}
	err := tr.Append(dup)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate EventID")
	require.Contains(t, err.Error(), `"dup"`)

	// Existing event is not mutated and the rejected event never appears.
	events := tr.Events()
	require.Len(t, events, 1)
	got, ok := events[0].(StepStarted)
	require.True(t, ok)
	require.Equal(t, "step-1", got.StepID)

	// A second snapshot after the rejection still excludes the duplicate.
	again := tr.Events()
	require.Equal(t, events, again)
}

func TestTranscript_AppendRejectsNilAndEmptyID(t *testing.T) {
	t.Parallel()
	tr := NewTranscript()
	require.Error(t, tr.Append(nil))
	require.Error(t, tr.Append(StepStarted{}))
}

func TestTranscript_TreeByParent(t *testing.T) {
	t.Parallel()
	tr := NewTranscript()
	root := StepStarted{BaseEvent: BaseEvent{ID: "r1", EventKind: KindStepStarted, At: time.Now()}}
	child := StepStarted{BaseEvent: BaseEvent{ID: "c1", ParentID: "r1", EventKind: KindStepStarted, At: time.Now()}}
	grand := StepStarted{BaseEvent: BaseEvent{ID: "g1", ParentID: "c1", EventKind: KindStepStarted, At: time.Now()}}
	require.NoError(t, tr.Append(root))
	require.NoError(t, tr.Append(child))
	require.NoError(t, tr.Append(grand))

	tree := tr.Tree()
	require.Len(t, tree.ByID, 3)
	require.Equal(t, "r1", tree.Children[""][0].EventID())
	require.Equal(t, "c1", tree.Children["r1"][0].EventID())
	require.Equal(t, "g1", tree.Children["c1"][0].EventID())
}

// Branch shares ancestors and isolates subsequent appends.
func TestTranscript_BranchSharesAncestors(t *testing.T) {
	t.Parallel()
	tr := NewTranscript()
	t0 := time.Now()
	ids := []string{"a", "b", "c", "d", "e"}
	for i, id := range ids {
		require.NoError(t, tr.Append(StepStarted{
			BaseEvent: BaseEvent{ID: id, EventKind: KindStepStarted, At: t0.Add(time.Duration(i) * time.Millisecond)},
			StepID:    id,
		}))
	}

	branch, err := tr.Branch("c")
	require.NoError(t, err)

	branchEvents := branch.Events()
	require.Len(t, branchEvents, 3)
	require.Equal(t, "a", branchEvents[0].EventID())
	require.Equal(t, "b", branchEvents[1].EventID())
	require.Equal(t, "c", branchEvents[2].EventID())

	pivotTS := branchEvents[2].Timestamp()
	for _, ev := range branchEvents {
		require.False(t, ev.Timestamp().After(pivotTS), "event %s should be <= pivot", ev.EventID())
	}

	// Appends on the branch do not leak to the parent.
	require.NoError(t, branch.Append(StepStarted{
		BaseEvent: BaseEvent{ID: "branch-only", EventKind: KindStepStarted, At: time.Now()},
	}))
	require.Len(t, branch.Events(), 4)
	require.Len(t, tr.Events(), 5)

	// And vice versa.
	require.NoError(t, tr.Append(StepStarted{
		BaseEvent: BaseEvent{ID: "parent-only", EventKind: KindStepStarted, At: time.Now()},
	}))
	require.Len(t, tr.Events(), 6)
	require.Len(t, branch.Events(), 4)
}

func TestTranscript_BranchRejectsMissingPivot(t *testing.T) {
	t.Parallel()
	tr := NewTranscript()
	_, err := tr.Branch("nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	_, err = tr.Branch("")
	require.Error(t, err)
}

// JSONL round-trip preserves typed event fields and timestamps.
func TestTranscript_JSONLRoundTrip(t *testing.T) {
	t.Parallel()
	tr := NewTranscript()
	now := time.Now().UTC().Round(time.Millisecond)
	events := []Event{
		RunStarted{
			BaseEvent:    BaseEvent{ID: "ev-1", EventKind: KindRunStarted, At: now},
			RunID:        "run-1",
			ModelID:      "gpt-x",
			ToolNames:    []string{"web_fetch", "send_to_user"},
			IterationCap: 20,
		},
		StepStarted{
			BaseEvent:      BaseEvent{ID: "ev-2", ParentID: "ev-1", EventKind: KindStepStarted, At: now.Add(time.Millisecond)},
			StepID:         "step-1",
			IterationIndex: 0,
		},
		ToolCallStart{
			BaseEvent:   BaseEvent{ID: "ev-3", ParentID: "ev-2", EventKind: KindToolCallStart, At: now.Add(2 * time.Millisecond)},
			CallID:      "call-abc",
			ToolName:    "web_fetch",
			ArgsPreview: `{"url":"https://example.com"}`,
		},
		ToolResult{
			BaseEvent:      BaseEvent{ID: "ev-4", ParentID: "ev-3", EventKind: KindToolResult, At: now.Add(3 * time.Millisecond)},
			CallID:         "call-abc",
			ContentPreview: "ok",
			BytesTotal:     1024,
			IsError:        false,
		},
		Final{
			BaseEvent: BaseEvent{ID: "ev-5", ParentID: "ev-2", EventKind: KindFinal, At: now.Add(4 * time.Millisecond)},
			FinalText: "done",
			Citations: []Citation{{URL: "https://example.com", Title: "Example"}},
			Origin:    FinalOriginSendToUser,
		},
		RunFinished{
			BaseEvent:    BaseEvent{ID: "ev-6", ParentID: "ev-1", EventKind: KindRunFinished, At: now.Add(5 * time.Millisecond)},
			RunID:        "run-1",
			TerminatedBy: TerminatedBySendToUser,
			TotalUsage:   TotalUsage{TokensIn: 100, TokensOut: 50, ToolCalls: 1, Iterations: 1},
		},
		Error{
			BaseEvent: BaseEvent{ID: "ev-7", EventKind: KindError, At: now.Add(6 * time.Millisecond)},
			Code:      "boom",
			Message:   "transport failed",
		},
	}
	for _, ev := range events {
		require.NoError(t, tr.Append(ev))
	}

	var buf bytes.Buffer
	require.NoError(t, tr.JSONL(&buf))

	// One line per event.
	require.Equal(t, len(events), bytes.Count(buf.Bytes(), []byte{'\n'}))

	parsed, err := ParseJSONL(&buf)
	require.NoError(t, err)
	require.Len(t, parsed, len(events))

	for i, want := range events {
		got := parsed[i]
		require.Equal(t, want.EventID(), got.EventID(), "event %d EventID", i)
		require.Equal(t, want.ParentEventID(), got.ParentEventID(), "event %d ParentEventID", i)
		require.Equal(t, want.Kind(), got.Kind(), "event %d Kind", i)
		require.True(t, want.Timestamp().Equal(got.Timestamp()), "event %d Timestamp: want %v got %v", i, want.Timestamp(), got.Timestamp())
	}

	// Spot-check typed fields are preserved.
	rs, ok := parsed[0].(RunStarted)
	require.True(t, ok)
	require.Equal(t, "run-1", rs.RunID)
	require.Equal(t, []string{"web_fetch", "send_to_user"}, rs.ToolNames)
	require.Equal(t, 20, rs.IterationCap)

	tc, ok := parsed[2].(ToolCallStart)
	require.True(t, ok)
	require.Equal(t, "call-abc", tc.CallID)
	require.Equal(t, "web_fetch", tc.ToolName)

	fn, ok := parsed[4].(Final)
	require.True(t, ok)
	require.Equal(t, "done", fn.FinalText)
	require.Equal(t, []Citation{{URL: "https://example.com", Title: "Example"}}, fn.Citations)
	require.Equal(t, FinalOriginSendToUser, fn.Origin)

	rf, ok := parsed[5].(RunFinished)
	require.True(t, ok)
	require.Equal(t, TerminatedBySendToUser, rf.TerminatedBy)
	require.Equal(t, TotalUsage{TokensIn: 100, TokensOut: 50, ToolCalls: 1, Iterations: 1}, rf.TotalUsage)
}

func TestTranscript_EventsReturnsCopy(t *testing.T) {
	t.Parallel()
	tr := NewTranscript()
	require.NoError(t, tr.Append(StepStarted{BaseEvent: BaseEvent{ID: "x", EventKind: KindStepStarted, At: time.Now()}}))
	snap := tr.Events()
	require.Len(t, snap, 1)
	snap[0] = nil // mutating the snapshot should not affect the transcript

	require.Len(t, tr.Events(), 1)
	require.NotNil(t, tr.Events()[0])
}
