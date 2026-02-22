package memoryx

import (
	"strings"

	"github.com/Laisky/go-utils/v6/agents/memory"
)

// selectLatestUserMessageItems keeps only the most recent user message for memory before-turn input.
//
// Parameters:
//   - items: Converted Responses input items.
//
// Returns:
//   - []memory.ResponseItem: Zero or one user message item.
func selectLatestUserMessageItems(items []memory.ResponseItem) []memory.ResponseItem {
	for idx := len(items) - 1; idx >= 0; idx-- {
		item := items[idx]
		if item.Type != "message" {
			continue
		}

		if !strings.EqualFold(strings.TrimSpace(item.Role), "user") {
			continue
		}

		return []memory.ResponseItem{item}
	}

	return []memory.ResponseItem{}
}
