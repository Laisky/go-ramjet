package memoryx

import (
	stdjson "encoding/json"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/agents/memory"
)

// ResponsesInputToMemoryItems converts Responses API input items into memory SDK items.
//
// Parameters:
//   - inputItems: Flattened Responses API input item list.
//
// Returns:
//   - []memory.ResponseItem: Converted memory items.
//   - error: Non-nil when an item cannot be converted.
func ResponsesInputToMemoryItems(inputItems []any) ([]memory.ResponseItem, error) {
	out := make([]memory.ResponseItem, 0, len(inputItems))
	for _, item := range inputItems {
		converted, err := toMemoryResponseItem(item)
		if err != nil {
			return nil, errors.Wrap(err, "convert responses input item")
		}
		if converted == nil {
			continue
		}

		out = append(out, *converted)
	}

	return out, nil
}

func toMemoryResponseItem(input any) (*memory.ResponseItem, error) {
	m, err := toMap(input)
	if err != nil {
		return nil, errors.Wrap(err, "to map")
	}

	if role := stringField(m["role"]); role != "" {
		item := memory.ResponseItem{Type: "message", Role: role}
		parts, parseErr := parseMessageContentParts(m["content"])
		if parseErr != nil {
			return nil, errors.Wrap(parseErr, "parse message content")
		}
		item.Content = parts
		return &item, nil
	}

	typ := stringField(m["type"])
	switch typ {
	case "function_call_output":
		return &memory.ResponseItem{
			Type:   typ,
			CallID: stringField(m["call_id"]),
			Output: stringField(m["output"]),
		}, nil
	case "function_call":
		metadata := map[string]string{}
		if name := stringField(m["name"]); name != "" {
			metadata["name"] = name
		}
		if id := stringField(m["id"]); id != "" {
			metadata["id"] = id
		}

		item := &memory.ResponseItem{
			Type:     typ,
			CallID:   stringField(m["call_id"]),
			Output:   stringField(m["arguments"]),
			Metadata: metadata,
		}
		if len(item.Metadata) == 0 {
			item.Metadata = nil
		}
		return item, nil
	default:
		if typ == "" {
			return nil, nil
		}
		return &memory.ResponseItem{Type: typ}, nil
	}
}

func parseMessageContentParts(raw any) ([]memory.ResponseContentPart, error) {
	if raw == nil {
		return nil, nil
	}

	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		return []memory.ResponseContentPart{{Type: "input_text", Text: v}}, nil
	case []map[string]any:
		parts := make([]memory.ResponseContentPart, 0, len(v))
		for _, item := range v {
			part := parseContentPart(item)
			if part == nil {
				continue
			}
			parts = append(parts, *part)
		}
		return parts, nil
	case []any:
		parts := make([]memory.ResponseContentPart, 0, len(v))
		for _, item := range v {
			partMap, err := toMap(item)
			if err != nil {
				return nil, errors.Wrap(err, "content part to map")
			}

			part := parseContentPart(partMap)
			if part == nil {
				continue
			}
			parts = append(parts, *part)
		}
		return parts, nil
	default:
		return nil, errors.Errorf("unsupported message content type %T", raw)
	}
}

func parseContentPart(partMap map[string]any) *memory.ResponseContentPart {
	typ := strings.ToLower(stringField(partMap["type"]))
	switch typ {
	case "input_text", "output_text", "text":
		text := stringField(partMap["text"])
		if text == "" {
			return nil
		}
		return &memory.ResponseContentPart{Type: "input_text", Text: text}
	case "input_image", "image_url", "output_image":
		imageURL := extractImageURL(partMap["image_url"])
		if imageURL == "" {
			return nil
		}
		return &memory.ResponseContentPart{Type: "input_image", ImageURL: imageURL}
	case "input_file":
		part := &memory.ResponseContentPart{Type: "input_file"}
		if fileID := stringField(partMap["file_id"]); fileID != "" {
			part.FileID = fileID
		}
		if filename := stringField(partMap["filename"]); filename != "" {
			part.Filename = filename
		}
		return part
	default:
		text := stringField(partMap["text"])
		if text != "" {
			return &memory.ResponseContentPart{Type: "input_text", Text: text}
		}
		return nil
	}
}

func extractImageURL(raw any) string {
	if raw == nil {
		return ""
	}

	if s := stringField(raw); s != "" {
		return s
	}

	if m, ok := raw.(map[string]any); ok {
		return stringField(m["url"])
	}

	return ""
}

// MemoryItemsToResponsesInput converts memory SDK response items back to Responses API input items.
//
// Parameters:
//   - items: Memory SDK response items.
//
// Returns:
//   - []any: Responses API compatible input items.
func MemoryItemsToResponsesInput(items []memory.ResponseItem) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case "message":
			content := make([]map[string]any, 0, len(item.Content))
			for _, part := range item.Content {
				mapped := map[string]any{"type": part.Type}
				switch part.Type {
				case "input_text", "output_text", "text":
					mapped["type"] = "input_text"
					mapped["text"] = part.Text
				case "input_image", "image_url", "output_image":
					mapped["type"] = "input_image"
					mapped["image_url"] = part.ImageURL
				case "input_file":
					if part.FileID != "" {
						mapped["file_id"] = part.FileID
					}
					if part.Filename != "" {
						mapped["filename"] = part.Filename
					}
				default:
					if part.Text != "" {
						mapped["type"] = "input_text"
						mapped["text"] = part.Text
					}
				}
				content = append(content, mapped)
			}

			out = append(out, map[string]any{"role": item.Role, "content": content})
		case "function_call_output":
			out = append(out, map[string]any{
				"type":    "function_call_output",
				"call_id": item.CallID,
				"output":  item.Output,
			})
		case "function_call":
			mapped := map[string]any{
				"type":      "function_call",
				"call_id":   item.CallID,
				"arguments": item.Output,
			}
			if name := item.Metadata["name"]; name != "" {
				mapped["name"] = name
			}
			if id := item.Metadata["id"]; id != "" {
				mapped["id"] = id
			}
			out = append(out, mapped)
		default:
			out = append(out, map[string]any{"type": item.Type})
		}
	}

	return out
}

// BuildAssistantOutputItems builds memory output items from final assistant text.
//
// Parameters:
//   - finalText: Final assistant answer text.
//
// Returns:
//   - []memory.ResponseItem: Output items for memory AfterTurn hook.
func BuildAssistantOutputItems(finalText string) []memory.ResponseItem {
	return []memory.ResponseItem{{
		Type: "message",
		Role: "assistant",
		Content: []memory.ResponseContentPart{{
			Type: "output_text",
			Text: finalText,
		}},
	}}
}

func toMap(v any) (map[string]any, error) {
	if existing, ok := v.(map[string]any); ok {
		return existing, nil
	}

	data, err := stdjson.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "marshal any")
	}

	out := map[string]any{}
	if err = stdjson.Unmarshal(data, &out); err != nil {
		return nil, errors.Wrap(err, "unmarshal any to map")
	}

	return out, nil
}

func stringField(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}
