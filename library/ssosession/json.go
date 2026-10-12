package ssosession

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

// requireUnambiguousJSON rejects duplicate fields and excessive nesting only for
// strict private-session requests; CV retains its existing decoder semantics.
func requireUnambiguousJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return errors.New("invalid JSON encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := consumeUniqueJSON(decoder, 0); err != nil {
		return errors.Wrap(err, "invalid JSON object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("extra JSON data")
	}
	return nil
}

// consumeUniqueJSON validates one bounded JSON value without retaining secrets.
func consumeUniqueJSON(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("JSON nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.Wrap(err, "read JSON token")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		names := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return errors.Wrap(err, "read JSON field")
			}
			name, ok := key.(string)
			if !ok || names[name] {
				return errors.New("ambiguous JSON field")
			}
			names[name] = true
			if err := consumeUniqueJSON(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("invalid JSON object")
		}
	case '[':
		for decoder.More() {
			if err := consumeUniqueJSON(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("invalid JSON array")
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	return nil
}

// decodeExactFields prevents encoding/json's case-insensitive struct matching
// from interpreting unrecognized aliases as authentication fields.
func decodeExactFields(raw []byte, target any, names ...string) error {
	if err := requireUnambiguousJSON(raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return errors.Wrap(err, "decode JSON fields")
	}
	selected := map[string]json.RawMessage{}
	for _, name := range names {
		if value, ok := fields[name]; ok {
			selected[name] = value
		}
	}
	encoded, err := json.Marshal(selected)
	if err != nil {
		return errors.Wrap(err, "encode selected JSON fields")
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		return errors.Wrap(err, "decode selected JSON fields")
	}
	return nil
}

// exactWhoAmIEnvelope preserves only the documented case-sensitive GraphQL
// fields for strict requests; the legacy CV decoder remains unchanged.
func exactWhoAmIEnvelope(raw []byte) ([]byte, error) {
	if err := requireUnambiguousJSON(raw); err != nil {
		return nil, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, errors.Wrap(err, "decode identity envelope")
	}
	result := map[string]any{}
	if failure, ok := envelope["errors"]; ok {
		result["errors"] = failure
	}
	if data, ok := envelope["data"]; ok {
		if string(data) == "null" {
			result["data"] = nil
		} else {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				return nil, errors.Wrap(err, "decode identity data")
			}
			who, ok := fields["WhoAmI"]
			if ok {
				if string(who) == "null" {
					result["data"] = map[string]any{"WhoAmI": nil}
				} else {
					var identity map[string]json.RawMessage
					if err := json.Unmarshal(who, &identity); err != nil {
						return nil, errors.Wrap(err, "decode account identity")
					}
					selected := map[string]json.RawMessage{}
					for _, name := range []string{"id", "username"} {
						if value, ok := identity[name]; ok {
							selected[name] = value
						}
					}
					result["data"] = map[string]any{"WhoAmI": selected}
				}
			} else {
				result["data"] = map[string]any{}
			}
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, errors.Wrap(err, "encode exact identity envelope")
	}
	return encoded, nil
}
