package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// NewCircuitHook returns an OnBeforeToolCall hook that trips when the same
// (tool_name, normalized_args) pair appears `repeats` times in a row. On
// trip, the hook synthesizes an IsError result on the event and returns it
// via the hook chain so the loop sees a tool-level failure (incrementing the
// error budget) without actually invoking the tool.
//
// The repeat counter resets when a different (tool_name, normalized_args)
// is observed — only consecutive repeats trip the breaker, matching the
// proposal §3.5 description.
//
// State is per-instance and protected by sync.Mutex so the parallel executor
// can dispatch the chain from multiple goroutines.
func NewCircuitHook(repeats int) func(context.Context, hook.ToolCallEvent) (hook.ToolCallEvent, error) {
	if repeats < 1 {
		// repeats < 1 disables the breaker; passthrough.
		return func(_ context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
			return ev, nil
		}
	}

	var (
		mu        sync.Mutex
		lastKey   string
		streak    int
		threshold = repeats
	)

	return func(_ context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
		// PointBeforeToolCall sees Result == nil; we only care about that
		// stage. Defensive guard in case the bus ever sends a populated
		// Result to a Before hook (it doesn't today).
		if ev.Result != nil {
			return ev, nil
		}

		key := ev.ToolName + "\x00" + normalizeArgs(ev.Args)

		mu.Lock()
		if key == lastKey {
			streak++
		} else {
			lastKey = key
			streak = 1
		}
		tripped := streak >= threshold
		// Reset the streak on trip so we get one synthetic failure per
		// repeat-run, not a permanent denial of every subsequent call.
		if tripped {
			streak = 0
			lastKey = ""
		}
		mu.Unlock()

		if !tripped {
			return ev, nil
		}

		ev.Result = &tool.Result{
			Content: fmt.Sprintf(
				"repeated tool call detected: %s called %d times in a row with identical arguments; circuit breaker tripped",
				ev.ToolName, threshold,
			),
			IsError: true,
		}
		return ev, nil
	}
}

// normalizeArgs returns a canonical string form of args so syntactic
// differences (whitespace, key order) don't defeat repeat detection. When
// the args are not valid JSON we fall back to the raw bytes so we still
// detect literal duplicates.
func normalizeArgs(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(args, &v); err != nil {
		return string(args)
	}
	b, err := canonicalJSON(v)
	if err != nil {
		return string(args)
	}
	return b
}

// canonicalJSON marshals v with sorted object keys, recursively, producing a
// stable encoding suitable for equality comparison.
func canonicalJSON(v any) (string, error) {
	switch x := v.(type) {
	case map[string]any:
		// Sort keys.
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sortStrings(keys)
		var buf []byte
		buf = append(buf, '{')
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return "", err
			}
			buf = append(buf, kb...)
			buf = append(buf, ':')
			vs, err := canonicalJSON(x[k])
			if err != nil {
				return "", err
			}
			buf = append(buf, vs...)
		}
		buf = append(buf, '}')
		return string(buf), nil
	case []any:
		var buf []byte
		buf = append(buf, '[')
		for i, el := range x {
			if i > 0 {
				buf = append(buf, ',')
			}
			vs, err := canonicalJSON(el)
			if err != nil {
				return "", err
			}
			buf = append(buf, vs...)
		}
		buf = append(buf, ']')
		return string(buf), nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

// sortStrings is a tiny insertion sort to avoid importing "sort" just for
// the canonical-JSON helper. Object key counts in tool arg shapes are small.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1] > s[j] {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}
