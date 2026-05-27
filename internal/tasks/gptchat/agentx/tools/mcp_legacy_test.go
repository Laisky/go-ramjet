package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// recordingDeps captures the inputs the provider was asked for, so the test
// can assert the (callID, toolName) pair routed through correctly.
type recordingDeps struct {
	gotCtx      context.Context
	gotCallID   string
	gotToolName string
	out         httppkg.LegacyDeps
	err         error
}

func (r *recordingDeps) LegacyDeps(ctx context.Context, callID, toolName string) (httppkg.LegacyDeps, error) {
	r.gotCtx = ctx
	r.gotCallID = callID
	r.gotToolName = toolName
	return r.out, r.err
}

// installFakeDispatcher swaps the package-level dispatcher with a recording
// fake and returns a cleanup func to restore the original. Tests that
// exercise legacyDispatchTool.Execute through the public constructor go
// through this seam.
func installFakeDispatcher(t *testing.T, fn legacyDispatcher) func() {
	t.Helper()
	orig := defaultLegacyDispatcher
	defaultLegacyDispatcher = fn
	return func() { defaultLegacyDispatcher = orig }
}

func TestLegacyDispatch_RoundTripsCall(t *testing.T) {
	// Cannot use t.Parallel here: we mutate the package-level dispatcher.
	var (
		gotDeps httppkg.LegacyDeps
		gotFC   httppkg.OpenAIResponsesFunctionCall
	)
	restore := installFakeDispatcher(t, func(_ context.Context, deps httppkg.LegacyDeps, fc httppkg.OpenAIResponsesFunctionCall) (string, string, error) {
		gotDeps = deps
		gotFC = fc
		return `{"hits":3}`, "exec local tool: web_search", nil
	})
	defer restore()

	provider := &recordingDeps{
		out: httppkg.LegacyDeps{
			User: &config.UserConfig{UserName: "u1"},
		},
	}
	tt := NewLegacyDispatchTool(
		"web_search",
		"search the web",
		json.RawMessage(`{"type":"object"}`),
		provider,
	)
	require.Equal(t, "web_search", tt.Name())
	require.Equal(t, "search the web", tt.Description())
	require.JSONEq(t, `{"type":"object"}`, string(tt.Schema()))

	args := json.RawMessage(`{"q":"anthropic claude blog"}`)
	res, err := tt.Execute(context.Background(), tool.Call{
		CallID: "call_42",
		Name:   "web_search",
		Args:   args,
	}, nil)
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Equal(t, `{"hits":3}`, res.Content)

	// Provider was called with the right (callID, toolName).
	require.Equal(t, "call_42", provider.gotCallID)
	require.Equal(t, "web_search", provider.gotToolName)

	// Deps round-tripped to ExecuteToolCallCtx untouched.
	require.NotNil(t, gotDeps.User)
	require.Equal(t, "u1", gotDeps.User.UserName)

	// FunctionCall envelope built correctly.
	require.Equal(t, "function_call", gotFC.Type)
	require.Equal(t, "call_42", gotFC.CallID)
	require.Equal(t, "web_search", gotFC.Name)
	require.Equal(t, string(args), gotFC.Arguments)
}

func TestLegacyDispatch_ErrorBecomesIsError(t *testing.T) {
	restore := installFakeDispatcher(t, func(_ context.Context, _ httppkg.LegacyDeps, _ httppkg.OpenAIResponsesFunctionCall) (string, string, error) {
		return "", "exec MCP tool: web_search @ http://mcp", errors.New("upstream 500")
	})
	defer restore()

	tt := NewLegacyDispatchTool(
		"web_search",
		"",
		json.RawMessage(`{}`),
		LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) {
			return httppkg.LegacyDeps{}, nil
		}),
	)
	res, err := tt.Execute(context.Background(), tool.Call{
		CallID: "c1",
		Args:   json.RawMessage(`{}`),
	}, nil)
	require.NoError(t, err, "tool errors must NOT escape as Go errors")
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "upstream 500")
}

func TestLegacyDispatch_DispatcherErrorWithPartialOutput(t *testing.T) {
	restore := installFakeDispatcher(t, func(_ context.Context, _ httppkg.LegacyDeps, _ httppkg.OpenAIResponsesFunctionCall) (string, string, error) {
		return "partial output", "exec MCP tool: web_search @ http://mcp", errors.New("connection reset")
	})
	defer restore()

	tt := NewLegacyDispatchTool("web_search", "", nil, LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) {
		return httppkg.LegacyDeps{}, nil
	}))
	res, err := tt.Execute(context.Background(), tool.Call{Args: json.RawMessage(`{}`)}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "partial output")
	require.Contains(t, res.Content, "connection reset")
}

func TestLegacyDispatch_ProviderError(t *testing.T) {
	// No dispatcher swap needed: the provider error must short-circuit
	// before defaultLegacyDispatcher is consulted.
	tt := NewLegacyDispatchTool("web_search", "", nil, LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) {
		return httppkg.LegacyDeps{}, errors.New("missing user")
	}))
	res, err := tt.Execute(context.Background(), tool.Call{Args: json.RawMessage(`{}`)}, nil)
	require.Error(t, err) // provider failures DO propagate the underlying go error
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "missing user")
}

func TestLegacyDispatch_NilProviderIsErrorResult(t *testing.T) {
	t.Parallel()
	tt := NewLegacyDispatchTool("web_search", "", nil, nil)
	res, err := tt.Execute(context.Background(), tool.Call{Args: json.RawMessage(`{}`)}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "missing LegacyDepsProvider")
}

func TestLegacyDepsFunc_Adapter(t *testing.T) {
	t.Parallel()
	called := false
	var provider LegacyDepsProvider = LegacyDepsFunc(func(_ context.Context, callID, toolName string) (httppkg.LegacyDeps, error) {
		called = true
		require.Equal(t, "c1", callID)
		require.Equal(t, "web_search", toolName)
		return httppkg.LegacyDeps{RawUserToken: "tok"}, nil
	})
	deps, err := provider.LegacyDeps(context.Background(), "c1", "web_search")
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "tok", deps.RawUserToken)
}
