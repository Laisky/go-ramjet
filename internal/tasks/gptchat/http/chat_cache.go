package http

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// responseCacheAllowed is the single policy deciding whether the answer to req may
// be stored in or served from llmRespCache; every read and write goes through it.
//
// Caching is refused while transparent memory is enabled, because answers are
// personalized, and for requests carrying MCP API keys, so that answers derived
// from data fetched with a caller's credentials are never kept. A request without
// messages is not cacheable either.
//
// It returns true when the answer may be cached.
func responseCacheAllowed(req *FrontendReq) bool {
	if req == nil || len(req.Messages) == 0 {
		return false
	}
	if config.Config != nil && config.Config.EnableMemory {
		return false
	}
	for _, srv := range req.MCPServers {
		if strings.TrimSpace(srv.APIKey) != "" {
			return false
		}
	}

	return true
}

// loadCachedResponse returns the cached answer for req.
//
// It returns ok=false when responseCacheAllowed refuses req or nothing is cached,
// and an error when the cache key cannot be derived.
func loadCachedResponse(req *FrontendReq) (answer string, ok bool, err error) {
	if !responseCacheAllowed(req) {
		return "", false, nil
	}

	key, err := req2CacheKey(req)
	if err != nil {
		return "", false, errors.Wrap(err, "derive response cache key")
	}

	answer, ok = llmRespCache.Load(key)
	return answer, ok, nil
}

// storeCachedResponse caches answer for req when responseCacheAllowed permits it.
//
// It returns an error when the cache key cannot be derived.
func storeCachedResponse(req *FrontendReq, answer string) error {
	if !responseCacheAllowed(req) {
		return nil
	}

	key, err := req2CacheKey(req)
	if err != nil {
		return errors.Wrap(err, "derive response cache key")
	}

	llmRespCache.Store(key, answer)
	return nil
}
