package hook

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestErrAskUser_Error verifies the canonical rendering and the nil-guard.
func TestErrAskUser_Error(t *testing.T) {
	t.Parallel()

	e := &ErrAskUser{Code: "write_gate", Message: "Confirm?"}
	require.Equal(t, "ask_user[write_gate]: Confirm?", e.Error())

	var nilErr *ErrAskUser
	require.Equal(t, "", nilErr.Error())
}

// TestErrAskUser_RoundTripViaErrorsAs is the proposal §3.7 contract test:
// a hook returns &ErrAskUser{...}; the dispatcher returns it; the caller
// uses errors.As(err, &hook.ErrAskUser{}) to extract Code and Message.
func TestErrAskUser_RoundTripViaErrorsAs(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	bus.OnBeforeToolCall(func(_ context.Context, ev ToolCallEvent) (ToolCallEvent, error) {
		return ev, &ErrAskUser{
			Code:    "write_gate",
			Message: "Confirm?",
			Details: map[string]any{"tool": "file_write"},
		}
	})

	_, err := bus.DispatchBeforeToolCall(context.Background(), ToolCallEvent{ToolName: "file_write"})
	require.Error(t, err)

	var asAsk *ErrAskUser
	require.True(t, errors.As(err, &asAsk), "errors.As must extract *ErrAskUser")
	require.NotNil(t, asAsk)
	require.Equal(t, "write_gate", asAsk.Code)
	require.Equal(t, "Confirm?", asAsk.Message)
	require.Equal(t, "file_write", asAsk.Details["tool"])
}

// TestErrAskUser_DistinctFromPlainError ensures a non-ErrAskUser failure
// does NOT errors.As into the sentinel — the loop relies on this to keep
// "deny" semantics distinct from "ask".
func TestErrAskUser_DistinctFromPlainError(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	bus.OnBeforeToolCall(func(_ context.Context, ev ToolCallEvent) (ToolCallEvent, error) {
		return ev, errors.New("plain deny")
	})

	_, err := bus.DispatchBeforeToolCall(context.Background(), ToolCallEvent{})
	require.Error(t, err)

	var asAsk *ErrAskUser
	require.False(t, errors.As(err, &asAsk))
}

// TestErrAskUser_PointerReceiverContract verifies the Error() method is on
// the pointer receiver, which is what makes errors.As(err, &ErrAskUser{})
// work the way the loop expects.
func TestErrAskUser_PointerReceiverContract(t *testing.T) {
	t.Parallel()

	var err error = &ErrAskUser{Code: "circuit_breaker", Message: "loop is stuck"}
	require.Contains(t, err.Error(), "circuit_breaker")
	require.True(t, strings.HasPrefix(err.Error(), "ask_user["))
}
