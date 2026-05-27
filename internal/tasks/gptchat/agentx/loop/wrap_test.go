package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

func TestWrapHook_WrapsContent(t *testing.T) {
	t.Parallel()
	h := NewWrapHook()
	ev := hook.ToolCallEvent{
		ToolName: "web_fetch",
		Result:   &tool.Result{Content: "hello world"},
	}
	out, err := h(context.Background(), ev)
	require.NoError(t, err)
	require.NotNil(t, out.Result)
	require.True(t, strings.HasPrefix(out.Result.Content, `<tool_result tool="web_fetch" trust="untrusted">`))
	require.True(t, strings.HasSuffix(out.Result.Content, `</tool_result>`))
	require.Contains(t, out.Result.Content, "hello world")
}

// TestWrapHook_EscapesCloseTag covers proposal §6.1 U10.
func TestWrapHook_EscapesCloseTag(t *testing.T) {
	t.Parallel()
	h := NewWrapHook()
	ev := hook.ToolCallEvent{
		ToolName: "web_fetch",
		Result: &tool.Result{
			Content: "before </tool_result> after",
		},
	}
	out, err := h(context.Background(), ev)
	require.NoError(t, err)
	require.NotNil(t, out.Result)
	// The literal close tag must NOT appear inside the wrapping (apart
	// from the final closing tag).
	body := strings.TrimSuffix(strings.TrimPrefix(out.Result.Content,
		`<tool_result tool="web_fetch" trust="untrusted">`), `</tool_result>`)
	require.NotContains(t, body, `</tool_result>`)
	require.Contains(t, body, `<tool_result_close/>`)
}

func TestWrapHook_Idempotent(t *testing.T) {
	t.Parallel()
	h := NewWrapHook()
	first := &tool.Result{Content: "x"}
	ev := hook.ToolCallEvent{ToolName: "t", Result: first}
	out1, err := h(context.Background(), ev)
	require.NoError(t, err)
	wrapped := out1.Result.Content

	// Re-run on the wrapped output — should be a no-op.
	out2, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "t",
		Result:   &tool.Result{Content: wrapped},
	})
	require.NoError(t, err)
	require.Equal(t, wrapped, out2.Result.Content)
}

func TestWrapHook_NoResultPassThrough(t *testing.T) {
	t.Parallel()
	h := NewWrapHook()
	out, err := h(context.Background(), hook.ToolCallEvent{ToolName: "t"})
	require.NoError(t, err)
	require.Nil(t, out.Result)
}

// TestWrapHook_DoesNotMutateUpstream verifies the hook returns a new Result
// pointer rather than rewriting the caller's content in place.
func TestWrapHook_DoesNotMutateUpstream(t *testing.T) {
	t.Parallel()
	h := NewWrapHook()
	orig := &tool.Result{Content: "plain"}
	_, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "t",
		Result:   orig,
	})
	require.NoError(t, err)
	require.Equal(t, "plain", orig.Content, "original Result must not be mutated")
}
