package model

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"

	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// oneAPIClient is the Phase 1 Client implementation. It wraps
// http.CallUpstreamResponsesCtx for non-streaming requests and
// http.StreamUpstreamResponsesEventsCtx for streaming requests, translating
// the model-agnostic Request into the OneAPI Responses-API wire shape and
// the upstream's SSE events into typed StreamChunks.
//
// The adapter is stateless — all per-request state lives in the goroutine
// the Stream call spawns. Multiple concurrent Stream invocations are safe.
type oneAPIClient struct {
	deps OneAPIDeps
}

// NewOneAPIClient returns the Phase 1 OneAPI-backed model.Client.
//
// Capability table is hard-coded for Phase 1 — see Capabilities() docstring
// for the rationale. Future implementations would consult a per-model table
// (e.g. parsed from a YAML configmap).
func NewOneAPIClient(deps OneAPIDeps) Client {
	return &oneAPIClient{deps: deps}
}

// Capabilities reports static capability flags for the OneAPI backend.
//
// Phase 1 values:
//
//   - SupportsParallelToolCalls: true. All current Responses-API-aware
//     upstreams (Anthropic 4.x, OpenAI o-series, Google Gemini) handle
//     parallel function_calls. Older Claude 3.x is intentionally not on
//     the curated belt; if it were, we'd add a per-model table here.
//
//   - SupportsReasoning: true. The adapter forwards Reasoning unchanged;
//     models that don't reason simply ignore the field.
//
//   - MaxContextTokens: 200000. Conservative default chosen to be safe for
//     most of the curated upstreams (Claude 4.x is 200k native, Gemini 2.x
//     1M, GPT-5 200k). The agent loop's input cap
//     (defaultMaxResponseInputTokens, currently 120k) is the binding
//     constraint in practice; this value is a hint for future compaction
//     logic, not a hard enforcement.
//
// A future implementation would consult a per-model lookup table keyed by
// the request Model string; that table replaces these constants without
// changing the interface.
func (c *oneAPIClient) Capabilities() Capabilities {
	return Capabilities{
		SupportsParallelToolCalls: true,
		SupportsReasoning:         true,
		MaxContextTokens:          200000,
	}
}

// Stream is the entry point. It validates the request shape, translates it
// to the OneAPI wire format, and dispatches to either the streaming or
// non-streaming worker. Both workers push StreamChunks onto the returned
// channel and close it exactly once.
func (c *oneAPIClient) Stream(ctx context.Context, req Request) (<-chan StreamChunk, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("empty Model in Request")
	}
	wireReq, err := c.translateRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "translate request")
	}

	out := make(chan StreamChunk, 16)

	if req.Stream {
		go c.runStreaming(ctx, wireReq, out)
	} else {
		go c.runNonStreaming(ctx, wireReq, out)
	}

	return out, nil
}

