package http

import (
	"context"
	"time"

	gutils "github.com/Laisky/go-utils/v5"
)

var (
	// dataReg = regexp.MustCompile(`^data: (.*)$`)
	// llmRespCache cache llm response to quick response
	llmRespCache = gutils.NewExpCache[string](context.Background(), time.Second*3)
)

// func sendAndParseChat(ctx *gin.Context) (toolCalls []OpenaiCompletionStreamRespToolCall) {
// 	logger := gmw.GetLogger(ctx)
// 	frontReq, openaiReq, err := convert2OpenaiRequest(ctx) //nolint:bodyclose
// 	if web.AbortErr(ctx, err) {
// 		return
// 	}

// 	reservation := getTokenReservation(ctx)
// 	actualOutputTokens := 0
// 	defer func() {
// 		if reservation != nil {
// 			if err := reservation.Finalize(gmw.Ctx(ctx), actualOutputTokens); err != nil {
// 				logger.Warn("finalize token reservation", zap.Error(err))
// 			}
// 		}
// 		clearTokenReservation(ctx)
// 	}()

// 	// read cache
// 	if frontReq != nil && len(frontReq.Messages) > 0 {
// 		if cacheKey, err := req2CacheKey(frontReq); err != nil {
// 			logger.Warn("marshal req for cache key", zap.Error(err))
// 		} else if respContent, ok := llmRespCache.Load(cacheKey); ok {
// 			res := &OpenaiCompletionStreamResp{
// 				Choices: []OpenaiCompletionStreamRespChoice{
// 					{
// 						Delta: OpenaiCompletionStreamRespDelta{
// 							Content: respContent,
// 						},
// 						FinishReason: "stop",
// 					},
// 				},
// 			}
// 			if data, err := json.Marshal(res); err != nil {
// 				logger.Warn("marshal resp", zap.Error(err))
// 			} else {
// 				data = append([]byte("data: "), data...)
// 				data = append(data, []byte("\n\n")...)

// 				if _, err = io.Copy(ctx.Writer, bytes.NewReader(data)); err != nil {
// 					logger.Warn("resp from cache", zap.Error(err))
// 				} else {
// 					logger.Debug("hit cache for llm response")
// 					tokens := CountTextTokens(respContent)
// 					if tokens == 0 && reservation != nil {
// 						tokens = reservation.EstimatedOutputTokens()
// 					}
// 					actualOutputTokens = tokens
// 					return
// 				}
// 			}
// 		}
// 	}

// 	// send request to openai
// 	logger.Debug("try send request to upstream server",
// 		zap.String("url", openaiReq.URL.String()))
// 	resp, err := httpcli.Do(openaiReq) //nolint: bodyclose
// 	if web.AbortErr(ctx, err) {
// 		return
// 	}
// 	defer gutils.LogErr(resp.Body.Close, logger)

// 	if resp.StatusCode != http.StatusOK {
// 		body, _ := io.ReadAll(resp.Body)
// 		modelName := "<unknown>"
// 		if frontReq != nil && frontReq.Model != "" {
// 			modelName = frontReq.Model
// 		}
// 		web.AbortErr(ctx, errors.Errorf("request model %q got [%d]%s",
// 			modelName, resp.StatusCode, string(body)))
// 		return
// 	}

// 	CopyHeader(ctx.Writer.Header(), resp.Header)
// 	ctx.Header("Access-Control-Expose-Headers", "x-oneapi-request-id, x-request-id")
// 	isStream := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

// 	// heartbeat should be enabled after header is set
// 	if isStream {
// 		enableHeartBeatForStreamReq(ctx)
// 	}

// 	if !isStream {
// 		bodyBytes, readErr := io.ReadAll(resp.Body)
// 		if web.AbortErr(ctx, readErr) {
// 			return
// 		}

// 		if _, writeErr := ctx.Writer.Write(bodyBytes); web.AbortErr(ctx, writeErr) {
// 			actualOutputTokens = 0
// 			return
// 		}

// 		if tokens, ok := tryExtractCompletionTokens(bodyBytes); ok {
// 			actualOutputTokens = tokens
// 		} else if reservation != nil {
// 			actualOutputTokens = reservation.EstimatedOutputTokens()
// 		}

// 		return
// 	}

// 	bodyReader := resp.Body
// 	reader := bufio.NewScanner(bodyReader)

// 	buf := make([]byte, 0, 10*1024*1024)
// 	reader.Buffer(buf, len(buf))

// 	reader.Split(bufio.ScanLines)

// 	var respContent string
// 	var lastResp *OpenaiCompletionStreamResp
// 	var line []byte
// 	for reader.Scan() {
// 		line = bytes.TrimSpace(reader.Bytes())
// 		// logger.Debug("got response line", zap.ByteString("line", line)) // debug only

// 		if len(line) == 0 {
// 			continue
// 		}

// 		var chunk []byte
// 		if matched := dataReg.FindAllSubmatch(line, -1); len(matched) != 0 {
// 			chunk = matched[0][1]
// 		} else {
// 			logger.Warn("unsupport resp line", zap.ByteString("line", line))
// 			continue
// 		}
// 		if len(chunk) == 0 {
// 			logger.Debug("empty chunk")
// 			continue
// 		}

