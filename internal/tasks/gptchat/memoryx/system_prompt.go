package memoryx

import (
	"strings"

	"github.com/Laisky/go-utils/v6/agents/memory"
)

// preserveSystemMessageItems keeps original system messages unchanged while using memory-prepared items.
//
// Parameters:
//   - originalItems: Input items before memory.BeforeTurn.
//   - preparedItems: Input items returned by memory.BeforeTurn.
//
// Returns:
//   - []memory.ResponseItem where original system messages are preserved.
func preserveSystemMessageItems(originalItems []memory.ResponseItem, preparedItems []memory.ResponseItem) []memory.ResponseItem {
	originalSystemItems := collectSystemItems(originalItems)
	if len(originalSystemItems) == 0 {
		return append([]memory.ResponseItem(nil), preparedItems...)
	}

	out := make([]memory.ResponseItem, 0, len(preparedItems)+len(originalSystemItems))
	out = append(out, originalSystemItems...)
	for _, item := range preparedItems {
		if isSystemMessageItem(item) {
			continue
		}
		out = append(out, item)
	}

	return out
}

// collectSystemItems extracts system-role messages from input items.
//
// Parameters:
//   - items: Input items to inspect.
//
// Returns:
//   - []memory.ResponseItem: Items whose role is system.
func collectSystemItems(items []memory.ResponseItem) []memory.ResponseItem {
	out := make([]memory.ResponseItem, 0, len(items))
	for _, item := range items {
		if isSystemMessageItem(item) {
			out = append(out, item)
		}
	}

	return out
}

// isSystemMessageItem returns whether the item is a message with role system.
//
// Parameters:
//   - item: Generic Responses API item.
//
// Returns:
//   - bool: True when role is system.
func isSystemMessageItem(item memory.ResponseItem) bool {
	return item.Type == "message" && strings.EqualFold(strings.TrimSpace(item.Role), "system")
}
