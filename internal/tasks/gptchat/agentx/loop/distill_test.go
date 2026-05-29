package loop

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/distiller"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// stubDistiller is a controllable Distiller for hook tests. Calls counts
// invocations; reply / replyErr script the next return.
type stubDistiller struct {
	calls    atomic.Int64
	mu       sync.Mutex
	reply    distiller.Result
	replyErr error
	lastReq  distiller.Request
}

func (s *stubDistiller) Distill(_ context.Context, req distiller.Request) (distiller.Result, error) {
	s.calls.Add(1)
	s.mu.Lock()
	s.lastReq = req
	r, err := s.reply, s.replyErr
	s.mu.Unlock()
	return r, err
}

func TestDistillHook_PassThroughUnderThreshold(t *testing.T) {
	t.Parallel()
	d := &stubDistiller{reply: distiller.Result{Content: "should-not-appear"}}
	stash := session.NewRawStash()
	h := NewDistillHook(d, 200, stash, "user goal")

	ev := hook.ToolCallEvent{
		ToolName: "web_search",
		CallID:   "call_short",
		Result:   &tool.Result{Content: "tiny output"},
	}
	out, err := h(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, "tiny output", out.Result.Content)
	require.Equal(t, int64(0), d.calls.Load(), "distill should not be called below threshold")
	require.Equal(t, 0, stash.Len(), "no stash on pass-through")
}

func TestDistillHook_DistillsAboveThreshold(t *testing.T) {
	t.Parallel()
	d := &stubDistiller{reply: distiller.Result{Content: "Ottawa: 10°C, sunny."}}
	stash := session.NewRawStash()
	h := NewDistillHook(d, 100, stash, "Ottawa weather?")

	raw := strings.Repeat("noise ", 500)
	ev := hook.ToolCallEvent{
		ToolName: "web_fetch",
		CallID:   "call_big",
		Args:     stdjson.RawMessage(`{"url":"https://weather"}`),
		Result:   &tool.Result{Content: raw},
	}
	out, err := h(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, int64(1), d.calls.Load())
	require.Contains(t, out.Result.Content, "Ottawa: 10°C, sunny.")
	require.Contains(t, out.Result.Content, "observation distilled from")
	require.Contains(t, out.Result.Content, "call_id=call_big")
	require.NotContains(t, out.Result.Content, "noise noise noise",
		"distilled content must not retain raw verbatim")

	// Raw bytes must be in the stash for post-hoc retrieval.
	got, ok := stash.Get("call_big")
	require.True(t, ok)
	require.Equal(t, raw, got)

	// Salience anchors were forwarded.
	require.Equal(t, "Ottawa weather?", d.lastReq.UserPrompt)
	require.Equal(t, "web_fetch", d.lastReq.ToolName)
	require.Equal(t, "call_big", d.lastReq.CallID)
}

func TestDistillHook_SkipsErrors(t *testing.T) {
	t.Parallel()
	d := &stubDistiller{reply: distiller.Result{Content: "rewrite"}}
	stash := session.NewRawStash()
	h := NewDistillHook(d, 100, stash, "u")

	raw := strings.Repeat("error context ", 200)
	ev := hook.ToolCallEvent{
		ToolName: "web_fetch",
		CallID:   "call_err",
		Result:   &tool.Result{Content: raw, IsError: true},
	}
	out, err := h(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, raw, out.Result.Content, "error content must pass through verbatim")
	require.Equal(t, int64(0), d.calls.Load())
	require.Equal(t, 0, stash.Len())
}

func TestDistillHook_SkipsAlreadyWrapped(t *testing.T) {
	t.Parallel()
	d := &stubDistiller{reply: distiller.Result{Content: "rewrite"}}
	stash := session.NewRawStash()
	h := NewDistillHook(d, 50, stash, "u")

	wrapped := `<tool_result tool="x" trust="untrusted">` + strings.Repeat("body ", 500) + `</tool_result>`
	ev := hook.ToolCallEvent{
		ToolName: "x",
		CallID:   "c",
		Result:   &tool.Result{Content: wrapped},
	}
	out, err := h(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, wrapped, out.Result.Content)
	require.Equal(t, int64(0), d.calls.Load())
}

