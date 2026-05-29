// Package distiller turns oversize tool outputs into short, high-density
// "Observation" strings before they enter the next ReAct round's input.
//
// The classic ReAct loop has an Observation role that *summarises* tool
// results into context — agentx Phase 1 collapsed Observation into raw
// `function_call_output`, which lets large web pages or file dumps bloat
// the next-round prompt verbatim. This package supplies the missing step:
// a Distiller takes the raw tool output plus salience anchors (user
// prompt, last assistant reasoning, tool args) and returns a compressed
// string capped at a few hundred tokens.
//
// Wiring is via an OnAfterToolCall hook (see loop.NewDistillHook): the
// hook reads the raw `tool.Result.Content`, calls Distill, replaces the
// content with the distilled string, and stashes the raw bytes on the
// session transcript so callers can recover them post-hoc.
package distiller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DefaultThresholdTokens is the estimated-token threshold above which a
// raw tool output is routed through the summariser. Outputs at or below
// this pass through unchanged.
const DefaultThresholdTokens = 800

// DefaultTargetTokens is the soft cap given to the summariser as the
// requested summary length. The hook also passes a hard MaxOutputTokens
// to the model.Client so a runaway summariser cannot blow past the soft
// cap by much.
const DefaultTargetTokens = 300

// DefaultTimeout bounds each Distill call. Tight by design — a slow
// summariser must not stall the parent ReAct loop's round.
const DefaultTimeout = 8 * time.Second

// DefaultFallbackHeadBytes and DefaultFallbackTailBytes shape the
// deterministic head/tail truncation that runs when the LLM summariser
// errors or times out.
const (
	DefaultFallbackHeadBytes = 1024
	DefaultFallbackTailBytes = 512
)

// Request bundles the inputs a Distiller needs to summarise one raw tool
// output. The salience anchors (UserPrompt, AssistantHint) tell the
// summariser what dimension of the raw text to preserve; without them the
// summariser has no theory of relevance and tends to drop load-bearing
// detail.
type Request struct {
	// ToolName is the registered tool the raw output came from. Used in
	// the summariser prompt and in the cache key.
	ToolName string
	// Args is the raw JSON arguments the model passed to the tool. The
	// summariser uses it as a salience anchor (e.g. for web_search, the
	// query terms).
	Args json.RawMessage
	// Raw is the verbatim tool output to be summarised.
	Raw string
	// UserPrompt is the original user-turn text. Stable across the
	// session — prompt-cache-friendly when the underlying provider
	// supports caching.
	UserPrompt string
	// AssistantHint is the most recent assistant reasoning / plan; it
	// tells the summariser what the agent was looking for on this round.
	AssistantHint string
	// CallID is the tool-call identifier. The hook uses it to key the
	// transcript raw-stash; the cache uses it only indirectly via Raw's
	// content hash.
	CallID string
	// TargetTokens is the soft summary-length cap shipped to the
	// summariser prompt. Zero means DefaultTargetTokens.
	TargetTokens int
}

// Result is the Distiller's return shape.
type Result struct {
	// Content is the distilled, high-density observation that replaces
	// the raw tool output in the next-round input.
	Content string
	// CacheHit is true when the result was served from the cache.
	CacheHit bool
	// Truncated is true when the LLM call failed and deterministic
	// head/tail truncation was used as a fallback. The Content field
	// already carries a visible failure header in that case; this flag
	// is for tests and telemetry.
	Truncated bool
}

// Distiller is the single shape every distillation backend implements.
// Implementations must be safe for concurrent use — the loop fans tool
// calls out in parallel and a single Distiller instance is shared across
// the fan-out.
type Distiller interface {
	Distill(ctx context.Context, req Request) (Result, error)
}

// EstimateTokens is the cheap, conservative chars-per-token approximation
// the hook uses to decide whether to call Distill at all. Four characters
// per token is the canonical heuristic for ASCII English; CJK is denser
// so this overestimates, which is fine — the threshold itself is a
// heuristic, not an SLA.
func EstimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// FallbackTruncate returns a deterministic head/tail truncation of raw
// prefaced by a noisy header so downstream callers can see the fallback
// happened. The output never exceeds len(header) + head + len(separator)
// + tail bytes. Inputs shorter than head+tail pass through unchanged.
func FallbackTruncate(raw string, head, tail int) string {
	if head < 0 {
		head = 0
	}
	if tail < 0 {
		tail = 0
	}
	if len(raw) <= head+tail {
		return raw
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[truncated: kept first %d and last %d bytes of %d total]\n",
		head, tail, len(raw))
	b.WriteString(raw[:head])
	b.WriteString("\n…\n")
	b.WriteString(raw[len(raw)-tail:])
	return b.String()
}

// hashContent returns the hex SHA-256 of s. The Distiller uses it as a
// component of the cache key so the raw string itself never appears in
// the key.
func hashContent(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
