package session

import (
	stdjson "encoding/json"
	"io"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/json"
)

// Transcript is the append-only event log. Append rejects duplicate
// EventID()s, Events() snapshots in insertion order, Tree() exposes the
// parent/child index, Branch forks a new transcript sharing ancestors up to
// (and including) the named pivot event, and JSONL serializes one event per
// line.
type Transcript interface {
	Append(Event) error
	Events() []Event
	Tree() *TranscriptTree
	Branch(fromEventID string) (Transcript, error)
	JSONL(w io.Writer) error
}

// TranscriptTree indexes events by parent for cheap child lookups. It is
// rebuilt from Events() on demand and is safe to keep around for the
// lifetime of the snapshot it was derived from.
type TranscriptTree struct {
	// ByID maps event ID to event.
	ByID map[string]Event
	// Children maps a parent event ID to its direct children, in insertion
	// order. The root events appear under the empty string key.
	Children map[string][]Event
}

// transcript is the concrete in-memory implementation.
type transcript struct {
	mu     sync.RWMutex
	events []Event
	ids    map[string]struct{}
}

// NewTranscript constructs an empty transcript.
func NewTranscript() Transcript {
	return &transcript{ids: make(map[string]struct{})}
}

// Append records ev. It returns a descriptive error if ev's EventID has
// already been seen; the existing event is not touched and ev is not added.
func (t *transcript) Append(ev Event) error {
	if ev == nil {
		return errors.New("transcript.Append: nil event")
	}
	id := ev.EventID()
	if id == "" {
		return errors.New("transcript.Append: event has empty EventID")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, dup := t.ids[id]; dup {
		return errors.Errorf("transcript.Append: duplicate EventID %q (kind=%s)", id, ev.Kind())
	}
	t.ids[id] = struct{}{}
	t.events = append(t.events, ev)
	return nil
}

// Events returns a defensive copy of the insertion-ordered event slice.
func (t *transcript) Events() []Event {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Event, len(t.events))
	copy(out, t.events)
	return out
}

// Tree returns the parent/child index over the current snapshot.
func (t *transcript) Tree() *TranscriptTree {
	t.mu.RLock()
	defer t.mu.RUnlock()
	tree := &TranscriptTree{
		ByID:     make(map[string]Event, len(t.events)),
		Children: make(map[string][]Event),
	}
	for _, ev := range t.events {
		tree.ByID[ev.EventID()] = ev
		parent := ev.ParentEventID()
		tree.Children[parent] = append(tree.Children[parent], ev)
	}
	return tree
}

// Branch returns a new transcript pre-populated with every event recorded up
// to and including the pivot. Subsequent Appends on either branch do not
// affect the other. Returns an error if fromEventID is not present.
func (t *transcript) Branch(fromEventID string) (Transcript, error) {
	if fromEventID == "" {
		return nil, errors.New("transcript.Branch: empty fromEventID")
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	pivot := -1
	for i, ev := range t.events {
		if ev.EventID() == fromEventID {
			pivot = i
			break
		}
	}
	if pivot == -1 {
		return nil, errors.Errorf("transcript.Branch: event %q not found", fromEventID)
	}
	child := &transcript{
		events: make([]Event, pivot+1),
		ids:    make(map[string]struct{}, pivot+1),
	}
	copy(child.events, t.events[:pivot+1])
	for _, ev := range child.events {
		child.ids[ev.EventID()] = struct{}{}
	}
	return child, nil
}

// JSONL writes one JSON object per line. Each line is an envelope carrying
// the base header plus a payload field with the kind-specific body. The
// format round-trips through ParseJSONL.
func (t *transcript) JSONL(w io.Writer) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, ev := range t.events {
		payload, err := json.Marshal(ev)
		if err != nil {
			return errors.Wrap(err, "marshal event payload")
		}
		env := envelope{
			ID:       ev.EventID(),
			ParentID: ev.ParentEventID(),
			Kind:     ev.Kind(),
			At:       ev.Timestamp(),
			Payload:  payload,
		}
		line, err := json.Marshal(env)
		if err != nil {
			return errors.Wrap(err, "marshal envelope")
		}
		if _, err := w.Write(line); err != nil {
			return errors.Wrap(err, "write line")
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			return errors.Wrap(err, "write newline")
		}
	}
	return nil
}

// ParseJSONL is the inverse of Transcript.JSONL: one JSON envelope per line,
// each decoded into the matching concrete event type. Used by tests and
// future persistence callers.
func ParseJSONL(r io.Reader) ([]Event, error) {
	dec := stdjson.NewDecoder(r)
	var out []Event
	for {
		var env envelope
		if err := dec.Decode(&env); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, errors.Wrap(err, "decode envelope")
		}
		ev, err := decodeEvent(env)
		if err != nil {
			return nil, errors.Wrap(err, "decode event")
		}
		out = append(out, ev)
	}
}

// decodeEvent dispatches on Kind and unmarshals the payload into the matching
// concrete type. New event types must register here.
func decodeEvent(env envelope) (Event, error) {
	switch env.Kind {
	case KindRunStarted:
		var ev RunStarted
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindRunStarted)
		}
		return ev, nil
	case KindStepStarted:
		var ev StepStarted
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindStepStarted)
		}
		return ev, nil
	case KindAssistantTextDelta:
		var ev AssistantTextDelta
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindAssistantTextDelta)
		}
		return ev, nil
	case KindAssistantReasoningDelta:
		var ev AssistantReasoningDelta
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindAssistantReasoningDelta)
		}
		return ev, nil
	case KindToolCallStart:
		var ev ToolCallStart
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindToolCallStart)
		}
		return ev, nil
	case KindToolCallEnd:
		var ev ToolCallEnd
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindToolCallEnd)
		}
		return ev, nil
	case KindToolResult:
		var ev ToolResult
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindToolResult)
		}
		return ev, nil
	case KindStepFinished:
		var ev StepFinished
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindStepFinished)
		}
		return ev, nil
	case KindFinal:
		var ev Final
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindFinal)
		}
		return ev, nil
	case KindRunFinished:
		var ev RunFinished
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindRunFinished)
		}
		return ev, nil
	case KindError:
		var ev Error
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, errors.Wrap(err, KindError)
		}
		return ev, nil
	default:
		return nil, errors.Errorf("unknown event kind %q", env.Kind)
	}
}
