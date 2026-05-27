package tool

import (
	"sort"
	"sync"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

// Registry is the process-global tool index used by the agent loop and the
// per-session belt assembly.
//
// Resolution invariant (proposal §3.2): tools are grouped by Source; within a
// source they are ordered lexicographically by Name. The lower-numbered
// Source wins on name collision and the rejected registration is silently
// skipped with a single agent_tool_shadowed warning. Get returns exactly one
// tool; Names is de-duplicated and globally ordered as the per-source sorted
// slices concatenated in Source order.
type Registry interface {
	Register(t Tool, src Source) error
	Get(name string) (Tool, bool)
	Names() []string
	Descriptors() []Descriptor
	Subset(names []string) (Registry, error)
}

// NewRegistry returns an empty Registry that logs collision warnings through
// the supplied logger. A nil logger is tolerated (warnings become silent),
// but production callers must provide one.
func NewRegistry(logger glog.Logger) Registry {
	return &registry{
		logger:  logger,
		entries: map[string]*entry{},
	}
}

// entry holds a single registered tool together with its winning source.
type entry struct {
	tool   Tool
	source Source
}

type registry struct {
	logger  glog.Logger
	mu      sync.RWMutex
	entries map[string]*entry
}

// Register installs t under its Name with the supplied source priority.
//
// On collision: if src is strictly higher priority (lower integer) than the
// existing entry, t replaces it; if strictly lower, the registration is
// silently skipped. Equal-source duplicates keep the existing entry (a
// re-registration is a no-op). Every collision attempt fires one
// agent_tool_shadowed warning.
func (r *registry) Register(t Tool, src Source) error {
	if t == nil {
		return errors.New("nil tool")
	}
	name := t.Name()
	if name == "" {
		return errors.New("tool has empty name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.entries[name]
	if !ok {
		r.entries[name] = &entry{tool: t, source: src}
		return nil
	}

	// Collision: deterministic resolution. Lower Source value wins.
	var kept, dropped Source
	if src < existing.source {
		kept = src
		dropped = existing.source
		r.entries[name] = &entry{tool: t, source: src}
	} else {
		kept = existing.source
		dropped = src
	}
	if r.logger != nil {
		r.logger.Warn("agent_tool_shadowed",
			zap.String("name", name),
			zap.String("kept_source", kept.String()),
			zap.String("dropped_source", dropped.String()),
		)
	}
	return nil
}

// Get returns the resolved tool for name. It never logs and is safe for
// hot-path use.
func (r *registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	if !ok {
		return nil, false
	}
	return e.tool, true
}

// Names returns the resolved tool names: per-source sorted slices
// concatenated in Source order (Local, then CuratedMCP, then UserMCP, then
// any future source by integer).
func (r *registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.namesLocked()
}

// Descriptors returns one Descriptor per resolved tool in the same order as
// Names.
func (r *registry) Descriptors() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := r.namesLocked()
	out := make([]Descriptor, 0, len(names))
	for _, n := range names {
		e := r.entries[n]
		out = append(out, Descriptor{
			Name:        n,
			Description: e.tool.Description(),
			Schema:      e.tool.Schema(),
			Source:      e.source,
		})
	}
	return out
}

// Subset returns a new Registry restricted to the supplied names. It errors
// if any name is unknown; the receiver registry is never mutated. The
// returned registry shares the same logger and preserves the source of every
// retained entry, so further registrations against it follow the same
// resolution rules.
func (r *registry) Subset(names []string) (Registry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	picked := make(map[string]*entry, len(names))
	for _, n := range names {
		e, ok := r.entries[n]
		if !ok {
			return nil, errors.Errorf("unknown tool: %q", n)
		}
		picked[n] = &entry{tool: e.tool, source: e.source}
	}
	return &registry{
		logger:  r.logger,
		entries: picked,
	}, nil
}

// namesLocked computes the deterministic ordering described on Names. It
// must be called with r.mu held.
func (r *registry) namesLocked() []string {
	if len(r.entries) == 0 {
		return nil
	}
	bySource := map[Source][]string{}
	for n, e := range r.entries {
		bySource[e.source] = append(bySource[e.source], n)
	}
	sources := make([]Source, 0, len(bySource))
	for s := range bySource {
		sources = append(sources, s)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
	out := make([]string, 0, len(r.entries))
	for _, s := range sources {
		group := bySource[s]
		sort.Strings(group)
		out = append(out, group...)
	}
	return out
}
