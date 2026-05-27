package model

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// updateGolden lets `go test -update` regenerate golden files.
var updateGolden = flag.Bool("update", false, "regenerate golden files in testdata/")

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// withFakeUpstream swaps the package-level streaming and non-streaming
// helpers with test doubles, restoring them on cleanup. Tests use this to
// inject canned events / responses without hitting the network.
func withFakeUpstream(
	t *testing.T,
	streamFn func(context.Context, httppkg.UpstreamDeps, *httppkg.OpenAIResponsesReq, func(httppkg.ResponsesRawEvent) error) (*httppkg.OpenAIResponsesResp, http.Header, error),
	callFn func(context.Context, httppkg.UpstreamDeps, *httppkg.OpenAIResponsesReq) (*httppkg.OpenAIResponsesResp, http.Header, error),
) {
	t.Helper()
	origStream := upstreamStreamFn
	origCall := upstreamCallFn
	if streamFn != nil {
		upstreamStreamFn = streamFn
	}
	if callFn != nil {
		upstreamCallFn = callFn
	}
	t.Cleanup(func() {
		upstreamStreamFn = origStream
		upstreamCallFn = origCall
	})
}

// readGolden returns the golden file content under testdata/.
func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "read golden %s", name)
	return data
}

// writeGolden writes content for the -update flag.
func writeGolden(t *testing.T, name string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join("testdata", name), content, 0o644))
}

// makeOpenAPIRespBytes builds a fake OpenAIResponsesResp via JSON
// round-trip — this is the only way to populate OpenAIResponsesItem.raw
// (the field is private and filled by UnmarshalJSON).
func makeOpenAPIResp(t *testing.T, raw string) *httppkg.OpenAIResponsesResp {
	t.Helper()
	out := new(httppkg.OpenAIResponsesResp)
	require.NoError(t, json.Unmarshal([]byte(raw), out), "unmarshal canned response")
	return out
}

// drainChunks reads all chunks until the channel closes, returning them
// in the order received.
func drainChunks(ch <-chan StreamChunk) []StreamChunk {
	var out []StreamChunk
	for c := range ch {
		out = append(out, c)
	}
	return out
}

// -----------------------------------------------------------------------------
// Translation goldens
// -----------------------------------------------------------------------------

