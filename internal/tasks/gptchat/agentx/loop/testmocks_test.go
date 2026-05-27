package loop

import (
	"context"
	stdjson "encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// fakeModelClient drives the loop with a scripted sequence of chunk batches:
// one batch per Stream() call. Each batch is closed automatically once the
// loop drains it.
type fakeModelClient struct {
	mu        sync.Mutex
	scripts   [][]model.StreamChunk
	caps      model.Capabilities
	callCount int
	// streamErr, when set on a given call index, makes the next Stream()
	// invocation return that error immediately instead of consuming a
	// scripted batch.
	streamErrAt map[int]error
}

func newFakeModelClient(scripts [][]model.StreamChunk) *fakeModelClient {
	return &fakeModelClient{
		scripts: scripts,
		caps: model.Capabilities{
			SupportsParallelToolCalls: true,
		},
		streamErrAt: map[int]error{},
	}
}

func (f *fakeModelClient) Stream(ctx context.Context, _ model.Request) (<-chan model.StreamChunk, error) {
	f.mu.Lock()
	idx := f.callCount
	f.callCount++
	if err, ok := f.streamErrAt[idx]; ok {
		f.mu.Unlock()
		return nil, err
	}
	if idx >= len(f.scripts) {
		f.mu.Unlock()
		// Out-of-script: emit a single text chunk + Done. Tests should
		// generally script enough rounds to avoid hitting this.
		ch := make(chan model.StreamChunk, 2)
		ch <- model.StreamChunk{Kind: model.ChunkText, Text: ""}
		ch <- model.StreamChunk{Kind: model.ChunkDone}
		close(ch)
		return ch, nil
	}
	batch := f.scripts[idx]
	f.mu.Unlock()

	ch := make(chan model.StreamChunk, len(batch)+1)
	go func() {
		defer close(ch)
		for _, c := range batch {
			select {
			case <-ctx.Done():
				return
			case ch <- c:
			}
		}
	}()
	return ch, nil
}

func (f *fakeModelClient) Capabilities() model.Capabilities { return f.caps }

// callIndex returns how many times Stream has been called so far. Used by
// tests that want to assert post-loop call counts.
func (f *fakeModelClient) callIndex() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}

// fakeTool is a configurable tool used by the executor tests. ExecuteFn, if
// set, runs instead of the canned sleep+result path. start/end timestamps
// are captured for concurrency assertions.
type fakeTool struct {
	name string
	desc string
	// sleep simulates work for the canned path.
	sleep time.Duration
	// output is the canned content the tool returns.
	output string
	// isError sets Result.IsError.
	isError bool
	// executeFn overrides the canned path when set.
	executeFn func(ctx context.Context, call tool.Call) (tool.Result, error)
	// concurrencyTracker, when non-nil, is decremented on tool entry/exit.
	concurrencyTracker *concurrencyTracker

	mu     sync.Mutex
	starts []time.Time
	ends   []time.Time
	calls  int
}

func newFakeTool(name string, sleep time.Duration, output string) *fakeTool {
	return &fakeTool{name: name, desc: "fake " + name, sleep: sleep, output: output}
}

func (f *fakeTool) Name() string                  { return f.name }
func (f *fakeTool) Description() string           { return f.desc }
func (f *fakeTool) Schema() stdjson.RawMessage    { return stdjson.RawMessage(`{"type":"object"}`) }

func (f *fakeTool) Execute(ctx context.Context, call tool.Call, _ session.EventSink) (tool.Result, error) {
	f.mu.Lock()
	f.starts = append(f.starts, time.Now())
	f.calls++
	f.mu.Unlock()
	if f.concurrencyTracker != nil {
		f.concurrencyTracker.enter()
	}
	defer func() {
		f.mu.Lock()
		f.ends = append(f.ends, time.Now())
		f.mu.Unlock()
		if f.concurrencyTracker != nil {
			f.concurrencyTracker.leave()
		}
	}()

	if f.executeFn != nil {
		return f.executeFn(ctx, call)
	}

	if f.sleep > 0 {
		select {
		case <-ctx.Done():
			return tool.Result{}, ctx.Err()
		case <-time.After(f.sleep):
		}
	}
	return tool.Result{Content: f.output, IsError: f.isError}, nil
}

func (f *fakeTool) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeTool) startTimes() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]time.Time, len(f.starts))
	copy(out, f.starts)
	return out
}

// concurrencyTracker counts the live in-flight goroutines for tools that
// share an instance. Peak value is the high-water mark.
type concurrencyTracker struct {
	inFlight atomic.Int64
	peak     atomic.Int64
}

func newConcurrencyTracker() *concurrencyTracker { return &concurrencyTracker{} }

