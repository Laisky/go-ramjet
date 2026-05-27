package agentx

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/json"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// coerceInputItems normalises a Responses-API input slice (which may arrive
// as the typed []httppkg.OpenAIResponsesInputMessage produced by
// convert2UpstreamResponsesRequest, or as a []any of bare map[string]any
// items after a JSON unmarshal — e.g. the memory enrichment hook output at
// responses_chat_handler.go:789 — or any mixture of the two shapes once the
// loop has injected its own map-shaped userMessage / systemMessage entries)
// into the three concrete structs accepted by the OneAPI Responses
// adapter's validateInputItem:
//
//   - httppkg.OpenAIResponsesInputMessage (role+content)
//   - httppkg.OpenAIResponsesFunctionCall (model's prior tool call)
//   - httppkg.OpenAIResponsesFunctionCallOutput (matching tool result)
//
// Per-item rules:
//
//   - typed structs (value or pointer) pass through unchanged so the caller's
//     identity / pointer-equality is preserved when no conversion is needed.
//   - map[string]any with a "role" field is round-tripped via JSON into
//     OpenAIResponsesInputMessage. We use the "role" field rather than the
//     "type" discriminator because the OpenAI Responses input message shape
//     does not carry a top-level "type" field; the role is the only stable
//     discriminator (system / user / assistant / developer).
//   - map[string]any with "type": "function_call" → OpenAIResponsesFunctionCall.
//   - map[string]any with "type": "function_call_output" →
//     OpenAIResponsesFunctionCallOutput.
//   - anything else returns a wrapped error that names the offending index
//     AND a JSON-truncated preview of the item so future shape drift is
//     obvious in the logs.
//
// nil items are skipped (defensive: maps of nil interface values can sneak
// through certain JSON unmarshal paths).
func coerceInputItems(items []any) ([]model.InputItem, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]model.InputItem, 0, len(items))
	for i, item := range items {
		converted, err := coerceInputItem(item)
		if err != nil {
			return nil, errors.Wrapf(err, "Input[%d] %s", i, previewItemForError(item))
		}
		if converted == nil {
			continue
		}
		out = append(out, converted)
	}
	return out, nil
}

// coerceInputItem handles one slot. Kept separate so the loop body in
// coerceInputItems stays small enough to inline mentally.
func coerceInputItem(item any) (model.InputItem, error) {
	switch v := item.(type) {
	case nil:
		return nil, nil
	case httppkg.OpenAIResponsesInputMessage,
		*httppkg.OpenAIResponsesInputMessage,
		httppkg.OpenAIResponsesFunctionCall,
		*httppkg.OpenAIResponsesFunctionCall,
		httppkg.OpenAIResponsesFunctionCallOutput,
		*httppkg.OpenAIResponsesFunctionCallOutput:
		return v, nil
	case map[string]any:
		return coerceMapInputItem(v)
	}
	return nil, errors.Errorf(
		"unsupported InputItem shape %T; expected OpenAIResponsesInputMessage, "+
			"OpenAIResponsesFunctionCall, OpenAIResponsesFunctionCallOutput, "+
			"or a map[string]any with a recognised type/role",
		item)
}

// coerceMapInputItem round-trips a single map item via JSON into the
// concrete struct selected by its discriminator. JSON is used (rather than
// reflective field copying) so any field shape the upstream's JSON
// unmarshal would accept also lands here — including nested content blocks
// and string-or-object content variants.
func coerceMapInputItem(m map[string]any) (model.InputItem, error) {
	// Function calls and call outputs use an explicit "type" discriminator.
	// Check that first because a function_call_output map carries no
	// "role" field — falling through to the role branch would silently
	// drop the call_id.
	if t, ok := m["type"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "function_call":
			var out httppkg.OpenAIResponsesFunctionCall
			if err := remarshalJSON(m, &out); err != nil {
				return nil, errors.Wrap(err, "decode function_call")
			}
			return out, nil
		case "function_call_output":
			var out httppkg.OpenAIResponsesFunctionCallOutput
			if err := remarshalJSON(m, &out); err != nil {
				return nil, errors.Wrap(err, "decode function_call_output")
			}
			return out, nil
		case "message":
			// Some upstreams (and the memory hook output) tag plain
			// role messages with "type": "message". Treat them as the
			// regular input-message shape.
			var out httppkg.OpenAIResponsesInputMessage
			if err := remarshalJSON(m, &out); err != nil {
				return nil, errors.Wrap(err, "decode message")
			}
			if strings.TrimSpace(out.Role) == "" {
				return nil, errors.Errorf("message item missing role")
			}
			return out, nil
		}
	}

	// Plain role/content messages — emitted by the agent loop's
	// userMessage/systemMessage helpers and by the memory enrichment
	// hook's MemoryItemsToResponsesInput.
	if role, ok := m["role"].(string); ok && strings.TrimSpace(role) != "" {
		var out httppkg.OpenAIResponsesInputMessage
		if err := remarshalJSON(m, &out); err != nil {
			return nil, errors.Wrap(err, "decode role message")
		}
		return out, nil
	}

	return nil, errors.Errorf(
		"unrecognised map InputItem; no recognised \"type\" or \"role\" discriminator")
}

