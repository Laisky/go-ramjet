// Package http contains the gptchat HTTP handlers.
//
// This file (agent_bridge.go) hosts the gin-context-independent variants of
// the tool-dispatch and upstream-call helpers. They are used by the agent
// loop in agentx/ (see docs/proposals/2026-05-26-gptchat-react-agent-mode.md
// §3.2, §3.4 and §5.2).
//
// The existing `executeToolCall` and `callUpstreamResponses` wrappers
// preserve byte-for-byte SSE behavior of the proxy path; both delegate to
// the Ctx variants here.
package http

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/json"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// LegacyDeps captures the per-request inputs that ExecuteToolCallCtx needs.
//
// The gin-context-bound `executeToolCall` wrapper builds one of these from
// the request context. The agent loop (Phase 1B) constructs one directly.
//
// Fields:
//   - User: per-request user configuration (required).
//   - FrontendReq: the original frontend payload, used for MCP routing and
//     for capToolOutput's "summary context" hints (required).
//   - RawUserToken: the original Authorization-header token, used both for
//     freetier rate-limit gating and as the MCP fallback API key.
//   - Logger: per-request structured logger (required); the wrapper uses
//     gmw.GetLogger(ctx); the agent path threads its own.
type LegacyDeps struct {
	User         *config.UserConfig
	FrontendReq  *FrontendReq
	RawUserToken string
	Logger       glog.Logger
}

// UpstreamDeps captures the per-request inputs that CallUpstreamResponsesCtx
// needs to talk to the upstream Responses API.
//
// Fields:
//   - User: user config — used for API base + bearer token (required).
//   - Logger: structured logger (required).
//   - RequestHeader: headers from the inbound request to forward upstream
//     (typically `ctx.Request.Header`). Pass nil to send a clean header set.
//   - RawQuery: raw URL query string, appended to the upstream URL.
//   - StreamSink: when non-nil and `req.Stream` is true, every SSE chunk is
//     emitted to this sink (the `data: …\n\n` framed bytes, including the
//     trailing blank line). The gin wrapper supplies a sink that writes to
//     `ctx.ResponseWriter` under `gmw.CtxLock`. Agent callers supply their
//     own sink. When nil and `req.Stream` is true the helper degrades to
//     buffered non-streaming consumption.
//   - OnContentStreamed: optional callback fired the first time assistant
//     content is observed in the stream. The gin wrapper uses it to flip
//     the legacy `llm_response_content_streamed` flag.
//   - SetStreamHeaders: optional callback invoked once with the upstream
//     response headers right before streaming starts. The gin wrapper uses
//     it to set SSE response headers and the `x-oneapi-request-id` echo.
type UpstreamDeps struct {
	User              *config.UserConfig
	Logger            glog.Logger
	RequestHeader     http.Header
	RawQuery          string
	StreamSink        func([]byte) error
	OnContentStreamed func()
	SetStreamHeaders  func(http.Header)
}