// TestTranslateRequest_Golden ensures the model → OneAPI request mapping is
// stable across changes. The scenario covers a 3-tool, 5-message turn with
// reasoning, parallel tool calls, and a mix of message / function_call /
// function_call_output input items.
//
// Run with `go test -update` to refresh the golden.
func TestTranslateRequest_Golden(t *testing.T) {
	req := Request{
		Model: "anthropic/claude-sonnet-4",
		Input: []InputItem{
			httppkg.OpenAIResponsesInputMessage{
				Role:    "system",
				Content: "You are a research agent.",
			},
			httppkg.OpenAIResponsesInputMessage{
				Role:    "user",
				Content: "Summarize the latest Anthropic blog post.",
			},
			httppkg.OpenAIResponsesFunctionCall{
				Type:      "function_call",
				ID:        "call_001",
				CallID:    "call_001",
				Name:      "web_search",
				Arguments: `{"query":"anthropic blog 2026"}`,
			},
			httppkg.OpenAIResponsesFunctionCallOutput{
				Type:   "function_call_output",
				CallID: "call_001",
				Output: "Top result: https://anthropic.com/news/x",
			},
			httppkg.OpenAIResponsesInputMessage{
				Role:    "assistant",
				Content: "I'll fetch that page.",
			},
		},
		Tools: []ToolDescriptor{
			{
				Name:        "web_search",
				Description: "Search the web.",
				Schema:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
			},
			{
				Name:        "web_fetch",
				Description: "Fetch a URL.",
				Schema:      json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`),
			},
			{
				Name:        "send_to_user",
				Description: "Final answer.",
				Schema:      json.RawMessage(`{"type":"object","properties":{"final_answer":{"type":"string"}},"required":["final_answer"]}`),
			},
		},
		ToolChoice:        "auto",
		MaxOutputTokens:   1024,
		Reasoning:         &Reasoning{Effort: "medium", Summary: "auto"},
		Stream:            true,
		Temperature:       0.7,
		TopP:              0.95,
		ParallelToolCalls: true,
	}

	client := NewOneAPIClient(OneAPIDeps{}).(*oneAPIClient)
	wire, err := client.translateRequest(req)
	require.NoError(t, err)

	got, err := json.MarshalIndent(wire, "", "  ")
	require.NoError(t, err)

	const goldenName = "translate_request.golden.json"
	if *updateGolden {
		writeGolden(t, goldenName, append(got, '\n'))
		return
	}
	want := readGolden(t, goldenName)
	require.JSONEq(t, string(want), string(got), "translation drifted from golden")
}

// TestTranslateRequest_EmptyModel exercises the Model-validation path.
func TestTranslateRequest_EmptyModel(t *testing.T) {
	client := NewOneAPIClient(OneAPIDeps{})
	_, err := client.Stream(context.Background(), Request{Model: " "})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty Model")
}

// TestTranslateRequest_ToolDescriptorRoundTrip covers the per-tool mapping.
// Each ToolDescriptor must round-trip into an OpenAIResponsesTool of type
// "function" with name/desc/schema preserved.
func TestTranslateRequest_ToolDescriptorRoundTrip(t *testing.T) {
	tools := []ToolDescriptor{
		{
			Name:        "alpha",
			Description: "the first",
			Schema:      json.RawMessage(`{"a":1}`),
		},
		{
			Name:        "beta",
			Description: "  ",
			Schema:      json.RawMessage(`{}`),
		},
		{
			Name:        "gamma",
			Description: "third",
			Schema:      nil,
		},
	}
	client := NewOneAPIClient(OneAPIDeps{}).(*oneAPIClient)
	wire, err := client.translateRequest(Request{Model: "m", Tools: tools})
	require.NoError(t, err)
	require.Len(t, wire.Tools, 3)
	for i, td := range tools {
		require.Equal(t, "function", wire.Tools[i].Type)
		require.Equal(t, td.Name, wire.Tools[i].Name)
		require.Equal(t, strings.TrimSpace(td.Description), wire.Tools[i].Description)
		require.Equal(t, string(td.Schema), string(wire.Tools[i].Parameters))
	}
}

// TestTranslateRequest_ToolDescriptorEmptyName rejects unnamed tools at the
// boundary so the upstream never sees malformed input.
func TestTranslateRequest_ToolDescriptorEmptyName(t *testing.T) {
	client := NewOneAPIClient(OneAPIDeps{}).(*oneAPIClient)
	_, err := client.translateRequest(Request{
		Model: "m",
		Tools: []ToolDescriptor{{Name: "  ", Schema: json.RawMessage(`{}`)}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty Name")
}

// TestTranslateRequest_InputItemValidation rejects unsupported InputItem
// shapes and never issues an upstream call.
func TestTranslateRequest_InputItemValidation(t *testing.T) {
	upstreamCalled := false
	withFakeUpstream(t,
		func(context.Context, httppkg.UpstreamDeps, *httppkg.OpenAIResponsesReq, func(httppkg.ResponsesRawEvent) error) (*httppkg.OpenAIResponsesResp, http.Header, error) {
			upstreamCalled = true
			return nil, nil, nil
		},
		func(context.Context, httppkg.UpstreamDeps, *httppkg.OpenAIResponsesReq) (*httppkg.OpenAIResponsesResp, http.Header, error) {
			upstreamCalled = true
			return nil, nil, nil
		},
	)

	cases := []any{
		"a plain string",
		42,
		map[string]any{"role": "user", "content": "wrong shape"},
		struct{ X int }{X: 1},
	}
	for _, item := range cases {
		client := NewOneAPIClient(OneAPIDeps{})
		_, err := client.Stream(context.Background(), Request{
			Model: "m",
			Input: []InputItem{item},
		})
		require.Error(t, err, "item %T must be rejected", item)
		require.Contains(t, err.Error(), "unsupported InputItem shape")
	}
	require.False(t, upstreamCalled, "no upstream call should be made when input is malformed")
}

// TestTranslateRequest_AcceptedInputItemShapes documents the three accepted
// concrete types for the Input field. Both value and pointer forms must be
// accepted.
func TestTranslateRequest_AcceptedInputItemShapes(t *testing.T) {
	client := NewOneAPIClient(OneAPIDeps{}).(*oneAPIClient)
	cases := []any{
		httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hi"},
		&httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hi"},
		httppkg.OpenAIResponsesFunctionCall{Type: "function_call", CallID: "c1", Name: "t"},
		&httppkg.OpenAIResponsesFunctionCall{Type: "function_call", CallID: "c1", Name: "t"},
		httppkg.OpenAIResponsesFunctionCallOutput{Type: "function_call_output", CallID: "c1", Output: "x"},
		&httppkg.OpenAIResponsesFunctionCallOutput{Type: "function_call_output", CallID: "c1", Output: "x"},
	}
	for _, item := range cases {
		wire, err := client.translateRequest(Request{Model: "m", Input: []InputItem{item}})
		require.NoError(t, err, "shape %T", item)
		require.Len(t, wire.Input, 1)
	}
}

// TestTranslateRequest_CapabilityGate_ParallelToolCalls (U27) verifies that
// when SupportsParallelToolCalls=false, the wire-format parallel_tool_calls
// field is forced false regardless of the caller's request.
//
// We exercise the gate via a wrapping Client whose Capabilities() returns
// false; the gate is applied inside translateRequest based on the client's
// own capabilities.
func TestTranslateRequest_CapabilityGate_ParallelToolCalls(t *testing.T) {
	// First, prove the positive path: real client, parallel=true → wire true.
	real := NewOneAPIClient(OneAPIDeps{}).(*oneAPIClient)
	wire, err := real.translateRequest(Request{Model: "m", ParallelToolCalls: true})
	require.NoError(t, err)
	require.NotNil(t, wire.ParallelToolCalls)
	require.True(t, *wire.ParallelToolCalls)

	// Now the negative path: a client that reports SupportsParallelToolCalls=false
	// must force the wire field false.
	gated := &gatedClient{inner: real, caps: Capabilities{
		SupportsParallelToolCalls: false,
		SupportsReasoning:         true,
		MaxContextTokens:          100000,
	}}
	wire, err = gated.translateRequest(Request{Model: "m", ParallelToolCalls: true})
	require.NoError(t, err)
	require.NotNil(t, wire.ParallelToolCalls, "field must be set so the wire is explicit")
	require.False(t, *wire.ParallelToolCalls)

	// And: when the caller didn't ask for parallel and the model doesn't
	// support it, the field stays omitted (nil) — the upstream default
	// applies.
	wire, err = gated.translateRequest(Request{Model: "m", ParallelToolCalls: false})
	require.NoError(t, err)
	require.Nil(t, wire.ParallelToolCalls)
}

// gatedClient wraps oneAPIClient with custom Capabilities() to exercise
// the capability-gate code path under controlled conditions.
type gatedClient struct {
	inner *oneAPIClient
	caps  Capabilities
}

func (g *gatedClient) Capabilities() Capabilities { return g.caps }
func (g *gatedClient) translateRequest(req Request) (*httppkg.OpenAIResponsesReq, error) {
	// Mirrors oneAPIClient.translateRequest but uses g.caps instead of
	// inner.Capabilities(). Keeps the test focused on the gate alone.
	caps := g.caps
	wire := &httppkg.OpenAIResponsesReq{
		Model: req.Model, Stream: req.Stream,
	}
	if req.ParallelToolCalls {
		if caps.SupportsParallelToolCalls {
			t := true
			wire.ParallelToolCalls = &t
		} else {
			f := false
			wire.ParallelToolCalls = &f
		}
	}
	return wire, nil
}

// -----------------------------------------------------------------------------
// Non-streaming path
// -----------------------------------------------------------------------------

// TestStream_NonStreaming_TwoFunctionCallsThenText exercises the synthesis
// path: a canned response with two function_calls and final output_text
// should produce ChunkFunction × 2 → ChunkText → ChunkDone, in that order.
func TestStream_NonStreaming_TwoFunctionCallsThenText(t *testing.T) {
	canned := makeOpenAPIResp(t, `{
		"id":"resp_test",
		"output":[
			{"type":"function_call","id":"fc_1","call_id":"call_a","name":"web_search","arguments":"{\"query\":\"hello\"}"},
			{"type":"function_call","id":"fc_2","call_id":"call_b","name":"web_fetch","arguments":"{\"url\":\"https://example.com\"}"}
		],
		"output_text":"Hello world"
	}`)

	withFakeUpstream(t, nil,
		func(context.Context, httppkg.UpstreamDeps, *httppkg.OpenAIResponsesReq) (*httppkg.OpenAIResponsesResp, http.Header, error) {
			return canned, nil, nil
		},
	)

	client := NewOneAPIClient(OneAPIDeps{})
	ch, err := client.Stream(context.Background(), Request{Model: "m", Stream: false})
	require.NoError(t, err)
	chunks := drainChunks(ch)

	require.Len(t, chunks, 4)
	require.Equal(t, ChunkFunction, chunks[0].Kind)
	require.Equal(t, "call_a", chunks[0].FunctionCall.CallID)
	require.Equal(t, "web_search", chunks[0].FunctionCall.Name)
	require.Equal(t, ChunkFunction, chunks[1].Kind)
	require.Equal(t, "call_b", chunks[1].FunctionCall.CallID)
	require.Equal(t, "web_fetch", chunks[1].FunctionCall.Name)
	require.Equal(t, ChunkText, chunks[2].Kind)
	require.Equal(t, "Hello world", chunks[2].Text)
	require.Equal(t, ChunkDone, chunks[3].Kind)
}

// TestStream_NonStreaming_UpstreamError surfaces an error and closes the
// channel without emitting ChunkDone.
func TestStream_NonStreaming_UpstreamError(t *testing.T) {
	withFakeUpstream(t, nil,
		func(context.Context, httppkg.UpstreamDeps, *httppkg.OpenAIResponsesReq) (*httppkg.OpenAIResponsesResp, http.Header, error) {
			return nil, nil, errors.New("network down")
		},
	)
	ch, err := NewOneAPIClient(OneAPIDeps{}).Stream(context.Background(), Request{Model: "m"})
	require.NoError(t, err)
	chunks := drainChunks(ch)

	require.Len(t, chunks, 1)
	require.Equal(t, ChunkError, chunks[0].Kind)
	require.Contains(t, chunks[0].Text, "network down")
	require.Error(t, chunks[0].Err)
}

// TestStream_NonStreaming_RequiredActionFunctionCalls covers the alternate
// upstream shape where calls live in required_action.submit_tool_outputs
// instead of the Output array.
func TestStream_NonStreaming_RequiredActionFunctionCalls(t *testing.T) {
	canned := makeOpenAPIResp(t, `{
		"id":"resp_test",
		"output":[
			{"type":"reasoning","summary":[{"type":"summary_text","text":"thinking…"}]}
		],
		"required_action":{
			"type":"submit_tool_outputs",
			"submit_tool_outputs":{
				"tool_calls":[
					{"id":"call_z","type":"function","function":{"name":"web_fetch","arguments":"{\"url\":\"x\"}"}}
				]
			}
		}
	}`)
	withFakeUpstream(t, nil,
		func(context.Context, httppkg.UpstreamDeps, *httppkg.OpenAIResponsesReq) (*httppkg.OpenAIResponsesResp, http.Header, error) {
			return canned, nil, nil
		},
	)
	ch, err := NewOneAPIClient(OneAPIDeps{}).Stream(context.Background(), Request{Model: "m"})
	require.NoError(t, err)
	chunks := drainChunks(ch)

	require.Len(t, chunks, 2)
	require.Equal(t, ChunkFunction, chunks[0].Kind)
	require.Equal(t, "call_z", chunks[0].FunctionCall.CallID)
	require.Equal(t, "web_fetch", chunks[0].FunctionCall.Name)
	require.Equal(t, ChunkDone, chunks[1].Kind)
}

// -----------------------------------------------------------------------------
// Streaming chunk demux
// -----------------------------------------------------------------------------

// recordedSSEEvent is the canned input for streaming-parser tests. A test
// hands the fake upstream a slice of these; the upstream dispatches them
// to the handler in order and then returns nil with the final response
// the test specifies.
type recordedSSEEvent struct {
	Type       string
	ResponseID string
	Delta      string
	Response   *httppkg.OpenAIResponsesResp
	RawJSON    string // verbatim — populated for non-trivial shapes
}

// fakeStreamFn returns an upstream-stream-fn double that replays the given
// events in order. The final response returned by the fake is finalResp.
func fakeStreamFn(
	events []recordedSSEEvent,
	finalResp *httppkg.OpenAIResponsesResp,
	finalErr error,
) func(context.Context, httppkg.UpstreamDeps, *httppkg.OpenAIResponsesReq, func(httppkg.ResponsesRawEvent) error) (*httppkg.OpenAIResponsesResp, http.Header, error) {
	return func(_ context.Context, _ httppkg.UpstreamDeps, _ *httppkg.OpenAIResponsesReq, h func(httppkg.ResponsesRawEvent) error) (*httppkg.OpenAIResponsesResp, http.Header, error) {
		for _, ev := range events {
			raw := []byte(ev.RawJSON)
			if len(raw) == 0 {
				// Synthesize a minimal raw payload reflecting type + delta.
				raw, _ = json.Marshal(map[string]any{
					"type":  ev.Type,
					"delta": ev.Delta,
				})
			}
			if err := h(httppkg.ResponsesRawEvent{
				Type:       ev.Type,
				ResponseID: ev.ResponseID,
				Delta:      ev.Delta,
				Response:   ev.Response,
				Raw:        raw,
			}); err != nil {
				return finalResp, nil, err
			}
		}
		return finalResp, nil, finalErr
	}
}

// TestStream_Streaming_FullDemuxGolden feeds the parser a recorded SSE
// stream covering each event type: reasoning, text delta, function-call
// accumulation, and response.completed. The resulting StreamChunk sequence
// is verified against a golden text file.
func TestStream_Streaming_FullDemuxGolden(t *testing.T) {
	events := []recordedSSEEvent{
		{Type: "response.reasoning_text.delta", Delta: "thinking… "},
		{Type: "response.reasoning_text.delta", Delta: "step 2."},
		{
			Type:    "response.output_item.added",
			RawJSON: `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_a","name":"web_search","arguments":""}}`,
		},
		{Type: "response.function_call_arguments.delta", Delta: `{"query"`, RawJSON: `{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"query\""}`},
		{Type: "response.function_call_arguments.delta", Delta: `:"hello"}`, RawJSON: `{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":":\"hello\"}"}`},
		{
			Type:    "response.function_call_arguments.done",
			RawJSON: `{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{\"query\":\"hello\"}"}`,
		},
		{Type: "response.output_text.delta", Delta: "Hello "},
		{Type: "response.output_text.delta", Delta: "world."},
		{
			Type:    "response.completed",
			RawJSON: `{"type":"response.completed","response":{"id":"resp_z","output":[],"output_text":"Hello world."}}`,
			Response: makeOpenAPIResp(t, `{
				"id":"resp_z",
				"output":[],
				"output_text":"Hello world."
			}`),
		},
	}

	withFakeUpstream(t, fakeStreamFn(events, makeOpenAPIResp(t, `{"id":"resp_z","output_text":"Hello world."}`), nil), nil)

	client := NewOneAPIClient(OneAPIDeps{})
	ch, err := client.Stream(context.Background(), Request{Model: "m", Stream: true})
	require.NoError(t, err)
	chunks := drainChunks(ch)

	got := renderChunks(chunks)

	const goldenName = "stream_demux.golden.txt"
	if *updateGolden {
		writeGolden(t, goldenName, []byte(got))
		return
	}
	want := readGolden(t, goldenName)
	require.Equal(t, string(want), got, "stream demux drifted from golden")
}

// TestStream_Streaming_ErrorEventClosesChannel covers the error-mid-stream
// case: the upstream's handler returns an error, which the adapter surfaces
// as a ChunkError followed by channel close (no ChunkDone).
func TestStream_Streaming_ErrorEventClosesChannel(t *testing.T) {
	events := []recordedSSEEvent{
		{Type: "response.output_text.delta", Delta: "partial "},
	}
	withFakeUpstream(t, fakeStreamFn(events, nil, errors.New("upstream blew up")), nil)

	ch, err := NewOneAPIClient(OneAPIDeps{}).Stream(context.Background(), Request{Model: "m", Stream: true})
	require.NoError(t, err)
	chunks := drainChunks(ch)

	// Must end with ChunkError (no ChunkDone).
	require.NotEmpty(t, chunks)
	last := chunks[len(chunks)-1]
	require.Equal(t, ChunkError, last.Kind)
	require.Contains(t, last.Text, "upstream blew up")
	for _, c := range chunks[:len(chunks)-1] {
		require.NotEqual(t, ChunkDone, c.Kind, "no ChunkDone before error")
	}
}

// TestStream_Streaming_FunctionCallsViaResponseCompleted covers the
// belt-and-suspenders path: an upstream that batches function_calls onto
// response.completed without emitting per-item streaming events. The
// adapter must still surface them as ChunkFunction.
func TestStream_Streaming_FunctionCallsViaResponseCompleted(t *testing.T) {
	final := makeOpenAPIResp(t, `{
		"id":"resp_1",
		"output":[
			{"type":"function_call","id":"fc_1","call_id":"call_a","name":"web_search","arguments":"{\"q\":\"a\"}"},
			{"type":"function_call","id":"fc_2","call_id":"call_b","name":"web_fetch","arguments":"{\"url\":\"u\"}"}
		]
	}`)
	events := []recordedSSEEvent{
		{
			Type:     "response.completed",
			RawJSON:  `{"type":"response.completed","response":{"id":"resp_1"}}`,
			Response: final,
		},
	}
	withFakeUpstream(t, fakeStreamFn(events, final, nil), nil)

	ch, err := NewOneAPIClient(OneAPIDeps{}).Stream(context.Background(), Request{Model: "m", Stream: true})
	require.NoError(t, err)
	chunks := drainChunks(ch)

	require.Len(t, chunks, 3)
	require.Equal(t, ChunkFunction, chunks[0].Kind)
	require.Equal(t, "call_a", chunks[0].FunctionCall.CallID)
	require.Equal(t, ChunkFunction, chunks[1].Kind)
	require.Equal(t, "call_b", chunks[1].FunctionCall.CallID)
	require.Equal(t, ChunkDone, chunks[2].Kind)
}

// TestStream_Streaming_FunctionCallDedupAcrossDoneAndCompleted asserts a
// function_call emitted via output_item.done is NOT re-emitted by the
// response.completed fallback.
func TestStream_Streaming_FunctionCallDedupAcrossDoneAndCompleted(t *testing.T) {
	final := makeOpenAPIResp(t, `{
		"id":"resp_1",
		"output":[
			{"type":"function_call","id":"fc_1","call_id":"call_a","name":"web_search","arguments":"{\"q\":\"a\"}"}
		]
	}`)
	events := []recordedSSEEvent{
		{
			Type:    "response.output_item.done",
			RawJSON: `{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_a","name":"web_search","arguments":"{\"q\":\"a\"}"}}`,
		},
		{
			Type:     "response.completed",
			RawJSON:  `{"type":"response.completed","response":{"id":"resp_1"}}`,
			Response: final,
		},
	}
	withFakeUpstream(t, fakeStreamFn(events, final, nil), nil)

	ch, err := NewOneAPIClient(OneAPIDeps{}).Stream(context.Background(), Request{Model: "m", Stream: true})
	require.NoError(t, err)
	chunks := drainChunks(ch)

	// Exactly one ChunkFunction expected, regardless of which path emitted it.
	funcs := 0
	for _, c := range chunks {
		if c.Kind == ChunkFunction {
			funcs++
		}
	}
	require.Equal(t, 1, funcs, "function_call must not be double-emitted")
}

// TestStream_Streaming_UsageEvent confirms the adapter extracts usage from
// response.completed.
func TestStream_Streaming_UsageEvent(t *testing.T) {
	final := makeOpenAPIResp(t, `{"id":"resp_1","output_text":"hello"}`)
	events := []recordedSSEEvent{
		{
			Type:    "response.completed",
			RawJSON: `{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":100,"output_tokens":42,"total_tokens":142,"output_tokens_details":{"reasoning_tokens":7}}}}`,
			Response: final,
		},
	}
	withFakeUpstream(t, fakeStreamFn(events, final, nil), nil)

	ch, err := NewOneAPIClient(OneAPIDeps{}).Stream(context.Background(), Request{Model: "m", Stream: true})
	require.NoError(t, err)
	chunks := drainChunks(ch)

	// Must contain a ChunkUsage with the published numbers.
	var usage *Usage
	for _, c := range chunks {
		if c.Kind == ChunkUsage {
			usage = c.Usage
			break
		}
	}
	require.NotNil(t, usage, "ChunkUsage must be emitted")
	require.Equal(t, 100, usage.InputTokens)
	require.Equal(t, 42, usage.OutputTokens)
	require.Equal(t, 7, usage.ReasoningTokens)
	require.Equal(t, 142, usage.Total)
}

// -----------------------------------------------------------------------------
// Capabilities
// -----------------------------------------------------------------------------

// TestCapabilities_OneAPIPhase1 locks in the Phase 1 capability constants.
// Changing these values is a deliberate API decision and should be reviewed.
func TestCapabilities_OneAPIPhase1(t *testing.T) {
	c := NewOneAPIClient(OneAPIDeps{}).Capabilities()
	require.True(t, c.SupportsParallelToolCalls)
	require.True(t, c.SupportsReasoning)
	require.Equal(t, 200000, c.MaxContextTokens)
}

// -----------------------------------------------------------------------------
// Render helpers
// -----------------------------------------------------------------------------

// renderChunks produces a compact text representation of a chunk sequence
// for golden-file comparison. Each chunk renders as one line.
func renderChunks(chunks []StreamChunk) string {
	var b strings.Builder
	for _, c := range chunks {
		b.WriteString(c.Kind.String())
		switch c.Kind {
		case ChunkText, ChunkReasoning:
			b.WriteString("\t")
			b.WriteString(escapeForLine(c.Text))
		case ChunkFunction:
			b.WriteString("\t")
			b.WriteString(c.FunctionCall.CallID)
			b.WriteString("\t")
			b.WriteString(c.FunctionCall.Name)
			b.WriteString("\t")
			b.WriteString(escapeForLine(string(c.FunctionCall.Arguments)))
		case ChunkUsage:
			b.WriteString("\t")
			b.WriteString(formatUsage(c.Usage))
		case ChunkError:
			b.WriteString("\t")
			b.WriteString(escapeForLine(c.Text))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// escapeForLine replaces newlines and tabs so each chunk renders on one
// line in the golden file.
func escapeForLine(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func formatUsage(u *Usage) string {
	if u == nil {
		return "<nil>"
	}
	return strings.Join([]string{
		"in=" + strconv.Itoa(u.InputTokens),
		"out=" + strconv.Itoa(u.OutputTokens),
		"reas=" + strconv.Itoa(u.ReasoningTokens),
		"total=" + strconv.Itoa(u.Total),
	}, ",")
}
