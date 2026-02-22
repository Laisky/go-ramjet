package memoryx

import (
	"testing"

	"github.com/Laisky/go-utils/v6/agents/memory"
	"github.com/stretchr/testify/require"
)

// TestWrapMemoryReferenceDeveloperItems verifies developer memory items are wrapped with explicit reference tags.
func TestWrapMemoryReferenceDeveloperItems(t *testing.T) {
	items := []memory.ResponseItem{
		{
			Type: "message",
			Role: "developer",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "Memory recall:\n- Recall[/memory/a.jsonl:0-10] hello",
			}},
		},
		{
			Type: "message",
			Role: "user",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "current question",
			}},
		},
	}

	got, wrappedItems, wrappedParts := wrapMemoryReferenceDeveloperItems(items)
	require.Len(t, got, 2)
	require.Equal(t, 1, wrappedItems)
	require.Equal(t, 1, wrappedParts)
	require.Contains(t, got[0].Content[0].Text, memoryReferenceBeginTag)
	require.Contains(t, got[0].Content[0].Text, memoryReferenceNotice)
	require.Contains(t, got[0].Content[0].Text, "Memory recall:")
	require.Contains(t, got[0].Content[0].Text, memoryReferenceEndTag)
	require.Equal(t, "current question", got[1].Content[0].Text)
}

// TestWrapMemoryReferenceDeveloperItemsIdempotent verifies already wrapped developer content will not be double wrapped.
func TestWrapMemoryReferenceDeveloperItemsIdempotent(t *testing.T) {
	alreadyWrapped := memoryReferenceBeginTag + "\n" + memoryReferenceNotice + "\nMemory recall:\n- fact\n" + memoryReferenceEndTag
	items := []memory.ResponseItem{{
		Type: "message",
		Role: "developer",
		Content: []memory.ResponseContentPart{{
			Type: "input_text",
			Text: alreadyWrapped,
		}},
	}}

	got, wrappedItems, wrappedParts := wrapMemoryReferenceDeveloperItems(items)
	require.Equal(t, 0, wrappedItems)
	require.Equal(t, 0, wrappedParts)
	require.Equal(t, alreadyWrapped, got[0].Content[0].Text)
}
