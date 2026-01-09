package http

import (
	"bufio"
	"bytes"
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// OpenAIResponsesTool defines a tool in the OpenAI Responses API schema.
//
// It intentionally mirrors the documented format:
// {"type":"function","name":"...","description":"...","parameters":{...},"strict":true}
type OpenAIResponsesTool struct {
	Type        string             `json:"type"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Parameters  stdjson.RawMessage `json:"parameters,omitempty"`
	Strict      *bool              `json:"strict,omitempty"`
}

// OpenAIResponsesReq is a subset of the OpenAI Responses API request schema.
type OpenAIResponsesReq struct {
	Model           string                   `json:"model"`
	Input           any                      `json:"input,omitempty"`
	MaxOutputTokens uint                     `json:"max_output_tokens,omitempty"`
	Reasoning       *OpenAIResponseReasoning `json:"reasoning,omitempty"` // Optional: Configuration options for reasoning models
	Stream          bool                     `json:"stream,omitempty"`
	Temperature     float64                  `json:"temperature,omitempty"`
	TopP            float64                  `json:"top_p,omitempty"`
	Tools           []OpenAIResponsesTool    `json:"tools,omitempty"`
	ToolChoice      any                      `json:"tool_choice,omitempty"`
	Store           *bool                    `json:"store,omitempty"`
}

// OpenAIResponseReasoning defines reasoning options for the Responses API.
type OpenAIResponseReasoning struct {
	// Effort defines the reasoning effort level
	Effort *string `json:"effort,omitempty" binding:"omitempty,oneof=low medium high"`
	// Summary defines whether to include a summary of the reasoning
	Summary *string `json:"summary,omitempty" binding:"omitempty,oneof=auto concise detailed"`
}

// OpenAIResponsesResp is a subset of the OpenAI Responses API response schema.
type OpenAIResponsesResp struct {
	ID             string                         `json:"id"`
	Output         []OpenAIResponsesItem          `json:"output"`
	OutputText     string                         `json:"output_text"`
	RequiredAction *OpenAIResponsesRequiredAction `json:"required_action,omitempty"`
	Error          map[string]any                 `json:"error,omitempty"`
	Metadata       map[string]string              `json:"metadata,omitempty"`
}

// OpenAIResponsesRequiredAction is a subset of Responses API required_action.
//
// OneAPI (and some upstreams) provide tool calls here even when output_text is empty.
type OpenAIResponsesRequiredAction struct {
	Type              string                            `json:"type"`
	SubmitToolOutputs *OpenAIResponsesSubmitToolOutputs `json:"submit_tool_outputs,omitempty"`
}

// OpenAIResponsesSubmitToolOutputs is the required_action payload.
type OpenAIResponsesSubmitToolOutputs struct {
	ToolCalls []OpenAIResponsesRequiredToolCall `json:"tool_calls"`
}

// OpenAIResponsesRequiredToolCall is a tool call descriptor inside required_action.
type OpenAIResponsesRequiredToolCall struct {
	ID       string                          `json:"id"`
	Type     string                          `json:"type"`
	Function OpenAIResponsesRequiredFunction `json:"function"`
}

// OpenAIResponsesRequiredFunction contains tool name/args.
type OpenAIResponsesRequiredFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIResponsesItem is a generic output item with a type discriminator.
//
// It keeps the raw JSON payload so we can unmarshal it into typed structs later.
type OpenAIResponsesItem struct {
	Type string `json:"type"`
	raw  stdjson.RawMessage
}

// UnmarshalJSON implements json unmarshalling and preserves the full raw payload.
func (i *OpenAIResponsesItem) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("nil OpenAIResponsesItem")
	}
	i.raw = append(i.raw[:0], data...)
	var aux struct {
		Type string `json:"type"`
	}
	if err := stdjson.Unmarshal(data, &aux); err != nil {
		return errors.Wrap(err, "unmarshal output item type")
	}
	i.Type = aux.Type
	return nil
}

// Raw returns the raw JSON payload for this output item.
func (i OpenAIResponsesItem) Raw() stdjson.RawMessage {
	return i.raw
}

// OpenAIResponsesFunctionCall is the Responses API function_call output item.
type OpenAIResponsesFunctionCall struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIResponsesFunctionCallOutput is the Responses API function_call_output input item.
type OpenAIResponsesFunctionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// OpenAIResponsesInputMessage is a Responses API input message item.
//
// In practice the API accepts {"role":"user","content":"..."} as shown in docs.
type OpenAIResponsesInputMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// convertFrontendToResponsesRequest builds an upstream OpenAI Responses API request from a frontend request.
func convertFrontendToResponsesRequest(frontendReq *FrontendReq) (*OpenAIResponsesReq, error) {
	if frontendReq == nil {
		return nil, errors.New("empty frontend request")
	}

	reasoningSummary := "auto"
	req := &OpenAIResponsesReq{
		Model:           frontendReq.Model,
		MaxOutputTokens: frontendReq.MaxTokens,
		Stream:          frontendReq.Stream,
		Temperature:     frontendReq.Temperature,
		TopP:            frontendReq.TopP,
		ToolChoice:      frontendReq.ToolChoice,
	}

	if frontendReq.ReasoningEffort != "" {
		req.Reasoning = &OpenAIResponseReasoning{
			Effort:  &frontendReq.ReasoningEffort,
			Summary: &reasoningSummary,
		}
	}

	if req.Model == "" {
		req.Model = ChatModel()
	}

	// Convert messages.
	msgs := make([]OpenAIResponsesInputMessage, 0, len(frontendReq.Messages))
	for _, m := range frontendReq.Messages {
		role := strings.TrimSpace(m.Role.String())
		if role == "" {
			role = "user"
		}

		var content []any
		if len(m.Content.ArrayContent) > 0 {
			for _, part := range m.Content.ArrayContent {
				mappedPart := map[string]any{}
				// OpenAI Responses API (Realtime/Responses) uses input_text and input_image
				// instead of text and image_url.
				switch strings.ToLower(string(part.Type)) {
				case "text", "input_text":
					mappedPart["type"] = "input_text"
					mappedPart["text"] = part.Text
				case "image_url", "input_image":
					mappedPart["type"] = "input_image"
					if part.ImageUrl != nil {
						mappedPart["image_url"] = part.ImageUrl.URL
					}
				default:
					mappedPart["type"] = part.Type
				}
				content = append(content, mappedPart)
			}
		} else if m.Content.StringContent != "" {
			content = append(content, map[string]any{"type": "input_text", "text": m.Content.StringContent})
		}

		for _, f := range m.Files {
			if len(f.Content) == 0 {
				continue
			}
			content = append(content, map[string]any{
				"type":      "input_image",
				"image_url": fmt.Sprintf("data:%s;base64,%s", imageType(f.Content), base64Encode(f.Content)),
			})
		}

		if len(content) == 0 {
			continue
		}

		msgs = append(msgs, OpenAIResponsesInputMessage{Role: role, Content: content})
	}

	req.Input = msgs

	// Convert tools from chat-completions tool schema to Responses tool schema.
	tools := make([]OpenAIResponsesTool, 0, len(frontendReq.Tools))
	for _, t := range frontendReq.Tools {
		if strings.TrimSpace(t.Type) == "" {
			continue
		}
		if t.Type != "function" {
			continue
		}
		name := strings.TrimSpace(t.Function.Name)
		if name == "" {
			continue
		}
		tools = append(tools, OpenAIResponsesTool{
			Type:        "function",
			Name:        name,
			Description: strings.TrimSpace(t.Function.Description),
			Parameters:  t.Function.Parameters,
			Strict:      t.Strict,
		})
	}

	// Extract tools from MCP servers if no explicit tools were provided.
	// This allows the frontend to just send mcp_servers with cached tools.
	enableMCP := frontendReq.EnableMCP == nil || *frontendReq.EnableMCP
	if enableMCP && len(tools) == 0 && len(frontendReq.MCPServers) > 0 {
		tools = append(tools, extractToolsFromMCPServers(frontendReq.MCPServers)...)
	}

	// Always include built-in local tools if any are defined.
	tools = append(tools, convertLocalToolsToResponsesTools(ToolsRequest())...)
	if len(tools) > 0 {
		req.Tools = tools
		if req.ToolChoice == nil {
			req.ToolChoice = "auto"
		}
	}

	return req, nil
}

func convertLocalToolsToResponsesTools(chatTools []OpenaiChatReqTool) []OpenAIResponsesTool {
	out := make([]OpenAIResponsesTool, 0, len(chatTools))
	for _, t := range chatTools {
		if t.Type != "function" {
			continue
		}
		name := strings.TrimSpace(t.Function.Name)
		if name == "" {
			continue
		}
		out = append(out, OpenAIResponsesTool{
			Type:        "function",
			Name:        name,
			Description: strings.TrimSpace(t.Function.Description),
			Parameters:  t.Function.Parameters,
			Strict:      t.Strict,
		})
	}
	return out
}

// extractToolsFromMCPServers extracts tools from enabled MCP servers.
// Each MCP server may have cached tool definitions in its `tools` field.
// Only enabled servers and tools with enabled_tool_names (or all if empty) are included.
func extractToolsFromMCPServers(servers []MCPServerConfig) []OpenAIResponsesTool {
	var tools []OpenAIResponsesTool
	for _, srv := range servers {
		if !srv.Enabled {
			continue
		}
		if len(srv.Tools) == 0 {
			continue
		}

		enabledSet := make(map[string]struct{}, len(srv.EnabledToolName))
		for _, name := range srv.EnabledToolName {
			enabledSet[strings.TrimSpace(name)] = struct{}{}
		}

		for _, rawTool := range srv.Tools {
			if len(rawTool) == 0 {
				continue
			}

			// Parse the raw tool definition.
			var toolDef struct {
				Name        string             `json:"name"`
				Description string             `json:"description"`
				Parameters  stdjson.RawMessage `json:"parameters"`
				InputSchema stdjson.RawMessage `json:"input_schema"`
				Function    *struct {
					Name        string             `json:"name"`
					Description string             `json:"description"`
					Parameters  stdjson.RawMessage `json:"parameters"`
				} `json:"function"`
			}
			if err := stdjson.Unmarshal(rawTool, &toolDef); err != nil {
				continue
			}

			// Extract tool name from nested function or top-level.
			name := strings.TrimSpace(toolDef.Name)
			description := strings.TrimSpace(toolDef.Description)
			params := toolDef.Parameters
			if len(params) == 0 {
				params = toolDef.InputSchema
			}
			if toolDef.Function != nil {
				if name == "" {
					name = strings.TrimSpace(toolDef.Function.Name)
				}
				if description == "" {
					description = strings.TrimSpace(toolDef.Function.Description)
				}
				if len(params) == 0 {
					params = toolDef.Function.Parameters
				}
			}

			if name == "" {
				continue
			}

			// Filter by enabled_tool_names if specified.
			if len(enabledSet) > 0 {
				if _, ok := enabledSet[name]; !ok {
					continue
				}
			}

			tools = append(tools, OpenAIResponsesTool{
				Type:        "function",
				Name:        name,
				Description: description,
				Parameters:  params,
			})
		}
	}
	return tools
}

func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// buildResponsesHTTPRequest creates an HTTP request to /v1/responses.
func buildResponsesHTTPRequest(ctx *gin.Context, user *config.UserConfig, reqBody []byte) (*http.Request, error) {
	newURL := fmt.Sprintf("%s/%s", strings.TrimRight(user.APIBase, "/"), "v1/responses")
	if ctx.Request.URL.RawQuery != "" {
		newURL += "?" + ctx.Request.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(gmw.Ctx(ctx), http.MethodPost, newURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}

	CopyHeader(req.Header, ctx.Request.Header)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+user.OpenaiToken)
	req.Header.Del("Accept-Encoding")
	return req, nil
}

// parseStreamingResponses parses a streaming Responses API response and emits deltas to the UI.
func parseStreamingResponses(
	ctx *gin.Context,
	resp *http.Response,
) (*OpenAIResponsesResp, error) {
	logger := gmw.GetLogger(ctx)
	scanner := bufio.NewScanner(resp.Body)
	finalResp := new(OpenAIResponsesResp)

	requestID := resp.Header.Get("x-oneapi-request-id")
	if requestID == "" {
		requestID = resp.Header.Get("x-request-id")
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
			logger.Warn("unmarshal responses event", zap.Error(err), zap.ByteString("data", data))
			continue
		}

		if event.Error != nil {
			return nil, errors.Errorf("upstream responses error: %v", event.Error)
		}

		if event.ResponseID != "" {
			requestID = event.ResponseID
		}

		switch event.Type {
		case "response.output_text.delta":
			emitTextDelta(ctx, true, requestID, event.Delta)
		case "response.refusal.delta":
			emitTextDelta(ctx, true, requestID, "refusal: "+event.Delta)
		case "response.reasoning_text.delta", "response.thought.delta", "response.reasoning.delta":
			emitThinkingDelta(ctx, true, requestID, event.Delta)
		case "response.function_call_arguments.delta":
			emitThinkingDelta(ctx, true, requestID, event.Delta)
		case "response.completed":
			if event.Response != nil {
				finalResp = event.Response
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "scanner error")
	}

	if finalResp.ID == "" {
		finalResp.ID = requestID
	}

	return finalResp, nil
}

// callUpstreamResponses executes a Responses API request and returns the parsed response.
func callUpstreamResponses(
	ctx *gin.Context,
	user *config.UserConfig,
	req *OpenAIResponsesReq,
) (*OpenAIResponsesResp, http.Header, error) {
	logger := gmw.GetLogger(ctx)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal responses req")
	}

	logger.Debug("send responses request to upstream",
		zap.String("model", req.Model),
		zap.Int("payload_bytes", len(body)),
	)

	upReq, err := buildResponsesHTTPRequest(ctx, user, body)
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
		setStreamHeaders(ctx, resp.Header)
		ctx.Set("llm_response_streamed", true)
		out, err := parseStreamingResponses(ctx, resp)
		return out, resp.Header, err
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

	if out.Error != nil {
		return nil, resp.Header, errors.Errorf("upstream responses error: %v", out.Error)
	}

	return out, resp.Header, nil
}

// truncateBytesForLog truncates raw bytes into a safe string for error messages.
// It avoids huge logs and reduces risk of accidental sensitive data exposure.
func truncateBytesForLog(b []byte, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 1024
	}
	if len(b) <= maxBytes {
		return string(b)
	}
	return string(b[:maxBytes]) + "..."
}

func extractFunctionCallsFromResponses(resp *OpenAIResponsesResp) ([]OpenAIResponsesFunctionCall, error) {
	if resp == nil {
		return nil, errors.New("empty responses resp")
	}

	// Prefer required_action.submit_tool_outputs.tool_calls if present.
	if resp.RequiredAction != nil &&
		resp.RequiredAction.Type == "submit_tool_outputs" &&
		resp.RequiredAction.SubmitToolOutputs != nil {
		calls := make([]OpenAIResponsesFunctionCall, 0, len(resp.RequiredAction.SubmitToolOutputs.ToolCalls))
		for _, tc := range resp.RequiredAction.SubmitToolOutputs.ToolCalls {
			name := strings.TrimSpace(tc.Function.Name)
			callID := strings.TrimSpace(tc.ID)
			if name == "" || callID == "" {
				continue
			}
			calls = append(calls, OpenAIResponsesFunctionCall{
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

	calls := make([]OpenAIResponsesFunctionCall, 0)
	for _, item := range resp.Output {
		if item.Type != "function_call" {
			continue
		}
		var fc OpenAIResponsesFunctionCall
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

// extractOutputTextFromResponses extracts assistant text from output items when output_text is empty.
func extractOutputTextFromResponses(resp *OpenAIResponsesResp) string {
	if resp == nil {
		return ""
	}
	if strings.TrimSpace(resp.OutputText) != "" {
		return resp.OutputText
	}

	// Typical shape:
	// {"type":"message","role":"assistant","content":[{"type":"output_text","text":"..."}]}
	texts := make([]string, 0, 4)
	for _, item := range resp.Output {
		if item.Type != "message" {
			continue
		}
		var msg struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(item.Raw(), &msg); err != nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(msg.Role)) != "assistant" {
			continue
		}
		for _, c := range msg.Content {
			if c.Type == "output_text" && strings.TrimSpace(c.Text) != "" {
				texts = append(texts, c.Text)
			}
		}
	}

	return strings.Join(texts, "")
}

// extractReasoningFromResponses extracts reasoning text from output items.
func extractReasoningFromResponses(resp *OpenAIResponsesResp) string {
	if resp == nil {
		return ""
	}

	var reasoning []string
	for _, item := range resp.Output {
		switch item.Type {
		case "reasoning":
			var rItem struct {
				Summary []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"summary"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			}
			if err := stdjson.Unmarshal(item.Raw(), &rItem); err == nil {
				for _, c := range rItem.Summary {
					if strings.TrimSpace(c.Text) != "" {
						reasoning = append(reasoning, c.Text)
					}
				}
				for _, c := range rItem.Content {
					if strings.TrimSpace(c.Text) != "" {
						reasoning = append(reasoning, c.Text)
					}
				}
			}
		case "message":
			var msg struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			}
			if err := stdjson.Unmarshal(item.Raw(), &msg); err == nil {
				if strings.ToLower(strings.TrimSpace(msg.Role)) == "assistant" {
					for _, c := range msg.Content {
						// Some models use "thought" or "reasoning_text"
						if (c.Type == "reasoning_text" || c.Type == "thought") && strings.TrimSpace(c.Text) != "" {
							reasoning = append(reasoning, c.Text)
						}
					}
				}
			}
		}
	}

	return strings.Join(reasoning, "\n")
}