func TestDistillHook_NilResult(t *testing.T) {
	t.Parallel()
	d := &stubDistiller{}
	h := NewDistillHook(d, 100, session.NewRawStash(), "u")

	out, err := h(context.Background(), hook.ToolCallEvent{ToolName: "x", CallID: "c"})
	require.NoError(t, err)
	require.Nil(t, out.Result)
	require.Equal(t, int64(0), d.calls.Load())
}

func TestDistillHook_NilDistillerPassesThrough(t *testing.T) {
	t.Parallel()
	stash := session.NewRawStash()
	h := NewDistillHook(nil, 100, stash, "u")

	raw := strings.Repeat("x", 5000)
	ev := hook.ToolCallEvent{
		ToolName: "t",
		CallID:   "c",
		Result:   &tool.Result{Content: raw},
	}
	out, err := h(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, raw, out.Result.Content)
	require.Equal(t, 0, stash.Len())
}

func TestDistillHook_DistillFailureKeepsRaw(t *testing.T) {
	t.Parallel()
	// reply has empty Content AND replyErr != nil — simulates a Distill
	// implementation that genuinely produced nothing usable.
	d := &stubDistiller{
		reply:    distiller.Result{},
		replyErr: errors.New("upstream down"),
	}
	stash := session.NewRawStash()
	h := NewDistillHook(d, 100, stash, "u")

	raw := strings.Repeat("payload ", 200)
	ev := hook.ToolCallEvent{
		ToolName: "web_fetch",
		CallID:   "call_x",
		Result:   &tool.Result{Content: raw},
	}
	out, err := h(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, raw, out.Result.Content,
		"distiller returning empty must leave raw untouched in the event")
	// Raw still stashed though — so a later read tool can recover it.
	got, ok := stash.Get("call_x")
	require.True(t, ok)
	require.Equal(t, raw, got)
}

func TestDistillHook_DistillReturnsFallback(t *testing.T) {
	t.Parallel()
	// LLM failed but the distiller produced a truncated fallback; the
	// hook should still write the fallback into the event.
	d := &stubDistiller{
		reply:    distiller.Result{Content: "[summariser failed: timeout; raw truncated]\nfirst bytes...last bytes", Truncated: true},
		replyErr: nil,
	}
	stash := session.NewRawStash()
	h := NewDistillHook(d, 100, stash, "u")

	raw := strings.Repeat("data ", 300)
	ev := hook.ToolCallEvent{
		ToolName: "web_fetch",
		CallID:   "call_t",
		Result:   &tool.Result{Content: raw},
	}
	out, err := h(context.Background(), ev)
	require.NoError(t, err)
	require.Contains(t, out.Result.Content, "summariser failed")
	require.Contains(t, out.Result.Content, "observation distilled from")
	gotRaw, ok := stash.Get("call_t")
	require.True(t, ok)
	require.Equal(t, raw, gotRaw)
}

func TestDistillHook_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	d := &stubDistiller{reply: distiller.Result{Content: "summary"}}
	stash := session.NewRawStash()
	h := NewDistillHook(d, 50, stash, "u")

	raw := strings.Repeat("x", 1000)
	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id := "call_" + string(rune('a'+i))
			_, err := h(context.Background(), hook.ToolCallEvent{
				ToolName: "t",
				CallID:   id,
				Result:   &tool.Result{Content: raw},
			})
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()
	require.Equal(t, int64(n), d.calls.Load())
	require.Equal(t, n, stash.Len())
}

// recordingModelClient wraps fakeModelClient and captures each Stream
// request so the integration test can assert that the next-round input
// carries the distilled observation, not the raw bytes.
type recordingModelClient struct {
	inner *fakeModelClient
	mu    sync.Mutex
	reqs  []model.Request
}

func (r *recordingModelClient) Stream(ctx context.Context, req model.Request) (<-chan model.StreamChunk, error) {
	r.mu.Lock()
	r.reqs = append(r.reqs, model.Request{Input: append([]model.InputItem{}, req.Input...)})
	r.mu.Unlock()
	return r.inner.Stream(ctx, req)
}

