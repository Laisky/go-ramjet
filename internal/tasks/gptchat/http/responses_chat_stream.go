package http

import (
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
)

// writeFinalToUI writes the final assistant answer to the client, either as SSE
// stream frames ending with [DONE] or as one chat.completion JSON body.
//
// Parameters:
//   - ctx: Gin context of the chat request.
//   - frontendReq: Parsed frontend request; Stream selects the output format.
//   - upstreamHeader: Upstream response headers carrying the request id, if any.
//   - finalText: Final answer text.
//   - reasoningText: Reasoning text for the non-stream body.
//   - thinkingSteps: Intermediate steps replayed as reasoning deltas when the
//     answer was not streamed already.
//
// It returns an error when the request is missing or the JSON body cannot be
// marshaled or written.
func writeFinalToUI(
	ctx *gin.Context,
	frontendReq *FrontendReq,
	upstreamHeader http.Header,
	finalText string,
	reasoningText string,
	thinkingSteps []string,
) error {
	logger := gmw.GetLogger(ctx)
	if frontendReq == nil {
		return errors.New("empty frontend request")
	}

	requestID := ""
	if upstreamHeader != nil {
		requestID = upstreamHeader.Get("x-oneapi-request-id")
		if requestID == "" {
			requestID = upstreamHeader.Get("x-request-id")
		}
	}

	if frontendReq.Stream {
		setStreamHeaders(ctx, upstreamHeader)
		stopHeartbeat := enableHeartBeatForStreamReq(ctx)
		defer stopHeartbeat()

		if !ctx.GetBool("llm_response_streamed") || !responseContentStreamed(ctx) {
			if ctx.GetBool("llm_response_streamed") && !responseContentStreamed(ctx) {
				logger.Debug("streamed response missing content, emitting final fallback",
					zap.Int("chars", len(finalText)),
					zap.String("request_id", requestID),
				)
			}
			for _, s := range thinkingSteps {
				emitThinkingDelta(ctx, true, requestID, s)
			}
			for _, chunk := range chunkString(finalText, 512) {
				_ = writeChatCompletionChunk(ctx, OpenaiCompletionStreamResp{
					ID: requestID,
					Choices: []OpenaiCompletionStreamRespChoice{{
						Delta: OpenaiCompletionStreamRespDelta{
							Role:    OpenaiMessageRoleAI,
							Content: chunk,
						},
						Index:        0,
						FinishReason: "",
					}},
				})
			}
		}

		_ = writeChatCompletionChunk(ctx, OpenaiCompletionStreamResp{
			ID: requestID,
			Choices: []OpenaiCompletionStreamRespChoice{{
				Delta:        OpenaiCompletionStreamRespDelta{Role: OpenaiMessageRoleAI},
				Index:        0,
				FinishReason: "stop",
			}},
		})
		// No heartbeat may follow the terminal frame or outlive the handler.
		stopHeartbeat()
		_, _ = io.WriteString(ctx.Writer, "data: [DONE]\n\n")
		return nil
	}

	out := &OpenaiCompletionResp{
		ID:     requestID,
		Object: "chat.completion",
		Model:  frontendReq.Model,
		Choices: []struct {
			Message struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
			Index        int    `json:"index"`
		}{{
			Message: struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content,omitempty"`
			}{Role: "assistant", Content: finalText, ReasoningContent: reasoningText},
			FinishReason: "stop",
			Index:        0,
		}},
	}

	data, err := json.Marshal(out)
	if err != nil {
		return errors.Wrap(err, "marshal completion")
	}
	ctx.Header("content-type", "application/json")
	_, err = ctx.Writer.Write(data)
	return err
}

func setStreamHeaders(ctx *gin.Context, upstreamHeader http.Header) {
	ctx.Header("content-type", "text/event-stream")
	ctx.Header("cache-control", "no-cache")
	ctx.Header("connection", "keep-alive")
	ctx.Header("Access-Control-Expose-Headers", "x-oneapi-request-id, x-request-id")

	// Preserve request id for cost display. Must be set before the first write.
	if upstreamHeader != nil {
		if rid := upstreamHeader.Get("x-oneapi-request-id"); rid != "" {
			ctx.Header("x-oneapi-request-id", rid)
		}
		if rid := upstreamHeader.Get("x-request-id"); rid != "" && ctx.Writer.Header().Get("x-oneapi-request-id") == "" {
			ctx.Header("x-oneapi-request-id", rid)
		}
	}
}

func emitThinkingDelta(ctx *gin.Context, isStream bool, requestID, text string) {
	if !isStream {
		return
	}
	_ = writeChatCompletionChunk(ctx, OpenaiCompletionStreamResp{
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

func emitTextDelta(ctx *gin.Context, isStream bool, requestID, text string) {
	if !isStream {
		return
	}
	_ = writeChatCompletionChunk(ctx, OpenaiCompletionStreamResp{
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

func chunkString(s string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	out := make([]string, 0, (len(s)/n)+1)
	for len(s) > 0 {
		if len(s) <= n {
			out = append(out, s)
			break
		}
		out = append(out, s[:n])
		s = s[n:]
	}
	return out
}

func writeChatCompletionChunk(ctx *gin.Context, chunk OpenaiCompletionStreamResp) error {
	data, err := json.Marshal(chunk)
	if err != nil {
		return errors.Wrap(err, "marshal stream chunk")
	}
	if err := gmw.CtxLock(ctx); err != nil {
		return errors.Wrap(err, "lock ctx")
	}
	_, werr := ctx.Writer.Write([]byte("data: "))
	if werr == nil {
		_, werr = ctx.Writer.Write(data)
	}
	if werr == nil {
		_, werr = ctx.Writer.Write([]byte("\n\n"))
	}
	if flush, ok := ctx.Writer.(http.Flusher); ok {
		flush.Flush()
	}
	if unlockErr := gmw.CtxUnlock(ctx); unlockErr != nil {
		return errors.Wrap(unlockErr, "unlock ctx")
	}
	return werr
}