// translateRequest converts a model.Request into the OneAPI wire shape.
// Input items are validated against the three accepted concrete types; any
// other shape returns an error without issuing an upstream call (per the
// proposal §3.4 "validate at the boundary" contract).
func (c *oneAPIClient) translateRequest(req Request) (*httppkg.OpenAIResponsesReq, error) {
	caps := c.Capabilities()

	wire := &httppkg.OpenAIResponsesReq{
		Model:           req.Model,
		MaxOutputTokens: req.MaxOutputTokens,
		Stream:          req.Stream,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		ToolChoice:      req.ToolChoice,
	}

	if req.Reasoning != nil {
		effort := req.Reasoning.Effort
		summary := req.Reasoning.Summary
		var effPtr, sumPtr *string
		if strings.TrimSpace(effort) != "" {
			effPtr = &effort
		}
		if strings.TrimSpace(summary) != "" {
			sumPtr = &summary
		}
		if effPtr != nil || sumPtr != nil {
			wire.Reasoning = &httppkg.OpenAIResponseReasoning{
				Effort:  effPtr,
				Summary: sumPtr,
			}
		}
	}

	// Tools — straight 1:1 mapping. Empty tools is allowed (some turns send
	// no tools, e.g. a forced final answer round).
	if len(req.Tools) > 0 {
		wireTools := make([]httppkg.OpenAIResponsesTool, 0, len(req.Tools))
		for i, t := range req.Tools {
			name := strings.TrimSpace(t.Name)
			if name == "" {
				return nil, errors.Errorf("Tools[%d] has empty Name", i)
			}
			wireTools = append(wireTools, httppkg.OpenAIResponsesTool{
				Type:        "function",
				Name:        name,
				Description: strings.TrimSpace(t.Description),
				Parameters:  t.Schema,
			})
		}
		wire.Tools = wireTools
	}

	// Input items — validate then pass through.
	if len(req.Input) > 0 {
		validatedInput := make([]any, 0, len(req.Input))
		for i, item := range req.Input {
			validated, verr := validateInputItem(item)
			if verr != nil {
				return nil, errors.Wrapf(verr, "Input[%d]", i)
			}
			validatedInput = append(validatedInput, validated)
		}
		wire.Input = validatedInput
	}

	// Capability gate (§3.8 invariant 6 / test U27). The wire request's
	// ParallelToolCalls field is set only when the request asked for it AND
	// the model supports it. When the upstream does not support parallel
	// function-calls, the field is forced false on the wire regardless of
	// what the caller requested.
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

// validateInputItem returns the item unchanged if its concrete type matches
// one of the three shapes the adapter accepts; otherwise it returns a
// descriptive error.
func validateInputItem(item any) (any, error) {
	switch item.(type) {
	case httppkg.OpenAIResponsesInputMessage,
		*httppkg.OpenAIResponsesInputMessage,
		httppkg.OpenAIResponsesFunctionCall,
		*httppkg.OpenAIResponsesFunctionCall,
		httppkg.OpenAIResponsesFunctionCallOutput,
		*httppkg.OpenAIResponsesFunctionCallOutput:
		return item, nil
	default:
		return nil, errors.Errorf(
			"unsupported InputItem shape %T; expected OpenAIResponsesInputMessage, "+
				"OpenAIResponsesFunctionCall, or OpenAIResponsesFunctionCallOutput",
			item)
	}
}

// pendingCall accumulates a function_call across SSE delta events. The
// OneAPI Responses stream emits the call shell on output_item.added, the
// arguments incrementally on function_call_arguments.delta, and the
// terminal event on function_call_arguments.done or output_item.done.
type pendingCall struct {
	itemID    string
	callID    string
	name      string
	argBuffer strings.Builder
	emitted   bool
}

// runStreaming consumes upstream SSE events and emits typed StreamChunks.
func (c *oneAPIClient) runStreaming(
	ctx context.Context,
	wire *httppkg.OpenAIResponsesReq,
	out chan<- StreamChunk,
) {
	defer close(out)

	deps := c.deps.UpstreamDeps
	// The agent path is the only consumer of typed events; we do not
	// also write framed chat-completion chunks to a sink.
	deps.StreamSink = nil

	byItemID := make(map[string]*pendingCall)
	emittedCallIDs := make(map[string]bool)

	emitFunction := func(callID, name, args string) {
		if callID == "" || name == "" || emittedCallIDs[callID] {
			return
		}
		emittedCallIDs[callID] = true
		out <- StreamChunk{
			Kind: ChunkFunction,
			FunctionCall: &FunctionCall{
				CallID:    callID,
				Name:      name,
				Arguments: json.RawMessage(args),
			},
		}
	}

	finalResp, _, err := upstreamStreamFn(ctx, deps, wire, func(ev httppkg.ResponsesRawEvent) error {
		switch ev.Type {
		case "response.output_text.delta", "response.refusal.delta":
			text := ev.Delta
			if ev.Type == "response.refusal.delta" {
				text = "refusal: " + text
			}
			if text != "" {
				out <- StreamChunk{Kind: ChunkText, Text: text}
			}
		case "response.output_text.done":
			// Aggregate-only; deltas already streamed.
		case "response.reasoning_text.delta",
			"response.reasoning.delta",
			"response.reasoning_summary_text.delta",
			"response.reasoning_summary_part.added",
			"response.thought.delta":
			text := extractReasoningText(ev)
			if text != "" {
				out <- StreamChunk{Kind: ChunkReasoning, Text: text}
			}
		case "response.reasoning_text.done",
			"response.reasoning_summary_text.done",
			"response.reasoning_summary_part.done",
			"response.thought.done":
			// Aggregate-only event; deltas already streamed.
		case "response.output_item.added":
			var added struct {
				Item httppkg.OpenAIResponsesFunctionCall `json:"item"`
			}
			if err := json.Unmarshal(ev.Raw, &added); err == nil && added.Item.Type == "function_call" {
				key := added.Item.ID
				if key == "" {
					key = added.Item.CallID
				}
				p := &pendingCall{
					itemID: added.Item.ID,
					callID: added.Item.CallID,
					name:   added.Item.Name,
				}
				p.argBuffer.WriteString(added.Item.Arguments)
				byItemID[key] = p
			}
		case "response.function_call_arguments.delta":
			var d struct {
				ItemID      string `json:"item_id"`
				OutputIndex int    `json:"output_index"`
			}
			_ = json.Unmarshal(ev.Raw, &d)
			if p, ok := byItemID[d.ItemID]; ok && ev.Delta != "" {
				p.argBuffer.WriteString(ev.Delta)
			}
		case "response.function_call_arguments.done":
			var d struct {
				ItemID    string `json:"item_id"`
				Arguments string `json:"arguments"`
			}
			if err := json.Unmarshal(ev.Raw, &d); err == nil {
				if p, ok := byItemID[d.ItemID]; ok {
					if d.Arguments != "" {
						p.argBuffer.Reset()
						p.argBuffer.WriteString(d.Arguments)
					}
					emitFunction(p.callID, p.name, p.argBuffer.String())
					p.emitted = true
				}
			}
		case "response.output_item.done":
			var d struct {
				Item httppkg.OpenAIResponsesFunctionCall `json:"item"`
			}
			if err := json.Unmarshal(ev.Raw, &d); err == nil && d.Item.Type == "function_call" {
				emitFunction(d.Item.CallID, d.Item.Name, d.Item.Arguments)
				if p, ok := byItemID[d.Item.ID]; ok {
					p.emitted = true
				}
				if p, ok := byItemID[d.Item.CallID]; ok {
					p.emitted = true
				}
			}
		case "response.completed":
			if ev.Response != nil {
				flushPending(byItemID, emitFunction)
				flushCompletedFunctionCalls(ev.Response, emittedCallIDs, func(fc *FunctionCall) {
					out <- StreamChunk{Kind: ChunkFunction, FunctionCall: fc}
				})
				if u := extractUsageFromRaw(ev.Raw); u != nil {
					out <- StreamChunk{Kind: ChunkUsage, Usage: u}
				}
			}
		}
		return nil
	})

	if err != nil {
		out <- StreamChunk{
			Kind: ChunkError,
			Text: err.Error(),
			Err:  err,
		}
		return
	}

	// Defense-in-depth: if the upstream skipped response.completed entirely
	// (truncated stream), flush whatever we accumulated.
	flushPending(byItemID, emitFunction)
	if finalResp != nil {
		flushCompletedFunctionCalls(finalResp, emittedCallIDs, func(fc *FunctionCall) {
			out <- StreamChunk{Kind: ChunkFunction, FunctionCall: fc}
		})
	}

	out <- StreamChunk{Kind: ChunkDone}
}

// flushPending emits any pending function_call that hasn't been emitted
// yet. Used as a defensive measure when the upstream skips a discrete
// .done event for one or more calls.
func flushPending(byItemID map[string]*pendingCall, emit func(callID, name, args string)) {
	for _, p := range byItemID {
		if p.emitted {
			continue
		}
		emit(p.callID, p.name, p.argBuffer.String())
		p.emitted = true
	}
}

// flushCompletedFunctionCalls emits any function_call items present in the
// final response that weren't already emitted via SSE deltas. This is the
// belt-and-suspenders path used when the upstream batches function calls
// onto response.completed instead of streaming them per item.
func flushCompletedFunctionCalls(
	resp *httppkg.OpenAIResponsesResp,
	emitted map[string]bool,
	emit func(*FunctionCall),
) {
	calls, err := extractFunctionCallsFromResp(resp)
	if err != nil {
		return
	}
	for _, fc := range calls {
		if emitted[fc.CallID] {
			continue
		}
		emitted[fc.CallID] = true
		emit(&FunctionCall{
			CallID:    fc.CallID,
			Name:      fc.Name,
			Arguments: json.RawMessage(fc.Arguments),
		})
	}
}

// runNonStreaming issues a single non-streaming upstream call and
// synthesizes a logical sequence of StreamChunks from the result.
//
// Order: ChunkFunction per function_call (in upstream order), then one
// ChunkText if there is final output_text, then ChunkDone.
func (c *oneAPIClient) runNonStreaming(
	ctx context.Context,
	wire *httppkg.OpenAIResponsesReq,
	out chan<- StreamChunk,
) {
	defer close(out)

	deps := c.deps.UpstreamDeps
	deps.StreamSink = nil

	resp, _, err := upstreamCallFn(ctx, deps, wire)
	if err != nil {
		out <- StreamChunk{Kind: ChunkError, Text: err.Error(), Err: err}
		return
	}
	if resp == nil {
		out <- StreamChunk{Kind: ChunkDone}
		return
	}

	if resp.Error != nil {
		err := errors.Errorf("upstream responses error: %v", resp.Error)
		out <- StreamChunk{Kind: ChunkError, Text: err.Error(), Err: err}
		return
	}

	calls, _ := extractFunctionCallsFromResp(resp)
	for _, fc := range calls {
		out <- StreamChunk{
			Kind: ChunkFunction,
			FunctionCall: &FunctionCall{
				CallID:    fc.CallID,
				Name:      fc.Name,
				Arguments: json.RawMessage(fc.Arguments),
			},
		}
	}

	if text := finalOutputTextFromResp(resp); text != "" {
		out <- StreamChunk{Kind: ChunkText, Text: text}
	}

	out <- StreamChunk{Kind: ChunkDone}
}

// upstreamStreamFn is an indirection seam: tests override it with a fake
// transport that yields canned ResponsesRawEvents.
var upstreamStreamFn = httppkg.StreamUpstreamResponsesEventsCtx

// upstreamCallFn is the non-streaming counterpart of upstreamStreamFn.
var upstreamCallFn = httppkg.CallUpstreamResponsesCtx

// extractFunctionCallsFromResp reads function_call items from the final
// response. The OneAPI Responses surface exposes them in two places:
// required_action.submit_tool_outputs.tool_calls and the Output array.
// Both shapes are covered.
func extractFunctionCallsFromResp(resp *httppkg.OpenAIResponsesResp) ([]httppkg.OpenAIResponsesFunctionCall, error) {
	if resp == nil {
		return nil, errors.New("nil response")
	}
	if resp.RequiredAction != nil &&
		resp.RequiredAction.Type == "submit_tool_outputs" &&
		resp.RequiredAction.SubmitToolOutputs != nil {
		calls := make([]httppkg.OpenAIResponsesFunctionCall, 0,
			len(resp.RequiredAction.SubmitToolOutputs.ToolCalls))
		for _, tc := range resp.RequiredAction.SubmitToolOutputs.ToolCalls {
			name := strings.TrimSpace(tc.Function.Name)
			callID := strings.TrimSpace(tc.ID)
			if name == "" || callID == "" {
				continue
			}
			calls = append(calls, httppkg.OpenAIResponsesFunctionCall{
				Type:      "function_call",
				ID:        callID,
				CallID:    callID,
				Name:      name,
				Arguments: tc.Function.Arguments,
			})
		}
		if len(calls) > 0 {
			return calls, nil
		}
	}

	calls := make([]httppkg.OpenAIResponsesFunctionCall, 0)
	for _, item := range resp.Output {
		if item.Type != "function_call" {
			continue
		}
		var fc httppkg.OpenAIResponsesFunctionCall
		if err := json.Unmarshal(item.Raw(), &fc); err != nil {
			return nil, errors.Wrap(err, "unmarshal function_call")
		}
		if fc.CallID == "" || fc.Name == "" {
			continue
		}
		calls = append(calls, fc)
	}
	return calls, nil
}

// finalOutputTextFromResp returns the final assistant text from the
// response. Prefers OutputText when set; falls back to walking the
// message-typed Output items.
func finalOutputTextFromResp(resp *httppkg.OpenAIResponsesResp) string {
	if resp == nil {
		return ""
	}
	if strings.TrimSpace(resp.OutputText) != "" {
		return resp.OutputText
	}
	parts := make([]string, 0, 4)
	for _, item := range resp.Output {
		if item.Type != "message" {
			continue
		}
		var msg struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(item.Raw(), &msg); err != nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") {
			continue
		}
		for _, c := range msg.Content {
			if (c.Type == "output_text" || c.Type == "text") &&
				strings.TrimSpace(c.Text) != "" {
				parts = append(parts, c.Text)
			}
		}
	}
	return strings.Join(parts, "")
}

