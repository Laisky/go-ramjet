package http

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
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

// responseCacheKey derives the llmRespCache key of req answered for user.
//
// Besides the request body, the key covers the tenant, identified by the caller's
// API key as AGENTS.md requires, and the upstream API base. A BYOK caller chooses
// its upstream through X-Laisky-Api-Base and so may author the answer that gets
// cached; scoping the key means an answer is only served to the API key and
// upstream that produced it. The API key enters the key only through a SHA-256
// digest.
//
// It returns an error when user is nil or req cannot be encoded.
func responseCacheKey(user *config.UserConfig, req *FrontendReq) (string, error) {
	if user == nil {
		return "", errors.New("response cache key needs the requesting user")
	}
	reqKey, err := req2CacheKey(req)
	if err != nil {
		return "", errors.Wrap(err, "request cache key")
	}

	var material []byte
	for _, part := range []string{user.Token, user.APIBase, reqKey} {
		// Length-prefix every part, so no two part lists encode alike.
		material = strconv.AppendInt(material, int64(len(part)), 10)
		material = append(material, ':')
		material = append(material, part...)
	}
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:]), nil
}

// loadCachedResponse returns the answer cached for req sent by user.
//
// It returns ok=false when responseCacheAllowed refuses req or nothing is cached
// for this tenant and upstream, and an error when the cache key cannot be derived.
func loadCachedResponse(user *config.UserConfig, req *FrontendReq) (answer string, ok bool, err error) {
	if !responseCacheAllowed(req) {
		return "", false, nil
	}

	key, err := responseCacheKey(user, req)
	if err != nil {
		return "", false, errors.Wrap(err, "derive response cache key")
	}

	answer, ok = llmRespCache.Load(key)
	return answer, ok, nil
}

// storeCachedResponse caches answer for req sent by user when responseCacheAllowed
// permits it; only the same API key and upstream can read it back.
//
// It returns an error when the cache key cannot be derived.
func storeCachedResponse(user *config.UserConfig, req *FrontendReq, answer string) error {
	if !responseCacheAllowed(req) {
		return nil
	}

	key, err := responseCacheKey(user, req)
	if err != nil {
		return errors.Wrap(err, "derive response cache key")
	}

	llmRespCache.Store(key, answer)
	return nil
}
