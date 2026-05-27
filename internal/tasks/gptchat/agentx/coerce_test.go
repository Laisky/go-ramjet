package agentx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// U_Coerce_MixedShapes covers the live e2e repro: the seed for the agent
// loop is a []any whose entries are a mixture of the typed structs the
// OneAPI validator expects (left over from convert2UpstreamResponsesRequest)
// and the bare map[string]any items the memory enrichment hook emits via
// MemoryItemsToResponsesInput. The helper must convert every map back into
// the matching typed struct.
func TestCoerceInputItems_MixedShapes(t *testing.T) {
	typedMsg := httppkg.OpenAIResponsesInputMessage{
		Role:    "system",
		Content: "you are a helpful agent",
	}
	mapUserMsg := map[string]any{
		"role":    "user",
		"content": "hi",
	}
	mapFunctionCall := map[string]any{
		"type":      "function_call",
		"name":      "web_search",
		"arguments": "{}",
		"call_id":   "abc",
	}
	mapFunctionCallOutput := map[string]any{
		"type":    "function_call_output",
		"call_id": "abc",
		"output":  "result text",
	}

	in := []any{typedMsg, mapUserMsg, mapFunctionCall, mapFunctionCallOutput}
	out, err := coerceInputItems(in)
	require.NoError(t, err)
	require.Len(t, out, 4)

	// Index 0 — typed input message passes through unchanged.
	gotTyped, ok := out[0].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok, "out[0] kind=%T", out[0])
	require.Equal(t, typedMsg, gotTyped)

	// Index 1 — map user message round-trips to OpenAIResponsesInputMessage.
	gotUser, ok := out[1].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok, "out[1] kind=%T", out[1])
	require.Equal(t, "user", gotUser.Role)
	require.Equal(t, "hi", gotUser.Content)

	// Index 2 — map function call → OpenAIResponsesFunctionCall.
	gotCall, ok := out[2].(httppkg.OpenAIResponsesFunctionCall)
	require.True(t, ok, "out[2] kind=%T", out[2])
	require.Equal(t, "function_call", gotCall.Type)
	require.Equal(t, "web_search", gotCall.Name)
	require.Equal(t, "{}", gotCall.Arguments)
	require.Equal(t, "abc", gotCall.CallID)

	// Index 3 — map function call output → OpenAIResponsesFunctionCallOutput.
	gotOut, ok := out[3].(httppkg.OpenAIResponsesFunctionCallOutput)
	require.True(t, ok, "out[3] kind=%T", out[3])
	require.Equal(t, "function_call_output", gotOut.Type)
	require.Equal(t, "abc", gotOut.CallID)
	require.Equal(t, "result text", gotOut.Output)
}

// U_Coerce_UnknownShape — an item with neither a recognised "type" nor a
// "role" must surface as a wrapped error that names the index AND a
// preview of the offending payload so future schema drift is easy to
// debug from a single log line.
func TestCoerceInputItems_UnknownShape(t *testing.T) {
	in := []any{
		httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hello"},
		map[string]any{"weird_field": 42, "kind": "tool_output"},
	}
	_, err := coerceInputItems(in)
	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "Input[1]", "error must name the failing index; got %q", msg)
	require.Contains(t, msg, "preview=", "error must carry a payload preview; got %q", msg)
	// The preview should contain at least one field from the offending
	// item so debugging future drift is fast.
	require.True(t,
		strings.Contains(msg, "weird_field") || strings.Contains(msg, "tool_output"),
		"preview must include a recognisable field from the failing item; got %q", msg)
}

// U_Coerce_TypedPassthrough — a slice already in the typed input shape
// must round-trip without losing value identity (typed items returned
// exactly as supplied).
func TestCoerceInputItems_TypedPassthrough(t *testing.T) {
	msg := httppkg.OpenAIResponsesInputMessage{
		Role:    "assistant",
		Content: "ok",
	}
	call := httppkg.OpenAIResponsesFunctionCall{
		Type:      "function_call",
		ID:        "fc-1",
		CallID:    "cc-1",
		Name:      "web_fetch",
		Arguments: `{"url":"https://x"}`,
	}
	out := httppkg.OpenAIResponsesFunctionCallOutput{
		Type:   "function_call_output",
		CallID: "cc-1",
		Output: "200 OK",
	}

	in := []any{msg, call, out}
	got, err := coerceInputItems(in)
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, msg, got[0])
	require.Equal(t, call, got[1])
	require.Equal(t, out, got[2])
}

// U_Coerce_TypedSliceConversion — the seed for the agent loop sometimes
// arrives as the original []OpenAIResponsesInputMessage produced by
// convert2UpstreamResponsesRequest. inputAsAnySlice + coerceInputItems
// together must accept that shape unchanged.
func TestCoerceInputItems_TypedSliceConversion(t *testing.T) {
	msgs := []httppkg.OpenAIResponsesInputMessage{
		{Role: "system", Content: "you are an agent"},
		{Role: "user", Content: "ping"},
	}
	got, err := coerceInputItems(inputAsAnySlice(msgs))
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, msgs[0], got[0])
	require.Equal(t, msgs[1], got[1])
}

// U_Coerce_MessageTypeDiscriminator — some upstreams (and the memory
// hook) include `"type": "message"` alongside the role. The helper must
// not get confused by the dual discriminator.
func TestCoerceInputItems_MessageTypeDiscriminator(t *testing.T) {
	in := []any{map[string]any{
		"type":    "message",
		"role":    "assistant",
		"content": "previous reply",
	}}
	got, err := coerceInputItems(in)
	require.NoError(t, err)
	require.Len(t, got, 1)
	msg, ok := got[0].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok, "want OpenAIResponsesInputMessage, got %T", got[0])
	require.Equal(t, "assistant", msg.Role)
	require.Equal(t, "previous reply", msg.Content)
}

// U_Coerce_NilAndEmpty — defensive: nil slice and empty slice both
// return nil without error; nil items inside a slice are silently
// skipped (they can appear after some unmarshal paths).
func TestCoerceInputItems_NilAndEmpty(t *testing.T) {
	got, err := coerceInputItems(nil)
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = coerceInputItems([]any{})
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = coerceInputItems([]any{nil, httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "x"}})
	require.NoError(t, err)
	require.Len(t, got, 1)
}

// U_InputAsAnySlice — the seed adapter accepts the three shapes the live
// path can produce: nil, the typed []OpenAIResponsesInputMessage, and the
// already-flat []any. Anything else returns nil so the loop seeds from
// scratch rather than panicking.
func TestInputAsAnySlice(t *testing.T) {
	require.Nil(t, inputAsAnySlice(nil))

	typed := []httppkg.OpenAIResponsesInputMessage{{Role: "user", Content: "a"}}
	got := inputAsAnySlice(typed)
	require.Len(t, got, 1)
	_, ok := got[0].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok)

	flat := []any{map[string]any{"role": "user", "content": "b"}}
	require.Equal(t, flat, inputAsAnySlice(flat))

	// Unrecognised shape -> nil (we never panic on shape drift).
	require.Nil(t, inputAsAnySlice("not a slice"))
}
