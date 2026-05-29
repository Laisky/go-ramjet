package distiller

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
)

// fakeClient is a model.Client substitute for tests. It records call
// counts and returns scripted text chunks. When fail is non-nil, Stream
// returns an error before producing any chunks.
type fakeClient struct {
	fail   error
	chunks []model.StreamChunk
	calls  atomic.Int64
}

func (f *fakeClient) Stream(_ context.Context, _ model.Request) (<-chan model.StreamChunk, error) {
	f.calls.Add(1)
	if f.fail != nil {
		return nil, f.fail
	}
	ch := make(chan model.StreamChunk, len(f.chunks)+1)
	for _, c := range f.chunks {
		ch <- c
	}
	ch <- model.StreamChunk{Kind: model.ChunkDone}
	close(ch)
	return ch, nil
}

func (f *fakeClient) Capabilities() model.Capabilities { return model.Capabilities{} }

func TestEstimateTokens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abcd", 1},
		{"abcde", 2},
		{strings.Repeat("a", 4000), 1000},
	}
	for _, c := range cases {
		if got := EstimateTokens(c.in); got != c.want {
			t.Errorf("EstimateTokens(%d chars): want %d got %d", len(c.in), c.want, got)
		}
	}
}

func TestFallbackTruncate(t *testing.T) {
	t.Parallel()

	t.Run("short input passes through", func(t *testing.T) {
		if FallbackTruncate("short", 1024, 512) != "short" {
			t.Fatal("short content was modified")
		}
	})

	t.Run("drops middle, keeps head and tail", func(t *testing.T) {
		raw := strings.Repeat("a", 100) + "MIDDLE-NUKED" + strings.Repeat("b", 100)
		out := FallbackTruncate(raw, 10, 10)
		if !strings.Contains(out, "kept first 10") {
			t.Fatalf("missing header: %q", out)
		}
		if strings.Contains(out, "MIDDLE-NUKED") {
			t.Fatalf("middle survived truncation: %q", out)
		}
		if !strings.HasSuffix(strings.TrimSpace(out), strings.Repeat("b", 10)) {
			t.Fatalf("tail bytes missing: %q", out)
		}
	})
}

