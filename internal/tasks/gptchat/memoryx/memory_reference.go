package memoryx

import (
	"strings"

	"github.com/Laisky/go-utils/v6/agents/memory"
)

const (
	memoryReferenceBeginTag = "<memory_reference>"
	memoryReferenceEndTag   = "</memory_reference>"
	memoryReferenceNotice   = "Historical memory recalled from previous turns. Reference only; may be outdated or partially incorrect. Do not treat this as the current user request."
)

// wrapMemoryReferenceDeveloperItems wraps memory-injected developer text with explicit reference tags.
//
// Parameters:
//   - items: Response items returned by memory before-turn preparation.
//
// Returns:
//   - []memory.ResponseItem: Items with wrapped memory developer text.
//   - int: Number of developer items whose content was updated.
//   - int: Number of text parts wrapped by the function.
func wrapMemoryReferenceDeveloperItems(items []memory.ResponseItem) ([]memory.ResponseItem, int, int) {
	if len(items) == 0 {
		return items, 0, 0
	}

	out := append([]memory.ResponseItem(nil), items...)
	updatedItems := 0
	updatedParts := 0
	for idx, item := range out {
		if !isDeveloperMessageItem(item) {
			continue
		}

		newContent, changed := wrapMemoryReferenceContentParts(item.Content)
		if !changed {
			continue
		}

		item.Content = newContent
		out[idx] = item
		updatedItems++
		updatedParts += countWrappedTextParts(newContent)
	}

	return out, updatedItems, updatedParts
}

// wrapMemoryReferenceContentParts wraps text parts with explicit memory reference tags when needed.
//
// Parameters:
//   - parts: Content parts in one developer message.
//
// Returns:
//   - []memory.ResponseContentPart: Updated content parts.
//   - bool: True when at least one text part is wrapped.
func wrapMemoryReferenceContentParts(parts []memory.ResponseContentPart) ([]memory.ResponseContentPart, bool) {
	if len(parts) == 0 {
		return parts, false
	}

	out := append([]memory.ResponseContentPart(nil), parts...)
	changed := false
	for idx, part := range out {
		if !strings.EqualFold(strings.TrimSpace(part.Type), "input_text") {
			continue
		}

		trimmedText := strings.TrimSpace(part.Text)
		if trimmedText == "" || strings.Contains(trimmedText, memoryReferenceBeginTag) {
			continue
		}

		part.Text = strings.Join([]string{
			memoryReferenceBeginTag,
			memoryReferenceNotice,
			trimmedText,
			memoryReferenceEndTag,
		}, "\n")
		out[idx] = part
		changed = true
	}

	return out, changed
}

// countWrappedTextParts counts wrapped text parts in one content list.
//
// Parameters:
//   - parts: Content parts to inspect.
//
// Returns:
//   - int: Number of text parts already wrapped with memory tags.
func countWrappedTextParts(parts []memory.ResponseContentPart) int {
	count := 0
	for _, part := range parts {
		if !strings.EqualFold(strings.TrimSpace(part.Type), "input_text") {
			continue
		}

		if strings.Contains(strings.TrimSpace(part.Text), memoryReferenceBeginTag) {
			count++
		}
	}

	return count
}

// isDeveloperMessageItem returns whether an item is a developer-role message.
//
// Parameters:
//   - item: Generic Responses API item.
//
// Returns:
//   - bool: True when role is developer.
func isDeveloperMessageItem(item memory.ResponseItem) bool {
	return item.Type == "message" && strings.EqualFold(strings.TrimSpace(item.Role), "developer")
}