func (r *recordingModelClient) Capabilities() model.Capabilities { return r.inner.Capabilities() }

func (r *recordingModelClient) requestAt(i int) model.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reqs[i]
}

// TestDistillHook_EndToEnd_ShrinksNextRoundInput drives a full
// loop.Run with a recording model client + an oversize tool to confirm
// that the distill hook fires inside the round, replaces the
// function_call_output in the transcript, and the next round's input
// contains the dense observation rather than the raw bytes.
func TestDistillHook_EndToEnd_ShrinksNextRoundInput(t *testing.T) {
	t.Parallel()

	bigTool := newFakeTool("big_tool", 0, strings.Repeat("verbose body text. ", 600))
	scripts := [][]model.StreamChunk{
		scriptedRound{
			functionCalls: []model.FunctionCall{{
				CallID:    "call_big_1",
				Name:      "big_tool",
				Arguments: rawArgs(t, map[string]any{}),
			}},
		}.chunks(),
		sendToUserBatch(t, "done"),
	}

	h := newHarness(t, scripts, []tool.Tool{bigTool})
	recorder := &recordingModelClient{inner: h.modelClient}

	d := &stubDistiller{reply: distiller.Result{Content: "DENSE-OBSERVATION"}}
	stash := session.NewRawStash()
	h.bus.OnAfterToolCall(NewDistillHook(d, 100, stash, "what is the body?"))

	require.NoError(t, h.sess.Submit(context.Background(),
		session.OpUserTurn{Text: "what is the body?"}))
	err := Run(context.Background(), h.sess, RunDeps{
		Bus:        h.bus,
		Registry:   h.registry,
		Model:      recorder,
		Caps:       h.caps,
		UserPrompt: "what is the body?",
		SessionID:  "test-session",
		ModelID:    "test-model",
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), d.calls.Load(), "distill hook must have fired exactly once")
	require.GreaterOrEqual(t, len(recorder.reqs), 2,
		"loop must have made at least two model calls")

	round1Input := recorder.requestAt(1).Input
	var foundOutput bool
	for _, item := range round1Input {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t != "function_call_output" {
			continue
		}
		out, _ := m["output"].(string)
		require.Contains(t, out, "DENSE-OBSERVATION",
			"function_call_output must carry the distilled observation")
		require.Contains(t, out, "observation distilled from",
			"function_call_output must carry the distillation footer")
		require.NotContains(t, out, "verbose body text",
			"raw bytes must not appear in the next-round input")
		foundOutput = true
	}
	require.True(t, foundOutput, "no function_call_output found in round 1 input")

	// Raw bytes recoverable from the stash by call_id.
	raw, ok := stash.Get("call_big_1")
	require.True(t, ok)
	require.Contains(t, raw, "verbose body text",
		"raw bytes must be retained in the stash for post-hoc retrieval")
}

// TestDistillHook_ComposesWithWrapHook verifies the ordering invariant
// in the godoc: when DistillHook is registered BEFORE WrapHook the wrap
// envelope encloses the *distilled* string (not the raw bytes).
func TestDistillHook_ComposesWithWrapHook(t *testing.T) {
	t.Parallel()
	d := &stubDistiller{reply: distiller.Result{Content: "DENSE-SUMMARY"}}
	stash := session.NewRawStash()
	distill := NewDistillHook(d, 50, stash, "u")
	wrap := NewWrapHook()

	raw := strings.Repeat("verbose ", 200)
	ev := hook.ToolCallEvent{
		ToolName: "web_fetch",
		CallID:   "c",
		Result:   &tool.Result{Content: raw},
	}
	ev, err := distill(context.Background(), ev)
	require.NoError(t, err)
	ev, err = wrap(context.Background(), ev)
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(ev.Result.Content, `<tool_result tool="web_fetch" trust="untrusted">`))
	require.True(t, strings.HasSuffix(ev.Result.Content, `</tool_result>`))
	require.Contains(t, ev.Result.Content, "DENSE-SUMMARY")
	require.NotContains(t, ev.Result.Content, "verbose verbose verbose",
		"wrap envelope must enclose distilled content, not raw bytes")
}