// ExecuteToolCallCtx executes a single function call (local registry first,
// falling back to MCP) and returns the (possibly capped) output along with
// a short human-readable info string and an error.
//
// Behavior is identical to the historical `executeToolCall(*gin.Context, …)`;
// the only difference is that the inputs come from `deps` instead of being
// read out of gin.
func ExecuteToolCallCtx(
	ctx context.Context,
	deps LegacyDeps,
	fc OpenAIResponsesFunctionCall,
) (string, string, error) {
	user := deps.User
	frontendReq := deps.FrontendReq
	logger := deps.Logger

	// 1) Try local tools.
	if out, err := Call(fc.Name, fc.Arguments); err == nil {
		capped, changed, capErr := capToolOutput(ctx, user, frontendReq, fc.Name, fc.Arguments, out)
		info := "exec local tool: " + fc.Name
		if changed {
			info += " (output capped)"
		}
		if capErr != nil {
			info += " (cap warn: " + capErr.Error() + ")"
		}
		return capped, info, nil
	}

	// 2) Try MCP.
	if frontendReq.EnableMCP != nil && !*frontendReq.EnableMCP {
		if logger != nil {
			logger.Warn("prevent MCP tool call because it's disabled", zap.String("tool", fc.Name))
		}
		return "", "", errors.Errorf("MCP is disabled, but tool %q was called", fc.Name)
	}

	server := findMCPServerForToolName(frontendReq.MCPServers, fc.Name)
	if server == nil {
		return "", "", errors.Errorf("tool %q not found in enabled MCP servers", fc.Name)
	}

	// Rate limit MCP tools for freetier users.
	// Only applies to users whose API key begins with "FREETIER-".
	if strings.HasPrefix(deps.RawUserToken, "FREETIER-") {
		if expensiveModelRateLimiter == nil {
			onceLimiter.Do(setupRateLimiter)
		}
		ratelimitCost := config.Config.RateLimitExpensiveModelsIntervalSeconds
		if ratelimitCost <= 0 {
			ratelimitCost = 600
		}
		if !expensiveModelRateLimiter.AllowN(ratelimitCost) {
			return "", "", errors.New("MCP tools are rate limited for freetier users; please try again later")
		}
	}

	info := "exec MCP tool: " + fc.Name + " @ " + strings.TrimSpace(server.URL)

	// Use session API key as fallback when MCP server has no configured key.
	mcpOpts := &MCPCallOption{
		FallbackAPIKey: deps.RawUserToken,
	}
	out, err := callMCPTool(ctx, server, fc.Name, fc.Arguments, mcpOpts)
	if err != nil {
		return out, info, err
	}
	// Cap MCP output before passing to upstream.
	capped, changed, capErr := capToolOutput(ctx, user, frontendReq, fc.Name, fc.Arguments, out)
	if changed {
		info += " (output capped)"
	}
	if capErr != nil {
		info += " (cap warn: " + capErr.Error() + ")"
	}
	return capped, info, nil
}

// CallUpstreamResponsesCtx executes a Responses API request without depending
// on a `*gin.Context`. SSE streaming, when enabled, is delivered to
// `deps.StreamSink` as fully-framed `data: …\n\n` chunks.
//
// Headers on the inbound HTTP request are forwarded via `deps.RequestHeader`;
// the upstream URL is suffixed with `deps.RawQuery` when present.
func CallUpstreamResponsesCtx(
	ctx context.Context,
	deps UpstreamDeps,
	req *OpenAIResponsesReq,
) (*OpenAIResponsesResp, http.Header, error) {
	logger := deps.Logger
	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal responses req")
	}

	if logger != nil {
		logger.Debug("send responses request to upstream",
			zap.String("model", req.Model),
			zap.Int("payload_bytes", len(body)),
			zap.Any("request", sanitizePayloadForLog(req)),
		)
	}

	upReq, err := buildResponsesHTTPRequestCtx(ctx, deps, body)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build responses http request")
	}

	resp, err := httpcli.Do(upReq) //nolint:bodyclose
	if err != nil {
		return nil, nil, errors.Wrap(err, "do upstream request")
	}
	defer gutils.LogErr(resp.Body.Close, logger)

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, resp.Header, errors.Errorf(
			"upstream responses returned [%d] %s",
			resp.StatusCode,
			truncateBytesForLog(data, 2048),
		)
	}

	if req.Stream {
		if deps.SetStreamHeaders != nil {
			deps.SetStreamHeaders(resp.Header)
		}
		out, perr := parseStreamingResponsesViaSink(ctx, deps, resp)
		return out, resp.Header, perr
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, errors.Wrap(err, "read upstream responses")
	}

	out := new(OpenAIResponsesResp)
	if err := json.Unmarshal(data, out); err != nil {
		return nil, resp.Header, errors.Wrapf(
			err,
			"unmarshal upstream responses: %s",
			truncateBytesForLog(data, 2048),
		)
	}

	// Safe debug log: only shapes/lengths, no raw content.
	if logger != nil {
		types := make([]string, 0, len(out.Output))
		for _, it := range out.Output {
			if it.Type != "" {
				types = append(types, it.Type)
			}
		}
		raType := ""
		if out.RequiredAction != nil {
			raType = out.RequiredAction.Type
		}
		logger.Debug("upstream responses received",
			zap.String("id", out.ID),
			zap.Int("output_items", len(out.Output)),
			zap.Strings("output_types", types),
			zap.Int("output_text_len", len(out.OutputText)),
			zap.String("required_action", raType),
		)
	}

	if out.Error != nil {
		return nil, resp.Header, errors.Errorf("upstream responses error: %v", out.Error)
	}

	return out, resp.Header, nil
}

