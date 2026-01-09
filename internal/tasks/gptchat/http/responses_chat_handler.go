package http

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/web"
)

// defaultToolLoopMaxRounds defines the default maximum number of tool loop rounds.
const defaultToolLoopMaxRounds = 5

const toolStepMarker = "[[TOOLS]] "

// ChatHandler handles the web UI chat endpoint.
//
// It always talks to upstream using the OpenAI Responses API schema, injects enabled tools,
// executes tool calls (including MCP), and returns only the final assistant answer to the UI.
// Intermediate steps are streamed via delta.reasoning_content so the UI can render them
// inside the collapsible Thinking panel.
func ChatHandler(ctx *gin.Context) {
	_ = sendChatWithResponsesToolLoop(ctx)
}

func sendChatWithResponsesToolLoop(ctx *gin.Context) error {
	logger := gmw.GetLogger(ctx)
	frontendReq, user, responsesReq, err := convert2UpstreamResponsesRequest(ctx)
	if web.AbortErr(ctx, err) {
		return err
	}

	// ---------------------------------------------------------
	// Special flow: Image generation
	// If the user selected an image model but hit the /api endpoint
	// (e.g. via regeneration or edit), route to image logic.
	// ---------------------------------------------------------
	if isImageModel(frontendReq.Model) {
		logger.Debug("routing image model request to image generation logic",
			zap.String("model", frontendReq.Model))

		// Extract prompt from last user message
		var prompt string
		for i := len(frontendReq.Messages) - 1; i >= 0; i-- {
			if frontendReq.Messages[i].Role == OpenaiMessageRoleUser {
				prompt = frontendReq.Messages[i].Content.String()
				break
			}
		}

		if prompt == "" {
			err := errors.New("prompt is empty for image generation")
			web.AbortErr(ctx, err)
			return err
		}

		if user.EnableExternalImageBilling {
			if err := checkUserExternalBilling(gmw.Ctx(ctx),
				user, GetImageModelPrice(frontendReq.Model), "txt2image"); web.AbortErr(ctx, err) {
				return err
			}
		}

		taskID := gutils.RandomStringWithLength(36)
		taskCtx, cancel := context.WithTimeout(gmw.Ctx(ctx), time.Minute*5)
		defer cancel()

		if frontendReq.N <= 0 {
			frontendReq.N = 1
		}

		var imgContents [][]byte
		switch {
		case strings.Contains(user.ImageUrl, "openai.azure.com"):
			var pool errgroup.Group
			imgContents = make([][]byte, frontendReq.N)
			for i := range frontendReq.N {
				i := i
				pool.Go(func() (err error) {
					imgContents[i], err = fetchImageFromAzureDalle(taskCtx, user, prompt)
					return err
				})
			}
			if err := pool.Wait(); web.AbortErr(ctx, err) {
				return err
			}
		default:
			imgContents, err = fetchImageFromOpenaiDalle(taskCtx, user, frontendReq.Model, prompt, frontendReq.N, "")
			if web.AbortErr(ctx, err) {
				return err
			}
		}

		var pool errgroup.Group
		for i, imgContent := range imgContents {
			i, imgContent := i, imgContent
			pool.Go(func() error {
				return uploadImage2Minio(taskCtx,
					fmt.Sprintf("%s-%d", drawImageByTxtObjkeyPrefix(taskID), i),
					prompt,
					imgContent,
					".png",
				)
			})
		}

		if err := pool.Wait(); web.AbortErr(ctx, err) {
			return err
		}

		var markdownText string
		for i := range imgContents {
			url := fmt.Sprintf("https://%s/%s/%s-%d.%s",
				config.Config.S3.Endpoint,
				config.Config.S3.Bucket,
				drawImageByTxtObjkeyPrefix(taskID), i, "png",
			)
			markdownText += fmt.Sprintf("![Image](%s)\n\n", url)
		}

		return writeFinalToUI(ctx, frontendReq, nil, strings.TrimSpace(markdownText), "", nil)
	}

	reservation := getTokenReservation(ctx)
	defer clearTokenReservation(ctx)

	// If MCP is enabled (api keys present), skip cache to avoid persisting secrets.
	cacheAllowed := true
	for _, srv := range frontendReq.MCPServers {
		if strings.TrimSpace(srv.APIKey) != "" {
			cacheAllowed = false
			break
		}
	}

	if cacheAllowed && frontendReq != nil && len(frontendReq.Messages) > 0 {
		if cacheKey, err := req2CacheKey(frontendReq); err == nil {
			if respContent, ok := llmRespCache.Load(cacheKey); ok {
				finalText := respContent
				if reservation != nil {
					_ = reservation.Finalize(gmw.Ctx(ctx), CountTextTokens(finalText))
				}
				clearTokenReservation(ctx)
				return writeFinalToUI(ctx, frontendReq, nil, finalText, "", nil)
			}
		}
	}

	// Synchronous tool loop; we stream only to the browser.
	inputItems, err := flattenResponsesInput(responsesReq.Input)
	if web.AbortErr(ctx, err) {
		return err
	}
	thinkingSteps := make([]string, 0, 8)
	var fullReasoning string

	var lastUpstreamHeader http.Header
	var finalText string
	lastCalls := 0
	maxRounds := defaultToolLoopMaxRounds
	if config.Config != nil && config.Config.ToolLoopMaxRounds > 0 {
		maxRounds = config.Config.ToolLoopMaxRounds
	}
	for round := 0; round < maxRounds+1; round++ {
		r := *responsesReq
		r.Input = inputItems

		resp, hdr, callErr := callUpstreamResponses(ctx, user, &r)
		lastUpstreamHeader = hdr
		if callErr != nil {
			web.AbortErr(ctx, callErr)
			return callErr
		}

		// Extract reasoning and emit it immediately for streaming.
		if reasoning := extractReasoningFromResponses(resp); reasoning != "" {
			fullReasoning += reasoning
			thinkingSteps = append(thinkingSteps, reasoning)
			if frontendReq.Stream {
				requestID := lastUpstreamHeader.Get("x-oneapi-request-id")
				if requestID == "" {
					requestID = lastUpstreamHeader.Get("x-request-id")
				}
				emitThinkingDelta(ctx, true, requestID, reasoning)
			}
		}

		calls, extractErr := extractFunctionCallsFromResponses(resp)
		if extractErr != nil {
			web.AbortErr(ctx, extractErr)
			return extractErr
		}
		lastCalls = len(calls)

		if len(calls) == 0 {
			finalText = extractOutputTextFromResponses(resp)
			break
		}

		if round == maxRounds {
			if finalText == "" {
				finalText = extractOutputTextFromResponses(resp)
			}
			break
		}

		// Buffer tool-call steps (streaming starts after headers are set).
		for _, fc := range calls {
			thinkingSteps = append(thinkingSteps, toolStepMarker+"Upstream tool_call: "+fc.Name+"\n")
			if strings.TrimSpace(fc.Arguments) != "" {
				thinkingSteps = append(thinkingSteps, toolStepMarker+"args: "+fc.Arguments+"\n")
			}

			var (
				toolOutput string
				execInfo   string
				toolErr    error
			)
			if round == maxRounds-1 {
				toolOutput = "Tool execution failed: maximum tool call rounds reached. Please summarize current results and respond to the user based on existing information."
				thinkingSteps = append(thinkingSteps, toolStepMarker+"tool loop limit reached; informing AI\n")
			} else {
				toolOutput, execInfo, toolErr = executeToolCall(ctx, user, frontendReq, fc)
				if execInfo != "" {
					thinkingSteps = append(thinkingSteps, toolStepMarker+execInfo+"\n")
				}
				if toolErr != nil {
					thinkingSteps = append(thinkingSteps, toolStepMarker+"tool error: "+toolErr.Error()+"\n")
					toolOutput = "Tool execution failed: " + toolErr.Error()
				} else {
					thinkingSteps = append(thinkingSteps, toolStepMarker+"tool ok\n")
				}
			}

			// Feed back to upstream.
			inputItems = append(inputItems, fc)
			inputItems = append(inputItems, OpenAIResponsesFunctionCallOutput{
				Type:   "function_call_output",
				CallID: fc.CallID,
				Output: toolOutput,
			})
		}
	}

	if finalText == "" {
		if lastCalls > 0 {
			thinkingSteps = append(thinkingSteps, toolStepMarker+"tool loop limit reached; returning partial result\n")
			finalText = "(tool loop limit reached)"
		} else {
			finalText = "(no output)"
		}
	}

	// Finalize quota based on actual output tokens.
	if reservation != nil {
		_ = reservation.Finalize(gmw.Ctx(ctx), CountTextTokens(finalText))
	}

	// Save to cache and audit log.
	if cacheAllowed && frontendReq != nil && len(frontendReq.Messages) > 0 {
		if cacheKey, err := req2CacheKey(frontendReq); err == nil {
			llmRespCache.Store(cacheKey, finalText)
		}
	}
	if strings.ToLower(os.Getenv("DISABLE_LLM_CONSERVATION_AUDIT")) != "true" {
		if frontendReq != nil && len(frontendReq.Messages) > 0 && finalText != "" {
			go saveLLMConservation(frontendReq, finalText, fullReasoning)
		}
	}

	logger.Debug("responses chat completed", zap.Int("chars", len(finalText)))
	return writeFinalToUI(ctx, frontendReq, lastUpstreamHeader, finalText, fullReasoning, thinkingSteps)
}

