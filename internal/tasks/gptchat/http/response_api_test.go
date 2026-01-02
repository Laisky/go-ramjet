package http

import (
	"testing"

	"github.com/Laisky/go-utils/v5/json"
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
				Role: OpenaiMessageRoleUser,
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
