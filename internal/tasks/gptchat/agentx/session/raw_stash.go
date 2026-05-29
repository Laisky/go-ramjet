package session

import "sync"

// RawStash is the per-request side-storage that retains the raw bytes of
// oversize tool outputs after the distiller has replaced them in the
// model-facing transcript with a short, high-density observation.
//
// The classic ReAct "Observation" step trades raw bytes for density;
// keeping the raw bytes addressable by call_id buys back two things:
// (a) post-hoc inspectability (UI / audit) and (b) a future JIT
// retrieval tool (`read_raw_observation`) so the model can opt back into
// raw on demand when the summariser dropped a load-bearing detail.
//
// RawStash is intentionally not on the Transcript: the Transcript is
// strictly append-only and event-typed, and a per-call key/value lookup
// would distort that contract. RawStash is a sibling concept with its
// own lifetime — one instance per request, owned by the handler.
//
// Safe for concurrent use; Phase 1 mode has tools fanning out under the
// parallel executor and they all stash through one shared RawStash.
type RawStash struct {
	m sync.Map // call_id (string) -> raw content (string)
}

// NewRawStash returns an empty stash.
func NewRawStash() *RawStash { return &RawStash{} }

// Stash records raw content under callID. Repeated stashes for the same
// callID overwrite — call_ids are unique-per-run by upstream contract,
// so a duplicate stash means the same observation was distilled twice
// (idempotent overwrite is the right semantics).
//
// A nil receiver or an empty callID are no-ops so callers do not need to
// nil-check or sanity-check before stashing.
func (r *RawStash) Stash(callID, content string) {
	if r == nil || callID == "" {
		return
	}
	r.m.Store(callID, content)
}

// Get returns the raw content for callID, or ("", false) on miss.
// A nil receiver is a no-op miss.
func (r *RawStash) Get(callID string) (string, bool) {
	if r == nil || callID == "" {
		return "", false
	}
	v, ok := r.m.Load(callID)
	if !ok {
		return "", false
	}
	s, _ := v.(string)
	return s, true
}

// Len returns the number of entries currently stashed. Intended for
// tests and metrics; not safe to read for control flow because Stash
// calls race against it.
func (r *RawStash) Len() int {
	if r == nil {
		return 0
	}
	n := 0
	r.m.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}
