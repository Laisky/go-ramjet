package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExtractFunctionCallsFromResponses_RequiredAction(t *testing.T) {
	raw := `{
		"id":"resp-2025121303004859291666579997266",
		"object":"response",
		"status":"completed",
		"model":"openai/gpt-oss-20b",
		"output":[
			{"type":"reasoning","status":"completed","summary":[{"type":"summary_text","text":"Need to fetch weather info."}]},
			{"type":"function_call","status":"completed","call_id":"fc_8e684520-f6a1-4d02-b415-6782e07c7a54","name":"web_search","arguments":"{\"query\":\"Ottawa weather today\"}"}
		],
		"required_action":{
			"type":"submit_tool_outputs",
			"submit_tool_outputs":{
				"tool_calls":[
					{"id":"call_8e684520-f6a1-4d02-b415-6782e07c7a54","type":"function","function":{"name":"web_search","arguments":"{\"query\":\"Ottawa weather today\"}"}}
				]
			}
		}
	}`

	resp := new(OpenAIResponsesResp)
	require.NoError(t, json.Unmarshal([]byte(raw), resp))

	calls, err := extractFunctionCallsFromResponses(resp)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.Equal(t, "web_search", calls[0].Name)
	require.Equal(t, "call_8e684520-f6a1-4d02-b415-6782e07c7a54", calls[0].CallID)
	require.Contains(t, calls[0].Arguments, "Ottawa")
}

func TestConvertFrontendToResponsesRequest_Mapping(t *testing.T) {
	frontendReq := &FrontendReq{
		Model: "gpt-4o-mini",
		Messages: []FrontendReqMessage{
			{
				Role: OpenaiMessageRoleUser,
				Content: FrontendReqMessageContent{
					ArrayContent: []OpenaiVisionMessageContent{
						{Type: OpenaiVisionMessageContentTypeText, Text: "what do you see"},
						{Type: OpenaiVisionMessageContentTypeImageUrl, ImageUrl: &OpenaiVisionMessageContentImageUrl{URL: "data:image/png;base64,xxx"}},
					},
				},
			},
		},
	}

	respReq, err := convertFrontendToResponsesRequest(frontendReq)
	require.NoError(t, err)
	require.NotNil(t, respReq)

	input := respReq.Input.([]OpenAIResponsesInputMessage)
	require.Len(t, input, 1)
	content := input[0].Content.([]any)
	require.Len(t, content, 2)

	part1 := content[0].(map[string]any)
	require.Equal(t, "input_text", part1["type"])
	require.Equal(t, "what do you see", part1["text"])

	part2 := content[1].(map[string]any)
	require.Equal(t, "input_image", part2["type"])
	require.Equal(t, "data:image/png;base64,xxx", part2["image_url"])
}

func TestConvertFrontendToResponsesRequest_Files(t *testing.T) {
	frontendReq := &FrontendReq{
		Model: "gpt-4o-mini",
		Messages: []FrontendReqMessage{
			{
				Role:    OpenaiMessageRoleUser,
				Content: FrontendReqMessageContent{StringContent: "hello"},
				Files: []frontendReqMessageFiles{
					{Type: "image", Content: []byte("fake-image")},
				},
			},
		},
	}

	respReq, err := convertFrontendToResponsesRequest(frontendReq)
	require.NoError(t, err)
	require.NotNil(t, respReq)

	input := respReq.Input.([]OpenAIResponsesInputMessage)
	require.Len(t, input, 1)
	content := input[0].Content.([]any)
	require.Len(t, content, 2)

	part1 := content[0].(map[string]any)
	require.Equal(t, "input_text", part1["type"])

	part2 := content[1].(map[string]any)
	require.Equal(t, "input_image", part2["type"])
	require.Contains(t, part2["image_url"], "base64,")
}

func TestConvertFrontendToResponsesRequest_DisableMCPNoTools(t *testing.T) {
	enableMCP := false
	frontendReq := &FrontendReq{
		Model:     "gpt-4o-mini",
		EnableMCP: &enableMCP,
		Messages: []FrontendReqMessage{
			{Role: OpenaiMessageRoleUser, Content: FrontendReqMessageContent{StringContent: "hello"}},
		},
		Tools: []OpenaiChatReqTool{
			{Type: "function", Function: OpenaiChatReqToolFn{Name: "how-to-subscribe"}},
		},
		MCPServers: []MCPServerConfig{
			{Enabled: true},
		},
	}

	respReq, err := convertFrontendToResponsesRequest(frontendReq)
	require.NoError(t, err)
	require.NotNil(t, respReq)
	require.Nil(t, respReq.ToolChoice)
	require.Len(t, respReq.Tools, 0)
}