func writeFinalToUI(
	ctx *gin.Context,
	frontendReq *FrontendReq,
	upstreamHeader http.Header,
	finalText string,
	reasoningText string,
	thinkingSteps []string,
) error {
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
		enableHeartBeatForStreamReq(ctx)
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
		_ = writeChatCompletionChunk(ctx, OpenaiCompletionStreamResp{
			ID: requestID,
			Choices: []OpenaiCompletionStreamRespChoice{{
				Delta:        OpenaiCompletionStreamRespDelta{Role: OpenaiMessageRoleAI},
				Index:        0,
				FinishReason: "stop",
			}},
		})
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

func executeToolCall(
	ctx *gin.Context,
	user *config.UserConfig,
	frontendReq *FrontendReq,
	fc OpenAIResponsesFunctionCall,
) (string, string, error) {
	// 1) Try local tools.
	if out, err := Call(fc.Name, fc.Arguments); err == nil {
		capped, changed, capErr := capToolOutput(gmw.Ctx(ctx), user, frontendReq, fc.Name, fc.Arguments, out)
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
		gmw.GetLogger(ctx).Warn("prevent MCP tool call because it's disabled", zap.String("tool", fc.Name))
		return "", "", errors.Errorf("MCP is disabled, but tool %q was called", fc.Name)
	}

	server := findMCPServerForToolName(frontendReq.MCPServers, fc.Name)
	if server == nil {
		return "", "", errors.Errorf("tool %q not found in enabled MCP servers", fc.Name)
	}

	// Rate limit MCP tools for freetier users.
	// Only applies to users whose API key begins with "FREETIER-".
	if strings.HasPrefix(getRawUserToken(ctx), "FREETIER-") {
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

	// Use session API key as fallback when MCP server has no configured key
	mcpOpts := &MCPCallOption{
		FallbackAPIKey: getRawUserToken(ctx),
	}
	out, err := callMCPTool(gmw.Ctx(ctx), server, fc.Name, fc.Arguments, mcpOpts)
	if err != nil {
		return out, info, err
	}
	// Cap MCP output before passing to upstream.
	capped, changed, capErr := capToolOutput(gmw.Ctx(ctx), user, frontendReq, fc.Name, fc.Arguments, out)
	if changed {
		info += " (output capped)"
	}
	if capErr != nil {
		info += " (cap warn: " + capErr.Error() + ")"
	}
	return capped, info, nil
}

func findMCPServerForToolName(servers []MCPServerConfig, toolName string) *MCPServerConfig {
	name := strings.TrimSpace(toolName)
	if name == "" {
		return nil
	}
	for i := range servers {
		s := &servers[i]
		if !s.Enabled {
			continue
		}
		if len(s.Tools) == 0 {
			continue
		}
		for _, raw := range s.Tools {
			tn := extractToolNameFromDefinition(raw)
			if tn == name {
				return s
			}
		}
	}
	return nil
}

func extractToolNameFromDefinition(raw stdjson.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if v, ok := m["name"].(string); ok {
		return strings.TrimSpace(v)
	}
	if fn, ok := m["function"].(map[string]any); ok {
		if v, ok := fn["name"].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func flattenResponsesInput(in any) ([]any, error) {
	if in == nil {
		return []any{}, nil
	}
	if msgs, ok := in.([]OpenAIResponsesInputMessage); ok {
		out := make([]any, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, m)
		}
		return out, nil
	}
	if arr, ok := in.([]any); ok {
		return arr, nil
	}
	return nil, errors.Errorf("unsupported responses input type %T", in)
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

// convert2UpstreamResponsesRequest parses the frontend request, applies feature switches,
// reserves quota, and converts the request into the OpenAI Responses API schema.
func convert2UpstreamResponsesRequest(ctx *gin.Context) (*FrontendReq, *config.UserConfig, *OpenAIResponsesReq, error) {
	logger := gmw.GetLogger(ctx)
	var err error

	user, err := getUserByAuthHeader(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "get user")
	}

	frontendReq := &FrontendReq{}
	if gutils.Contains([]string{http.MethodPost, http.MethodPut}, ctx.Request.Method) {
		frontendReq, err = bodyChecker(ctx.Request.Body)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "request is illegal")
		}

		// enhance user query
		if config.Config.RamjetURL != "" &&
			frontendReq.LaiskyExtra != nil &&
			!frontendReq.LaiskyExtra.ChatSwitch.DisableHttpsCrawler {
			frontendReq.embeddingUrlContent(ctx, user)
		}

		if frontendReq.LaiskyExtra != nil &&
			frontendReq.LaiskyExtra.ChatSwitch.EnableGoogleSearch {
			if user.IsFree {
				ratelimitCost := gconfig.Shared.GetInt("openai.rate_limit_expensive_models_interval_secs")
				if !expensiveModelRateLimiter.AllowN(ratelimitCost) {
					return nil, nil, nil, errors.New("web search is limited for free users" +
						"you need upgrade to a paid membership to enable this feature unlimitedly, " +
						"more info at https://wiki.laisky.com/projects/gpt/pay/")
				}
			}
			frontendReq.embeddingGoogleSearch(ctx, user)
		}

		// never forward app-specific config to upstream.
		frontendReq.LaiskyExtra = nil

		if err := IsModelAllowed(ctx, user, frontendReq); err != nil {
			return nil, nil, nil, errors.Wrapf(err, "check is model allowed for user %q", user.UserName)
		}

		if frontendReq != nil && len(frontendReq.Messages) > 0 {
			reservation, reserveErr := ReserveTokens(ctx, user, frontendReq)
			if reserveErr != nil {
				return nil, nil, nil, errors.Wrap(reserveErr, "reserve token quota")
			}
			_ = reservation
		}

		if strings.HasPrefix(frontendReq.Model, "o") ||
			strings.HasPrefix(frontendReq.Model, "gpt-oss-") ||
			strings.HasPrefix(frontendReq.Model, "claude-") ||
			strings.HasPrefix(frontendReq.Model, "gpt-5") &&
				frontendReq.ReasoningEffort == "" {
			frontendReq.ReasoningEffort = "high"
		}
	}

	// Convert to Responses API request.
	responsesReq, err := convertFrontendToResponsesRequest(frontendReq)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "convert to responses request")
	}

	logger.Debug("prepared responses request",
		zap.String("model", responsesReq.Model),
		zap.Int("tools", len(responsesReq.Tools)),
		zap.Bool("enable_mcp", frontendReq.EnableMCP != nil && *frontendReq.EnableMCP),
		zap.Int("mcp_servers", len(frontendReq.MCPServers)),
	)

	return frontendReq, user, responsesReq, nil
}

// keep linter happy for build-conditional imports.
var _ = time.Second
