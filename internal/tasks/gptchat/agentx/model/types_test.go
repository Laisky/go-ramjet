package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStreamKind_String guards the diagnostic stringer used by golden files.
// Renaming or reordering kinds without updating the strings would silently
// invalidate captured streams.
func TestStreamKind_String(t *testing.T) {
	cases := map[StreamKind]string{
		ChunkText:      "text",
		ChunkReasoning: "reasoning",
		ChunkFunction:  "function",
		ChunkUsage:     "usage",
		ChunkDone:      "done",
		ChunkError:     "error",
		StreamKind(99): "unknown",
	}
	for k, want := range cases {
		require.Equal(t, want, k.String(), "kind %d", k)
	}
}

// TestStreamChunk_ZeroValuesNotAmbiguous documents the invariant that empty
// fields mean "not present" (not "present-but-empty"). The downstream loop
// relies on this to skip empty deltas without a separate "is-set" flag.
func TestStreamChunk_ZeroValuesNotAmbiguous(t *testing.T) {
	zero := StreamChunk{}
	require.Equal(t, ChunkText, zero.Kind, "zero Kind is ChunkText (iota=0)")
	require.Empty(t, zero.Text)
	require.Nil(t, zero.FunctionCall)
	require.Nil(t, zero.Usage)
	require.NoError(t, zero.Err)
}

// TestFunctionCall_RawArgumentsPreserved ensures the Arguments raw JSON is
// passed through verbatim — the loop validates against schemas; this layer
// must not parse or massage.
func TestFunctionCall_RawArgumentsPreserved(t *testing.T) {
	args := json.RawMessage(`{"query":"hello\nworld","n":3}`)
	fc := FunctionCall{
		CallID:    "call_123",
		Name:      "web_search",
		Arguments: args,
	}
	// Identity, not deep-equal — same backing slice expected.
	require.Equal(t, string(args), string(fc.Arguments))
}

// TestToolDescriptor_SchemaRawJSON verifies the Schema field accepts any
// json.RawMessage shape (object, scalar, null) without a typed-schema
// guard. The adapter forwards verbatim and the upstream validates.
func TestToolDescriptor_SchemaRawJSON(t *testing.T) {
	cases := []json.RawMessage{
		json.RawMessage(`{"type":"object","properties":{}}`),
		json.RawMessage(`null`),
		json.RawMessage(`{"type":"object"}`),
	}
	for _, s := range cases {
		td := ToolDescriptor{Name: "t", Schema: s}
		require.Equal(t, string(s), string(td.Schema))
	}
}

// TestCapabilities_ZeroValuesAreFalse documents the zero-value semantics so
// future implementations don't accidentally rely on positive defaults.
func TestCapabilities_ZeroValuesAreFalse(t *testing.T) {
	var c Capabilities
	require.False(t, c.SupportsParallelToolCalls)
	require.False(t, c.SupportsReasoning)
	require.Zero(t, c.MaxContextTokens)
}