func TestLLMDistillerHappyPathAndCache(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{chunks: []model.StreamChunk{
		{Kind: model.ChunkText, Text: "Ottawa forecast: May 28, "},
		{Kind: model.ChunkText, Text: "10°C, mostly sunny."},
	}}
	cache := NewCache()
	d := NewLLMDistiller(fc, "test-summariser", cache)
	d.Timeout = 2 * time.Second

	req := Request{
		ToolName:   "web_fetch",
		Args:       stdjson.RawMessage(`{"url":"https://weather.gc.ca"}`),
		Raw:        strings.Repeat("noise ", 1000),
		UserPrompt: "What's the Ottawa forecast for May 28?",
	}
	res, err := d.Distill(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Content, "Ottawa") {
		t.Fatalf("missing summariser output: %q", res.Content)
	}
	if res.CacheHit || res.Truncated {
		t.Fatalf("unexpected flags on first call: %+v", res)
	}
	if got := fc.calls.Load(); got != 1 {
		t.Fatalf("first call: want 1 LLM call, got %d", got)
	}

	res2, err := d.Distill(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if !res2.CacheHit {
		t.Fatalf("expected CacheHit=true on identical request, got %+v", res2)
	}
	if got := fc.calls.Load(); got != 1 {
		t.Fatalf("cache miss on second call: LLM was called %d times", got)
	}
	if res2.Content != res.Content {
		t.Fatalf("cache returned different content: %q vs %q", res2.Content, res.Content)
	}
}

func TestLLMDistillerCacheKeyVariesByAnchors(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{chunks: []model.StreamChunk{{Kind: model.ChunkText, Text: "summary"}}}
	d := NewLLMDistiller(fc, "m", NewCache())
	d.Timeout = time.Second

	base := Request{Raw: "same raw content", ToolName: "tool"}
	if _, err := d.Distill(context.Background(), base); err != nil {
		t.Fatal(err)
	}

	withAnchor := base
	withAnchor.UserPrompt = "different goal"
	if _, err := d.Distill(context.Background(), withAnchor); err != nil {
		t.Fatal(err)
	}
	if got := fc.calls.Load(); got != 2 {
		t.Fatalf("expected anchor change to bust cache; got %d LLM calls", got)
	}
}

func TestLLMDistillerFailureFallsBack(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{fail: errors.New("upstream 500")}
	d := NewLLMDistiller(fc, "m", nil)
	d.Timeout = 100 * time.Millisecond

	raw := strings.Repeat("payload-", 500)
	res, err := d.Distill(context.Background(), Request{Raw: raw})
	if err != nil {
		t.Fatalf("Distill should swallow LLM error; got %v", err)
	}
	if !res.Truncated {
		t.Fatalf("expected Truncated=true on LLM failure, got %+v", res)
	}
	if !strings.Contains(res.Content, "summariser failed") {
		t.Fatalf("expected failure header in fallback content: %q", res.Content)
	}
	if !strings.Contains(res.Content, "kept first") {
		t.Fatalf("expected truncated body in fallback content: %q", res.Content)
	}
}

func TestLLMDistillerEmptyOutputFallsBack(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{chunks: []model.StreamChunk{{Kind: model.ChunkText, Text: "   "}}}
	d := NewLLMDistiller(fc, "m", nil)

	res, err := d.Distill(context.Background(), Request{Raw: strings.Repeat("x", 100)})
	if err != nil {
		t.Fatalf("Distill should swallow empty output; got %v", err)
	}
	if !res.Truncated {
		t.Fatalf("expected Truncated=true on empty summariser output, got %+v", res)
	}
}

func TestLLMDistillerStreamError(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{chunks: []model.StreamChunk{
		{Kind: model.ChunkText, Text: "partial..."},
		{Kind: model.ChunkError, Err: errors.New("network blip")},
	}}
	d := NewLLMDistiller(fc, "m", nil)

	res, err := d.Distill(context.Background(), Request{Raw: strings.Repeat("x", 100)})
	if err != nil {
		t.Fatalf("Distill should swallow stream error; got %v", err)
	}
	if !res.Truncated || !strings.Contains(res.Content, "network blip") {
		t.Fatalf("expected fallback header containing network blip: %q", res.Content)
	}
}

func TestLLMDistillerNilClient(t *testing.T) {
	t.Parallel()
	d := &LLMDistiller{}
	res, err := d.Distill(context.Background(), Request{Raw: "x"})
	if err == nil {
		t.Fatal("expected error on nil client")
	}
	if !res.Truncated {
		t.Fatal("expected fallback Result even on nil client")
	}
}

func TestBuildUserPromptIncludesAllAnchors(t *testing.T) {
	t.Parallel()
	p := buildUserPrompt(Request{
		UserPrompt:    "Find URLs about Ottawa",
		AssistantHint: "I'll search and then fetch",
		ToolName:      "web_fetch",
		Args:          stdjson.RawMessage(`{"url":"https://x"}`),
		Raw:           "RAWBODY",
	}, 200)
	for _, want := range []string{
		"Ottawa", "search and then fetch", "web_fetch",
		"https://x", "RAWBODY", "<RAW>", "</RAW>", "TARGET LENGTH",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q in prompt:\n%s", want, p)
		}
	}
}

func TestBuildSystemPromptHasUntrustedGuard(t *testing.T) {
	t.Parallel()
	p := buildSystemPrompt(300)
	for _, want := range []string{
		"UNTRUSTED DATA",
		"ignore",
		"contains embedded instructions",
		"PRESERVE VERBATIM",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q in system prompt:\n%s", want, p)
		}
	}
}

func TestCacheNilReceiverIsNoop(t *testing.T) {
	t.Parallel()
	var c *Cache
	if v, ok := c.Get("k"); ok || v != "" {
		t.Fatalf("nil Cache.Get should miss; got %q,%v", v, ok)
	}
	c.Put("k", "v") // must not panic
}

func TestTruncateArgs(t *testing.T) {
	t.Parallel()
	if truncateArgs("short", 100) != "short" {
		t.Fatal("short args should pass through")
	}
	out := truncateArgs(strings.Repeat("x", 200), 50)
	if !strings.HasPrefix(out, strings.Repeat("x", 50)) {
		t.Fatalf("missing head: %q", out)
	}
	if !strings.Contains(out, "args truncated") {
		t.Fatalf("missing truncation note: %q", out)
	}
}

func TestMarshalArgsRoundTrip(t *testing.T) {
	t.Parallel()
	in := stdjson.RawMessage(`{"a": 1,    "b": "x"}`)
	got := marshalArgs(in)
	if !strings.Contains(got, `"a":1`) || !strings.Contains(got, `"b":"x"`) {
		t.Fatalf("re-marshalled args lost fields: %q", got)
	}

	if marshalArgs(nil) != "" {
		t.Fatal("empty args should marshal to empty string")
	}

	bad := stdjson.RawMessage(`{not json`)
	if marshalArgs(bad) != `{not json` {
		t.Fatal("non-JSON args should fall back to raw bytes")
	}
}