// 		if err := gmw.CtxLock(ctx); err != nil {
// 			web.AbortErr(ctx, errors.Wrap(err, "failed to lock context"))
// 			return
// 		}
// 		_, err = io.Copy(ctx.Writer, bytes.NewReader(append(line, []byte("\n\n")...)))
// 		if err := gmw.CtxUnlock(ctx); err != nil {
// 			web.AbortErr(ctx, errors.Wrap(err, "failed to unlock context"))
// 			return
// 		}

// 		if web.AbortErr(ctx, err) {
// 			return
// 		}

// 		if bytes.Equal(chunk, []byte("[DONE]")) {
// 			logger.Debug("got [DONE]")
// 			lastResp = &OpenaiCompletionStreamResp{
// 				Choices: []OpenaiCompletionStreamRespChoice{
// 					{
// 						FinishReason: "[DONE]",
// 					},
// 				},
// 			}
// 			break
// 		}

// 		lastResp = new(OpenaiCompletionStreamResp)
// 		if err = json.Unmarshal(chunk, lastResp); err != nil {
// 			logger.Warn("unmarshal resp",
// 				zap.ByteString("line", line),
// 				zap.ByteString("chunk", chunk),
// 				zap.Error(err))
// 			continue
// 		}

// 		if len(lastResp.Choices) > 0 {
// 			if len(lastResp.Choices[0].Delta.ToolCalls) != 0 {
// 				tokens := CountTextTokens(string(chunk))
// 				if tokens == 0 && reservation != nil {
// 					tokens = reservation.EstimatedOutputTokens()
// 				}
// 				actualOutputTokens = tokens
// 				logger.Debug("got tool calls")
// 				toolCalls = append(toolCalls, lastResp.Choices[0].Delta.ToolCalls...)
// 			}

// 			switch v := lastResp.Choices[0].Delta.Content.(type) {
// 			case string:
// 				respContent += v
// 			}
// 		}

// 		// new oai api will return empty choices first
// 		if len(lastResp.Choices) == 0 {
// 			continue
// 		}

// 		// check if resp is end
// 		if !isStream ||
// 			len(lastResp.Choices) == 0 ||
// 			lastResp.Choices[0].FinishReason != "" {
// 			logger.Debug("got last resp",
// 				zap.Any("is_stream", isStream),
// 				zap.Int("choices", len(lastResp.Choices)),
// 				zap.String("finish_reason", lastResp.Choices[0].FinishReason),
// 			)
// 			break
// 		}
// 	}

// 	if web.AbortErr(ctx, reader.Err()) {
// 		return
// 	}

// 	if respContent != "" {
// 		actualOutputTokens = CountTextTokens(respContent)
// 		if actualOutputTokens == 0 && reservation != nil {
// 			actualOutputTokens = reservation.EstimatedOutputTokens()
// 		}
// 	} else if reservation != nil {
// 		actualOutputTokens = reservation.EstimatedOutputTokens()
// 	}

// 	if strings.ToLower(os.Getenv("DISABLE_LLM_CONSERVATION_AUDIT")) != "true" {
// 		if frontReq != nil && len(frontReq.Messages) > 0 && respContent != "" {
// 			go saveLLMConservation(frontReq, respContent)
// 		}
// 	}

// 	if lastResp == nil {
// 		web.AbortErr(ctx, errors.New("no response"))
// 		return
// 	}

// 	// scanner quit unexpected, write last line
// 	if len(lastResp.Choices) != 0 &&
// 		lastResp.Choices[0].FinishReason == "" {
// 		lastResp.Choices[0].FinishReason = "stop"
// 		lastResp.Choices[0].Delta.Content = " [TERMINATED UNEXPECTEDLY]"
// 		payload, err := json.MarshalToString(lastResp)
// 		if web.AbortErr(ctx, err) {
// 			return
// 		}

// 		_, err = io.Copy(ctx.Writer, strings.NewReader("\ndata: "+payload))
// 		if web.AbortErr(ctx, err) {
// 			return
// 		}
// 	} else if gutils.IsEmpty(lastResp) {
// 		return // bypass empty response
// 	} else if isStream || len(lastResp.Choices) == 0 || lastResp.Choices[0].FinishReason != "" {
// 		return // normal response
// 	} else {
// 		web.AbortErr(ctx, errors.Errorf("unsupport resp body %q", string(line)))
// 	}

// 	return nil
// }

func (r *FrontendReq) fillDefault() {
	r.MaxTokens = gutils.OptionalVal(&r.MaxTokens, 500)
	r.Temperature = gutils.OptionalVal(&r.Temperature, 1)
	r.TopP = gutils.OptionalVal(&r.TopP, 1)
	r.N = gutils.OptionalVal(&r.N, 1)
	r.Model = gutils.OptionalVal(&r.Model, ChatModel())
	// r.BestOf = gutils.OptionalVal(&r.BestOf, 1)
}
