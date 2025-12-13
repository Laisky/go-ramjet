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
