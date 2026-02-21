package memoryx

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/memory"
)

func TestResponsesInputToMemoryItems(t *testing.T) {
	input := []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "hello"},
				map[string]any{"type": "input_image", "image_url": "https://a/b.png"},
			},
		},
		map[string]any{"type": "function_call_output", "call_id": "c1", "output": "ok"},
	}

	items, err := ResponsesInputToMemoryItems(input)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "message", items[0].Type)
	require.Equal(t, "user", items[0].Role)
	require.Len(t, items[0].Content, 2)
	require.Equal(t, "input_text", items[0].Content[0].Type)
	require.Equal(t, "hello", items[0].Content[0].Text)
	require.Equal(t, "input_image", items[0].Content[1].Type)
	require.Equal(t, "https://a/b.png", items[0].Content[1].ImageURL)
	require.Equal(t, "function_call_output", items[1].Type)
	require.Equal(t, "c1", items[1].CallID)
}

func TestMemoryItemsToResponsesInput(t *testing.T) {
	items := []memory.ResponseItem{
		{
			Type: "message",
			Role: "developer",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "memory block",
			}},
		},
		{Type: "function_call_output", CallID: "c1", Output: "done"},
	}

	output := MemoryItemsToResponsesInput(items)
	require.Len(t, output, 2)
	msg := output[0].(map[string]any)
	require.Equal(t, "developer", msg["role"])
	content := msg["content"].([]map[string]any)
	require.Equal(t, "memory block", content[0]["text"])
	fout := output[1].(map[string]any)
	require.Equal(t, "function_call_output", fout["type"])
	require.Equal(t, "c1", fout["call_id"])
}