// extractReasoningText pulls the delta-or-text from a reasoning event. The
// Responses API splits reasoning into deltas with a `delta` field and
// .done events with nested `text` / `part.text`.
func extractReasoningText(ev httppkg.ResponsesRawEvent) string {
	if strings.TrimSpace(ev.Delta) != "" {
		return ev.Delta
	}
	var payload struct {
		Text string `json:"text"`
		Part struct {
			Text string `json:"text"`
		} `json:"part"`
	}
	if err := json.Unmarshal(ev.Raw, &payload); err != nil {
		return ""
	}
	if strings.TrimSpace(payload.Text) != "" {
		return payload.Text
	}
	return payload.Part.Text
}

// extractUsageFromRaw decodes a usage block (when the upstream attaches one
// to response.completed). Returns nil when no usage field is present.
func extractUsageFromRaw(raw []byte) *Usage {
	if len(raw) == 0 {
		return nil
	}
	var env struct {
		Response struct {
			Usage *struct {
				InputTokens         int `json:"input_tokens"`
				OutputTokens        int `json:"output_tokens"`
				TotalTokens         int `json:"total_tokens"`
				OutputTokensDetails struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"output_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Response.Usage == nil {
		return nil
	}
	u := env.Response.Usage
	return &Usage{
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
		ReasoningTokens: u.OutputTokensDetails.ReasoningTokens,
		Total:           u.TotalTokens,
	}
}