// buildResponsesHTTPRequestCtx mirrors buildResponsesHTTPRequest but takes the
// inputs from UpstreamDeps instead of a `*gin.Context`.
func buildResponsesHTTPRequestCtx(
	ctx context.Context,
	deps UpstreamDeps,
	reqBody []byte,
) (*http.Request, error) {
	if deps.User == nil {
		return nil, errors.New("nil user in UpstreamDeps")
	}
	newURL := strings.TrimRight(deps.User.APIBase, "/") + "/v1/responses"
	if deps.RawQuery != "" {
		newURL += "?" + deps.RawQuery
	}
	// Validate target URL form to keep parity with the gin path (http.NewRequest
	// would otherwise silently accept e.g. an empty host).
	if _, perr := url.Parse(newURL); perr != nil {
		return nil, errors.Wrap(perr, "parse upstream url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, newURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}

	if deps.RequestHeader != nil {
		CopyHeader(req.Header, deps.RequestHeader)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+deps.User.OpenaiToken)
	req.Header.Del("Accept-Encoding")
	return req, nil
}

// parseStreamingResponsesViaSink mirrors parseStreamingResponses but writes
// every SSE chunk via deps.StreamSink instead of directly to a gin writer.
//
// This function is the single source of truth for the streaming wire format;
// the gin path's parseStreamingResponses simply wires StreamSink to the gin
// writer and delegates here. That structure guarantees byte-identical
// behavior between the two paths.
func parseStreamingResponsesViaSink(
	ctx context.Context,
	deps UpstreamDeps,
	resp *http.Response,
) (*OpenAIResponsesResp, error) {
	_ = ctx
	logger := deps.Logger
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, responsesStreamMaxLineBytes)
	finalResp := new(OpenAIResponsesResp)
	var contentBuf strings.Builder
	streamedContent := false
	loggedChatFallback := false

	requestID := resp.Header.Get("x-oneapi-request-id")
	if requestID == "" {
		requestID = resp.Header.Get("x-request-id")
	}

	emitText := func(text string) {
		if text == "" || deps.StreamSink == nil {
			return
		}
		_ = WriteChatCompletionChunkToSink(deps.StreamSink, OpenaiCompletionStreamResp{
			ID: requestID,
			Choices: []OpenaiCompletionStreamRespChoice{{
				Delta: OpenaiCompletionStreamRespDelta{
					Role:    OpenaiMessageRoleAI,
					Content: text,
				},
				Index:        0,
				FinishReason: "",
			}},
		})
	}
	emitThinking := func(text string) {
		if text == "" || deps.StreamSink == nil {
			return
		}
		_ = WriteChatCompletionChunkToSink(deps.StreamSink, OpenaiCompletionStreamResp{
			ID: requestID,
			Choices: []OpenaiCompletionStreamRespChoice{{
				Delta: OpenaiCompletionStreamRespDelta{
					Role:             OpenaiMessageRoleAI,
					ReasoningContent: text,
				},
				Index:        0,
				FinishReason: "",
			}},
		})
	}
	markStreamed := func() {
		if deps.OnContentStreamed != nil {
			deps.OnContentStreamed()
		}
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		data := bytes.TrimPrefix(line, []byte("data: "))
		if bytes.Equal(data, []byte("[DONE]")) {
			break
		}

		var event struct {
			Type       string               `json:"type"`
			ResponseID string               `json:"response_id"`
			Delta      string               `json:"delta"`
			Response   *OpenAIResponsesResp `json:"response"`
			Error      any                  `json:"error"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			if logger != nil {
				logger.Warn("unmarshal responses event", zap.Error(err), zap.ByteString("data", data))
			}
			continue
		}

		if event.Error != nil {
			return nil, errors.Errorf("upstream responses error: %v", event.Error)
		}

		if event.ResponseID != "" {
			requestID = event.ResponseID
		}

		if event.Type == "" && event.Delta == "" && event.Response == nil {
			var chunk OpenaiCompletionStreamResp
			if err := json.Unmarshal(data, &chunk); err == nil && chunk.Object == "chat.completion.chunk" {
				if !loggedChatFallback && logger != nil {
					logger.Debug("responses stream received chat completion chunks",
						zap.String("request_id", requestID),
					)
					loggedChatFallback = true
				}
				if chunk.ID != "" {
					requestID = chunk.ID
				}
				for _, choice := range chunk.Choices {
					content, reasoning := extractChatCompletionDeltaContent(choice.Delta)
					if reasoning != "" {
						emitThinking(reasoning)
					}
					if content != "" {
						emitText(content)
						markStreamed()
						streamedContent = true
						contentBuf.WriteString(content)
					}
				}
				continue
			}
		}

		switch event.Type {
		case "response.output_text.delta":
			emitText(event.Delta)
			markStreamed()
			streamedContent = true
			contentBuf.WriteString(event.Delta)
		case "response.refusal.delta":
			emitText("refusal: " + event.Delta)
			markStreamed()
			streamedContent = true
			contentBuf.WriteString("refusal: " + event.Delta)
		case "response.reasoning_text.delta",
			"response.reasoning_text.done",
			"response.reasoning.delta",
			"response.reasoning_summary_text.delta",
			"response.reasoning_summary_text.done",
			"response.reasoning_summary_part.added",
			"response.reasoning_summary_part.done",
			"response.thought.delta",
			"response.thought.done":
			thinkingText := extractResponsesReasoningEventText(data, event)
			if thinkingText == "" {
				if logger != nil {
					logger.Debug("responses reasoning event had no text",
						zap.String("event_type", event.Type),
						zap.String("request_id", requestID),
					)
				}
				continue
			}
			emitThinking(thinkingText)
		case "response.function_call_arguments.delta":
			emitThinking(event.Delta)
		case "response.completed":
			if event.Response != nil {
				finalResp = event.Response
				imageMarkdown := extractOutputImageMarkdownFromResponses(event.Response)
				if !streamedContent {
					finalContent := extractOutputTextFromResponses(event.Response)
					if finalContent != "" {
						emitText(finalContent)
						markStreamed()
						streamedContent = true
						contentBuf.WriteString(finalContent)
						if logger != nil {
							logger.Debug("responses stream emitted final content fallback",
								zap.Int("chars", len(finalContent)),
								zap.String("request_id", requestID),
							)
						}
					}
				} else if imageMarkdown != "" {
					emitText(imageMarkdown)
					merged := appendMarkdownBlock(contentBuf.String(), imageMarkdown)
					contentBuf.Reset()
					contentBuf.WriteString(merged)
				}

				if imageMarkdown != "" && logger != nil {
					logger.Debug("responses stream output images",
						zap.Int("image_count", countMarkdownImages(imageMarkdown)),
						zap.Int("markdown_chars", len(imageMarkdown)),
						zap.String("request_id", requestID),
					)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if logger != nil {
			logger.Debug("responses stream scanner error",
				zap.Error(err),
				zap.Int("max_line_bytes", responsesStreamMaxLineBytes),
				zap.String("request_id", requestID),
			)
		}
		return nil, errors.Wrap(err, "scanner error")
	}

	if finalResp.ID == "" {
		finalResp.ID = requestID
	}
	if strings.TrimSpace(finalResp.OutputText) == "" && contentBuf.Len() > 0 {
		finalResp.OutputText = contentBuf.String()
	}

	return finalResp, nil
}

// WriteChatCompletionChunkToSink marshals a chunk into the SSE wire format
// (`data: <json>\n\n`) and emits it through `sink` as a single byte slice.
//
// The gin path's writeChatCompletionChunk emits the same three byte sequences
// in order under a write lock; the concatenated bytes seen by the underlying
// connection are identical to what we produce here. That parity is what the
// proxy-invariance test (TestCallUpstreamResponsesCtx_Streaming_ByteIdentical)
// verifies.
//
// Exported so the agentx package (out-of-tree from http) can compose
// emit-function wrappers around the agent loop's SSE writer without
// re-implementing the data:-prefix + flush byte sequence.
func WriteChatCompletionChunkToSink(sink func([]byte) error, chunk OpenaiCompletionStreamResp) error {
	if sink == nil {
		return nil
	}
	data, err := json.Marshal(chunk)
	if err != nil {
		return errors.Wrap(err, "marshal stream chunk")
	}
	buf := make([]byte, 0, len(data)+8)
	buf = append(buf, []byte("data: ")...)
	buf = append(buf, data...)
	buf = append(buf, '\n', '\n')
	return sink(buf)
}

// GinStreamSink returns a StreamSink that writes the given pre-framed SSE bytes
// to the gin response writer under gmw.CtxLock, flushing after every chunk.
// The byte sequence is identical to what writeChatCompletionChunk produces.
//
// Exported so the agentx handler package (out-of-tree from http) can hand
// the same SSE-writer surface to its sse.Writer adapter without
// duplicating the locking discipline.
func GinStreamSink(ctx *gin.Context) func([]byte) error {
	return func(b []byte) error {
		if err := gmw.CtxLock(ctx); err != nil {
			return errors.Wrap(err, "lock ctx")
		}
		_, werr := ctx.Writer.Write(b)
		if flush, ok := ctx.Writer.(http.Flusher); ok {
			flush.Flush()
		}
		if unlockErr := gmw.CtxUnlock(ctx); unlockErr != nil && werr == nil {
			return errors.Wrap(unlockErr, "unlock ctx")
		}
		return werr
	}
}

// ResponsesRawEvent is one typed SSE event surfaced from the upstream
// Responses API stream. It is the unit of work that StreamUpstreamResponsesEventsCtx
// delivers to its handler.
//
// The fields mirror the header that parseStreamingResponsesViaSink decodes
// inline; sharing this type lets the model adapter consume the same event
// taxonomy without re-implementing the SSE byte-framing or the data:-prefix
// scan loop.
//
// The Raw field is the verbatim payload that followed `data: ` (without the
// trailing newlines). It is reused across iterations of the scanner — copy
// it before retaining beyond the handler call.
type ResponsesRawEvent struct {
	Type       string
	ResponseID string
	Delta      string
	Response   *OpenAIResponsesResp
	// Raw holds the verbatim JSON bytes after the `data: ` prefix. Callers
	// that need to unmarshal nested fields (e.g. function_call items, usage
	// summaries, reasoning part text) can do so from Raw.
	Raw []byte
}

// StreamUpstreamResponsesEventsCtx executes a Responses API request in
// streaming mode and delivers each parsed SSE event to handler.
//
// This is the typed-event counterpart of CallUpstreamResponsesCtx — same
// HTTP path, same upstream URL, same headers/auth, same scanner buffer
// size, same `[DONE]` sentinel. The only difference is that events are
// delivered as typed ResponsesRawEvent rather than framed
// `data: <chat-chunk>` bytes.
//
// Used by internal/tasks/gptchat/agentx/model so the agent loop can consume
// typed deltas without re-parsing the wire bytes. The existing proxy path
// continues to use CallUpstreamResponsesCtx unchanged.
//
// Behavior:
//
//   - The handler is invoked synchronously per SSE event, in upstream order.
//     Returning an error aborts the stream and propagates the error out.
//   - The `response.completed` event populates the returned
//     OpenAIResponsesResp; if no such event arrives, the returned value
//     has only the response_id observed in the stream header.
//   - Upstream `error` events are surfaced as `errors.Errorf` returns and
//     terminate the stream.
//   - deps.StreamSink is intentionally ignored — callers that also want the
//     framed-chunk path should drive it themselves from the handler.
func StreamUpstreamResponsesEventsCtx(
	ctx context.Context,
	deps UpstreamDeps,
	req *OpenAIResponsesReq,
	handler func(ResponsesRawEvent) error,
) (*OpenAIResponsesResp, http.Header, error) {
	if handler == nil {
		return nil, nil, errors.New("nil event handler")
	}
	if req == nil {
		return nil, nil, errors.New("nil responses request")
	}
	// Force the stream flag — the function name promises streaming.
	req.Stream = true

	logger := deps.Logger
	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal responses req")
	}

	if logger != nil {
		logger.Debug("send streaming responses request to upstream",
			zap.String("model", req.Model),
			zap.Int("payload_bytes", len(body)),
		)
	}

	upReq, err := buildResponsesHTTPRequestCtx(ctx, deps, body)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build responses http request")
	}

	resp, err := httpcli.Do(upReq) //nolint:bodyclose
	if err != nil {
		return nil, nil, errors.Wrap(err, "do upstream request")
	}
	defer gutils.LogErr(resp.Body.Close, logger)

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, resp.Header, errors.Errorf(
			"upstream responses returned [%d] %s",
			resp.StatusCode,
			truncateBytesForLog(data, 2048),
		)
	}

	if deps.SetStreamHeaders != nil {
		deps.SetStreamHeaders(resp.Header)
	}

	final, perr := parseStreamingResponsesIntoHandler(ctx, deps, resp, handler)
	return final, resp.Header, perr
}

// parseStreamingResponsesIntoHandler is the typed-event mirror of
// parseStreamingResponsesViaSink. Both functions read SSE lines from the
// same HTTP response body shape and decode the same event header; they
// diverge only in how they surface the events (framed chunks vs typed
// callback).
//
// Kept package-private because callers should invoke the higher-level
// StreamUpstreamResponsesEventsCtx which handles the HTTP plumbing.
func parseStreamingResponsesIntoHandler(
	ctx context.Context,
	deps UpstreamDeps,
	resp *http.Response,
	handler func(ResponsesRawEvent) error,
) (*OpenAIResponsesResp, error) {
	logger := deps.Logger
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, responsesStreamMaxLineBytes)

	finalResp := new(OpenAIResponsesResp)
	requestID := resp.Header.Get("x-oneapi-request-id")
	if requestID == "" {
		requestID = resp.Header.Get("x-request-id")
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return finalResp, err
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		data := bytes.TrimPrefix(line, []byte("data: "))
		if bytes.Equal(data, []byte("[DONE]")) {
			break
		}

		var event struct {
			Type       string               `json:"type"`
			ResponseID string               `json:"response_id"`
			Delta      string               `json:"delta"`
			Response   *OpenAIResponsesResp `json:"response"`
			Error      any                  `json:"error"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			if logger != nil {
				logger.Warn("unmarshal responses event",
					zap.Error(err),
					zap.ByteString("data", truncateBytesForLogBytes(data, 1024)),
				)
			}
			continue
		}

		if event.Error != nil {
			return finalResp, errors.Errorf("upstream responses error: %v", event.Error)
		}
		if event.ResponseID != "" {
			requestID = event.ResponseID
		}
		if event.Type == "response.completed" && event.Response != nil {
			finalResp = event.Response
		}

		// Copy data — the scanner reuses its buffer between iterations,
		// so the handler must not retain the original slice.
		rawCopy := make([]byte, len(data))
		copy(rawCopy, data)
		if err := handler(ResponsesRawEvent{
			Type:       event.Type,
			ResponseID: event.ResponseID,
			Delta:      event.Delta,
			Response:   event.Response,
			Raw:        rawCopy,
		}); err != nil {
			return finalResp, err
		}
	}

	if err := scanner.Err(); err != nil {
		return finalResp, errors.Wrap(err, "scanner error")
	}

	if finalResp.ID == "" {
		finalResp.ID = requestID
	}

	return finalResp, nil
}

// truncateBytesForLogBytes truncates raw bytes into a safe ByteString slice
// for logging. It avoids huge log lines and excessive allocation.
func truncateBytesForLogBytes(b []byte, maxBytes int) []byte {
	if maxBytes <= 0 {
		maxBytes = 1024
	}
	if len(b) <= maxBytes {
		return b
	}
	return b[:maxBytes]
}