// remarshalJSON marshals src to JSON and unmarshals into dst. Used to
// reinterpret a map-shaped item as the matching typed struct without
// hand-mapping each field.
func remarshalJSON(src any, dst any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return errors.Wrap(err, "marshal")
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return errors.Wrap(err, "unmarshal")
	}
	return nil
}

// previewItemForError renders a short JSON preview of the offending item
// so error messages name both the index AND enough of the shape to
// pinpoint a future schema drift. Returns an empty string when the item
// itself cannot be marshalled (the surrounding wrap still names the
// index).
func previewItemForError(item any) string {
	if item == nil {
		return "preview=<nil>"
	}
	data, err := json.Marshal(item)
	if err != nil {
		return "preview=<marshal-error>"
	}
	const max = 200
	if len(data) > max {
		return "preview=" + string(data[:max]) + "..."
	}
	return "preview=" + string(data)
}

// inputAsAnySlice converts the typed []OpenAIResponsesInputMessage that
// convert2UpstreamResponsesRequest produces (and the bare []any that the
// memory hook may overwrite it with) into a uniform []any so
// coerceInputItems can walk both shapes through the same path.
//
// Returns nil for a nil input or an unrecognised top-level type; the
// caller treats that as "no prior transcript" and the loop seeds its own
// userMessage on top.
func inputAsAnySlice(in any) []any {
	if in == nil {
		return nil
	}
	if arr, ok := in.([]any); ok {
		return arr
	}
	if msgs, ok := in.([]httppkg.OpenAIResponsesInputMessage); ok {
		out := make([]any, len(msgs))
		for i := range msgs {
			out[i] = msgs[i]
		}
		return out
	}
	return nil
}

// coercingModelClient wraps a model.Client and coerces Request.Input into
// the three typed structs before each Stream call. The wrapper is the
// single chokepoint that upholds the §3.4 "validate at the boundary"
// contract: the loop body intentionally emits map-shaped userMessage and
// appendFunctionCallAndOutput items (see loop.go:593+ — the comment there
// notes the OneAPI adapter is expected to accept the map shapes), and the
// validator (model/oneapi.go::validateInputItem) is intentionally strict
// about concrete structs. This wrapper bridges the two.
//
// Capabilities() is forwarded unchanged so the loop's parallel-tool-call
// gating behaves identically.
type coercingModelClient struct {
	inner model.Client
}

func newCoercingModelClient(inner model.Client) model.Client {
	if inner == nil {
		return nil
	}
	if _, already := inner.(*coercingModelClient); already {
		return inner
	}
	return &coercingModelClient{inner: inner}
}

// Stream coerces every item in req.Input to one of the three accepted
// concrete shapes and forwards. A coercion error is surfaced as the same
// kind of validator-style error the OneAPI adapter would have returned —
// the caller (loop.go) treats Stream errors as terminal for the round.
func (c *coercingModelClient) Stream(ctx context.Context, req model.Request) (<-chan model.StreamChunk, error) {
	if len(req.Input) > 0 {
		coerced, err := coerceInputItems(req.Input)
		if err != nil {
			return nil, errors.Wrap(err, "coerce model.Request.Input")
		}
		req.Input = coerced
	}
	return c.inner.Stream(ctx, req)
}

// Capabilities forwards unchanged.
func (c *coercingModelClient) Capabilities() model.Capabilities {
	return c.inner.Capabilities()
}
