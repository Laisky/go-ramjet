package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
)

// stubTool is a minimal Tool implementation used by tests in this package.
type stubTool struct {
	name        string
	description string
	schema      json.RawMessage
	execFn      func(ctx context.Context, call Call, sink session.EventSink) (Result, error)
}

func (s *stubTool) Name() string             { return s.name }
func (s *stubTool) Description() string      { return s.description }
func (s *stubTool) Schema() json.RawMessage  { return s.schema }
func (s *stubTool) Execute(ctx context.Context, call Call, sink session.EventSink) (Result, error) {
	if s.execFn != nil {
		return s.execFn(ctx, call, sink)
	}
	return Result{Content: "ok"}, nil
}

// nullSink is a private EventSink stub. The package builds against the real
// session.EventSink contract; this stub just absorbs whatever the tool emits.
type nullSink struct{ events []session.Event }

func (n *nullSink) Emit(ev session.Event) error {
	n.events = append(n.events, ev)
	return nil
}

func TestSource_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src  Source
		want string
	}{
		{SourceLocal, "local"},
		{SourceCuratedMCP, "curated_mcp"},
		{SourceUserMCP, "user_mcp"},
		{Source(99), "unknown"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, c.src.String())
	}
}

func TestTool_ExecuteReceivesArgsAndSink(t *testing.T) {
	t.Parallel()
	var observedCall Call
	var observedSink session.EventSink
	tt := &stubTool{
		name: "echo",
		execFn: func(_ context.Context, call Call, sink session.EventSink) (Result, error) {
			observedCall = call
			observedSink = sink
			return Result{Content: string(call.Args)}, nil
		},
	}
	sink := &nullSink{}
	res, err := tt.Execute(context.Background(), Call{
		CallID: "c1",
		Name:   "echo",
		Args:   json.RawMessage(`{"x":1}`),
	}, sink)
	require.NoError(t, err)
	require.Equal(t, `{"x":1}`, res.Content)
	require.False(t, res.IsError)
	require.Equal(t, "c1", observedCall.CallID)
	require.Equal(t, json.RawMessage(`{"x":1}`), observedCall.Args)
	require.Same(t, sink, observedSink)
}

func TestResult_IsErrorIsAdditiveNotFatal(t *testing.T) {
	t.Parallel()
	// IsError is the wire shape the loop uses to charge the error budget;
	// confirm a Result without an error is still well-formed.
	r := Result{Content: "boom", IsError: true}
	require.True(t, r.IsError)
	require.Equal(t, "boom", r.Content)
}
