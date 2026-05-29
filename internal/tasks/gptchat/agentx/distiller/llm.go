package distiller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
)

// LLMDistiller summarises raw tool output via an upstream model.Client.
// It is the production backend wired by loop.NewDistillHook; the
// deterministic head/tail truncator (FallbackTruncate) is the failure-mode
// fallback that runs when the LLM call errors or times out.
//
// LLMDistiller is safe for concurrent use; the underlying model.Client
// is expected to be.
type LLMDistiller struct {
	// Client is the upstream LLM. Required; Distill returns an error when
	// nil so misconfiguration is caught at the boundary instead of
	// silently passing raw content through.
	Client model.Client
	// Model is the model identifier passed on every Request. Empty means
	// the client picks its own default — fine for tests, ill-advised for
	// production.
	Model string
	// Timeout caps each Distill call. Zero falls back to DefaultTimeout.
	Timeout time.Duration
	// MaxOutputTokens hard-caps the summariser's reply. Zero means
	// `target * 2`, giving room for headers without letting the
	// summariser ignore the soft cap entirely.
	MaxOutputTokens uint
	// Cache (when non-nil) deduplicates calls across the process. The
	// hook is the canonical builder; tests may inject a fake or nil.
	Cache *Cache
	// PromptVersion tags the system-prompt template revision in the
	// cache key. Bumped whenever the prompt body changes so stale
	// entries from an older template are not reused.
	PromptVersion int
	// FallbackHead and FallbackTail size the deterministic truncation
	// used when the LLM call errors or times out. Zero means the
	// package defaults.
	FallbackHead int
	FallbackTail int
}

// NewLLMDistiller constructs a Distiller with sensible defaults wired in.
// modelID is the upstream model name passed on every summariser call;
// cache may be nil to disable caching.
func NewLLMDistiller(client model.Client, modelID string, cache *Cache) *LLMDistiller {
	return &LLMDistiller{
		Client:        client,
		Model:         modelID,
		Timeout:       DefaultTimeout,
		Cache:         cache,
		PromptVersion: PromptVersion,
		FallbackHead:  DefaultFallbackHeadBytes,
		FallbackTail:  DefaultFallbackTailBytes,
	}
}

// Distill summarises req.Raw. Cache hits return immediately. On LLM
// failure (network error, timeout, empty output) the function returns a
// deterministic head/tail truncation under Truncated=true and a nil
// error so the parent ReAct loop is never stalled by a summariser
// hiccup. A nil receiver or a nil Client is the one configuration that
// does surface an error — those are programmer mistakes, not runtime
// hiccups.
func (d *LLMDistiller) Distill(ctx context.Context, req Request) (Result, error) {
	if d == nil || d.Client == nil {
		return Result{
			Content:   FallbackTruncate(req.Raw, headOrDefault(d), tailOrDefault(d)),
			Truncated: true,
		}, errors.New("distiller: nil client")
	}
	target := req.TargetTokens
	if target <= 0 {
		target = DefaultTargetTokens
	}

	if d.Cache != nil {
		if cached, ok := d.Cache.Get(d.cacheKey(req, target)); ok {
			return Result{Content: cached, CacheHit: true}, nil
		}
	}

	timeout := d.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	text, err := d.callLLM(callCtx, req, target)
	if err != nil {
		fallback := FallbackTruncate(req.Raw, headOrDefault(d), tailOrDefault(d))
		header := fmt.Sprintf("[summariser failed: %s; raw truncated]\n", err.Error())
		return Result{Content: header + fallback, Truncated: true}, nil
	}

	if d.Cache != nil {
		d.Cache.Put(d.cacheKey(req, target), text)
	}
	return Result{Content: text}, nil
}

// cacheKey assembles the composite cache key. The format is documented
// in cache.go: tool_name : content_hash : model : prompt_version :
// target_tokens : anchors_hash. The anchors hash folds in UserPrompt
// and AssistantHint so a change in agent reasoning produces a fresh
// summary even when raw output is identical.
func (d *LLMDistiller) cacheKey(req Request, target int) string {
	var anchors strings.Builder
	anchors.WriteString(req.UserPrompt)
	anchors.WriteByte(0)
	anchors.WriteString(req.AssistantHint)
	return strings.Join([]string{
		req.ToolName,
		hashContent(req.Raw),
		d.Model,
		fmt.Sprintf("v%d", d.PromptVersion),
		fmt.Sprintf("t%d", target),
		hashContent(anchors.String()),
	}, ":")
}

// callLLM dispatches one summariser model call, drains the stream, and
// returns the concatenated assistant text. A stream error or an empty
// final text both surface as a Go error so Distill can fall back to
// deterministic truncation.
func (d *LLMDistiller) callLLM(ctx context.Context, req Request, target int) (string, error) {
	sys := buildSystemPrompt(target)
	usr := buildUserPrompt(req, target)

	maxOut := d.MaxOutputTokens
	if maxOut == 0 {
		// Soft cap target * 2 to give room for the summary header but
		// still bound runaway output.
		maxOut = uint(target * 2) //nolint:gosec // target is bounded by caller; cannot overflow uint
	}

	modelReq := model.Request{
		Model: d.Model,
		Input: []model.InputItem{
			map[string]any{"role": "system", "content": sys},
			map[string]any{"role": "user", "content": usr},
		},
		Stream:          true,
		MaxOutputTokens: maxOut,
		Temperature:     0,
	}
	ch, err := d.Client.Stream(ctx, modelReq)
	if err != nil {
		return "", errors.Wrap(err, "summariser stream")
	}
	var out strings.Builder
	var streamErr error
	for chunk := range ch {
		switch chunk.Kind {
		case model.ChunkText:
			out.WriteString(chunk.Text)
		case model.ChunkError:
			if chunk.Err != nil {
				streamErr = chunk.Err
			} else if chunk.Text != "" {
				streamErr = errors.New(chunk.Text)
			}
		}
	}
	if streamErr != nil {
		return "", streamErr
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return "", errors.New("summariser returned empty output")
	}
	return text, nil
}

func headOrDefault(d *LLMDistiller) int {
	if d == nil || d.FallbackHead <= 0 {
		return DefaultFallbackHeadBytes
	}
	return d.FallbackHead
}

func tailOrDefault(d *LLMDistiller) int {
	if d == nil || d.FallbackTail <= 0 {
		return DefaultFallbackTailBytes
	}
	return d.FallbackTail
}