func (t *concurrencyTracker) enter() {
	v := t.inFlight.Add(1)
	for {
		p := t.peak.Load()
		if v <= p || t.peak.CompareAndSwap(p, v) {
			return
		}
	}
}

func (t *concurrencyTracker) leave() { t.inFlight.Add(-1) }

func (t *concurrencyTracker) peakValue() int64 { return t.peak.Load() }

// sendToUserTool is the synthetic exit tool used in tests. In production it
// will be implemented in agentx/tools/send_to_user.go (Phase 1B-2), but for
// loop tests we register a stub so the registry contains the well-known
// name.
type sendToUserTool struct{}

func (sendToUserTool) Name() string               { return SendToUserToolName }
func (sendToUserTool) Description() string        { return "exit tool" }
func (sendToUserTool) Schema() stdjson.RawMessage { return stdjson.RawMessage(`{"type":"object"}`) }

func (sendToUserTool) Execute(_ context.Context, _ tool.Call, _ session.EventSink) (tool.Result, error) {
	return tool.Result{Content: "{\"ok\":true}"}, nil
}

// transcriptRecorder gives tests post-Run access to the captured event
// stream via the session's own transcript (which is appended synchronously
// on Emit). Reading the transcript directly avoids the fanout-channel
// timing race between Run returning and a subscriber goroutine catching up.
type transcriptRecorder struct {
	sess session.Session
}

func (r *transcriptRecorder) snapshot() []session.Event {
	return r.sess.Transcript().Events()
}

func newTestSession(t *testing.T) (session.Session, *transcriptRecorder) {
	t.Helper()
	l, err := glog.NewConsoleWithName("test_loop", glog.LevelError)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	// Generous buffer so the session's fanout worker never blocks during a
	// test even if no one drains the subscriber channel. We rely on the
	// transcript (appended synchronously inside Emit) for assertions, not
	// the subscriber channel — that way we avoid a race between a drain
	// goroutine starting up and the t.Cleanup Close call.
	sess := session.NewSession(session.Config{
		BufferSize: 4096,
		Logger:     l,
	})
	t.Cleanup(func() {
		_ = sess.Close()
	})
	return sess, &transcriptRecorder{sess: sess}
}

// scriptedRound is a small DSL for building a one-round model batch.
type scriptedRound struct {
	textChunks    []string
	reasoningChunks []string
	functionCalls []model.FunctionCall
	usage         *model.Usage
}

func (r scriptedRound) chunks() []model.StreamChunk {
	out := make([]model.StreamChunk, 0, len(r.textChunks)+len(r.functionCalls)+2)
	for _, t := range r.reasoningChunks {
		out = append(out, model.StreamChunk{Kind: model.ChunkReasoning, Text: t})
	}
	for _, t := range r.textChunks {
		out = append(out, model.StreamChunk{Kind: model.ChunkText, Text: t})
	}
	for i := range r.functionCalls {
		fc := r.functionCalls[i]
		out = append(out, model.StreamChunk{Kind: model.ChunkFunction, FunctionCall: &fc})
	}
	if r.usage != nil {
		out = append(out, model.StreamChunk{Kind: model.ChunkUsage, Usage: r.usage})
	}
	out = append(out, model.StreamChunk{Kind: model.ChunkDone})
	return out
}

// rawArgs marshals a Go value into json.RawMessage for FunctionCall.Arguments.
func rawArgs(t *testing.T, v any) stdjson.RawMessage {
	t.Helper()
	b, err := stdjson.Marshal(v)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return b
}

// sendToUserBatch is a small helper to build a one-round batch that exits.
func sendToUserBatch(t *testing.T, finalText string) []model.StreamChunk {
	return scriptedRound{
		functionCalls: []model.FunctionCall{{
			CallID:    "send-1",
			Name:      SendToUserToolName,
			Arguments: rawArgs(t, map[string]any{"final_answer": finalText}),
		}},
	}.chunks()
}

// buildTestRegistry creates a registry pre-populated with the supplied
// tools plus the send_to_user stub.
func buildTestRegistry(t *testing.T, tools ...tool.Tool) tool.Registry {
	t.Helper()
	l, err := glog.NewConsoleWithName("test_registry", glog.LevelError)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	reg := tool.NewRegistry(l)
	if err := reg.Register(sendToUserTool{}, tool.SourceLocal); err != nil {
		t.Fatalf("register send_to_user: %v", err)
	}
	for _, tl := range tools {
		if err := reg.Register(tl, tool.SourceCuratedMCP); err != nil {
			t.Fatalf("register %s: %v", tl.Name(), err)
		}
	}
	return reg
}
