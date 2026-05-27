package loop

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// dummyResult is used by tests that need a stable Result pointer.
var dummyResult = tool.Result{Content: "dummy"}

func TestWriteGate_Ask_ReturnsErrAskUser(t *testing.T) {
	t.Parallel()
	h := NewWriteGateHook(WriteGateAsk)

	_, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "file_write",
		CallID:   "c1",
		Args:     stdjson.RawMessage(`{"path":"/tmp/x","content":"y"}`),
	})
	require.Error(t, err)
	var ask *hook.ErrAskUser
	require.True(t, errors.As(err, &ask))
	require.Equal(t, "write_gate", ask.Code)
	require.Contains(t, ask.Message, "file_write")
	require.Contains(t, ask.Message, "/tmp/x")
	require.NotNil(t, ask.Details)
	require.Equal(t, "file_write", ask.Details["tool"])
}

func TestWriteGate_Deny_SynthesizesIsError(t *testing.T) {
	t.Parallel()
	h := NewWriteGateHook(WriteGateDeny)

	out, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "file_delete",
		Args:     stdjson.RawMessage(`{"path":"/x"}`),
	})
	require.NoError(t, err)
	require.NotNil(t, out.Result)
	require.True(t, out.Result.IsError)
	require.Contains(t, out.Result.Content, "disabled")
}

func TestWriteGate_Allow_PassThrough(t *testing.T) {
	t.Parallel()
	h := NewWriteGateHook(WriteGateAllow)
	out, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "file_write",
		Args:     stdjson.RawMessage(`{"path":"/x"}`),
	})
	require.NoError(t, err)
	require.Nil(t, out.Result)
}

func TestWriteGate_NonWriteTool_AlwaysPassThrough(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{WriteGateAsk, WriteGateDeny, WriteGateAllow} {
		h := NewWriteGateHook(mode)
		out, err := h(context.Background(), hook.ToolCallEvent{
			ToolName: "web_search",
			Args:     stdjson.RawMessage(`{"q":"x"}`),
		})
		require.NoError(t, err, "mode=%s", mode)
		require.Nil(t, out.Result, "mode=%s", mode)
	}
}

func TestWriteGate_UnknownMode_FailsOpen(t *testing.T) {
	t.Parallel()
	h := NewWriteGateHook("typo")
	out, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "file_write",
		Args:     stdjson.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.Nil(t, out.Result)
}

func TestWriteGate_AfterToolCall_PassThrough(t *testing.T) {
	t.Parallel()
	// Defensive guard: if a Before hook ever sees a populated Result, we
	// should pass through unmodified instead of looping the ask-user
	// prompt.
	h := NewWriteGateHook(WriteGateAsk)
	res := &dummyResult
	out, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "file_write",
		Result:   res,
	})
	require.NoError(t, err)
	require.Same(t, res, out.Result)
}
