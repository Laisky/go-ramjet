package distiller

import "sync"

// Cache is a process-lifetime, in-memory key/value store keyed by the
// composite string the LLMDistiller assembles in cacheKey
// (tool_name : content_hash : model : prompt_version : target_tokens :
// anchors_hash). Repeated tool calls with identical raw output and
// identical salience anchors return the same summary without re-calling
// the summariser model.
//
// The cache is unbounded by design — the (PromptVersion, model) tuple is
// fixed for a process and individual entries are at most a few kilobytes,
// so a typical session generates at most low-double-digit entries.
// Process restart turns it over, which is acceptable for v1. If memory
// pressure ever becomes a concern, swap the sync.Map for an LRU.
type Cache struct {
	m sync.Map
}

// NewCache returns an empty Cache ready for use.
func NewCache() *Cache { return &Cache{} }

// Get returns the cached entry for key and true on hit; empty string and
// false on miss. A nil receiver is a no-op miss so callers do not need
// to nil-check before consulting the cache.
func (c *Cache) Get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	v, ok := c.m.Load(key)
	if !ok {
		return "", false
	}
	s, _ := v.(string)
	return s, true
}

// Put stores value under key. Repeated puts overwrite. A nil receiver is
// a no-op so callers can ignore the cache by passing nil at construction.
func (c *Cache) Put(key, value string) {
	if c == nil {
		return
	}
	c.m.Store(key, value)
}
