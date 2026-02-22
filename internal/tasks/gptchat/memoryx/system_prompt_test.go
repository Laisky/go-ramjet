package memoryx

import (
	"testing"

	"github.com/Laisky/go-utils/v6/agents/memory"
	"github.com/stretchr/testify/require"
)

// TestPreserveSystemMessageItemsKeepsOriginalSystemPrompt verifies memory injection cannot overwrite original system prompt.
func TestPreserveSystemMessageItemsKeepsOriginalSystemPrompt(t *testing.T) {
	original := []memory.ResponseItem{
		{
			Type: "message",
			Role: "system",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "ORIGINAL SYSTEM PROMPT",
			}},
		},
		{
			Type: "message",
			Role: "user",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "hi",
			}},
		},
	}
	prepared := []memory.ResponseItem{
		{
			Type: "message",
			Role: "system",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "MUTATED SYSTEM PROMPT",
			}},
		},
		{
			Type: "message",
			Role: "developer",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "memory context",
			}},
		},
		{
			Type: "message",
			Role: "user",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "hi",
			}},
		},
	}

	got := preserveSystemMessageItems(original, prepared)
	require.Len(t, got, 3)
	require.Equal(t, "system", got[0].Role)
	require.Equal(t, "ORIGINAL SYSTEM PROMPT", got[0].Content[0].Text)
	require.Equal(t, "developer", got[1].Role)
	require.Equal(t, "user", got[2].Role)
}

// TestPreserveSystemMessageItemsWithoutSystem verifies behavior when original input has no system message.
func TestPreserveSystemMessageItemsWithoutSystem(t *testing.T) {
	original := []memory.ResponseItem{{
		Type: "message",
		Role: "user",
		Content: []memory.ResponseContentPart{{
			Type: "input_text",
			Text: "hi",
		}},
	}}
	prepared := []memory.ResponseItem{{
		Type: "message",
		Role: "developer",
		Content: []memory.ResponseContentPart{{
			Type: "input_text",
			Text: "memory context",
		}},
	}}

	got := preserveSystemMessageItems(original, prepared)
	require.Equal(t, prepared, got)
}