// TestParseStreamingResponses_LargeLine ensures the stream parser accepts large SSE lines without scanner errors.
func TestParseStreamingResponses_LargeLine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	largeDelta := strings.Repeat("a", 200*1024)
	data1, err := json.Marshal(map[string]any{
		"type":        "response.output_text.delta",
		"response_id": "resp-large",
		"delta":       largeDelta,
	})
	require.NoError(t, err)

	data2, err := json.Marshal(map[string]any{
		"type": "response.completed",
		"response": &OpenAIResponsesResp{
			ID: "resp-large",
		},
	})
	require.NoError(t, err)

	var sse strings.Builder
	sse.WriteString("data: ")
	sse.Write(data1)
	sse.WriteString("\n\n")
	sse.WriteString("data: ")
	sse.Write(data2)
	sse.WriteString("\n\n")
	sse.WriteString("data: [DONE]\n\n")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(sse.String())),
	}

	out, err := parseStreamingResponses(ctx, resp)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "resp-large", out.ID)
}

func TestExtractOutputTextFromResponses_WithImages(t *testing.T) {
	raw := `{
		"id":"resp-1",
		"output_text":"Here you go!",
		"output":[
			{"type":"message","role":"assistant","content":[
				{"type":"output_image","image_url":{"url":"data:image/png;base64,AAA"}}
			]}
		]
	}`

	resp := new(OpenAIResponsesResp)
	require.NoError(t, json.Unmarshal([]byte(raw), resp))

	out := extractOutputTextFromResponses(resp)
	require.Equal(t, "Here you go!\n\n![Image](data:image/png;base64,AAA)", out)
}

func TestExtractOutputTextFromResponses_MessageTextAndImages(t *testing.T) {
	raw := `{
		"id":"resp-2",
		"output_text":"",
		"output":[
			{"type":"message","role":"assistant","content":[
				{"type":"output_text","text":"Hello"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,BBB"}}
			]}
		]
	}`

	resp := new(OpenAIResponsesResp)
	require.NoError(t, json.Unmarshal([]byte(raw), resp))

	out := extractOutputTextFromResponses(resp)
	require.Equal(t, "Hello\n\n![Image](data:image/png;base64,BBB)", out)
}

func TestParseStreamingResponses_ChatCompletionFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(string(gmw.CtxKeyLock), &sync.RWMutex{})

	chunk1 := OpenaiCompletionStreamResp{
		ID:      "chatcmpl-1",
		Object:  "chat.completion.chunk",
		Created: 123,
		Model:   "gemini",
		Choices: []OpenaiCompletionStreamRespChoice{{
			Delta: OpenaiCompletionStreamRespDelta{
				Role:    OpenaiMessageRoleAI,
				Content: "Here",
			},
			Index: 0,
		}},
	}
	chunk2 := OpenaiCompletionStreamResp{
		ID:      "chatcmpl-1",
		Object:  "chat.completion.chunk",
		Created: 123,
		Model:   "gemini",
		Choices: []OpenaiCompletionStreamRespChoice{{
			Delta: OpenaiCompletionStreamRespDelta{
				Content: []map[string]any{{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/png;base64,AAA",
					},
				}},
			},
			Index: 0,
		}},
	}
	chunk3 := OpenaiCompletionStreamResp{
		ID:      "chatcmpl-1",
		Object:  "chat.completion.chunk",
		Created: 123,
		Model:   "gemini",
		Choices: []OpenaiCompletionStreamRespChoice{{
			Delta:        OpenaiCompletionStreamRespDelta{Role: OpenaiMessageRoleAI},
			Index:        0,
			FinishReason: "stop",
		}},
	}

	data1, err := json.Marshal(chunk1)
	require.NoError(t, err)
	data2, err := json.Marshal(chunk2)
	require.NoError(t, err)
	data3, err := json.Marshal(chunk3)
	require.NoError(t, err)

	var sse strings.Builder
	sse.WriteString("data: ")
	sse.Write(data1)
	sse.WriteString("\n\n")
	sse.WriteString("data: ")
	sse.Write(data2)
	sse.WriteString("\n\n")
	sse.WriteString("data: ")
	sse.Write(data3)
	sse.WriteString("\n\n")
	sse.WriteString("data: [DONE]\n\n")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(sse.String())),
	}

	out, err := parseStreamingResponses(ctx, resp)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "chatcmpl-1", out.ID)
	require.Contains(t, out.OutputText, "Here")
	require.Contains(t, out.OutputText, "![Image](data:image/png;base64,AAA)")
	require.Contains(t, recorder.Body.String(), "![Image](data:image/png;base64,AAA)")
}
