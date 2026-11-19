package http

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

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

// responseCacheKey derives the llmRespCache key of req sent with the caller's raw
// apiKey to the upstream apiBase.
//
// Besides the request body, the key covers the tenant, identified by the caller's
// own API key as AGENTS.md requires, and the upstream API base. The raw key is used
// rather than user.Token because every free-tier caller resolves to a copy of the
// shared free-tier user, while an MCP server without its own key is called with
// the caller's raw key, so an answer may carry that caller's private data. A BYOK
// caller also chooses its upstream through X-Laisky-Api-Base and so may author the
// cached answer. An answer is therefore only served to the key and upstream that
// produced it. The API key enters the key only through a SHA-256 digest.
//
// It returns an error when apiKey is empty or req cannot be encoded.
func responseCacheKey(apiKey, apiBase string, req *FrontendReq) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", errors.New("response cache key needs the caller's API key")
	}
	reqKey, err := req2CacheKey(req)
	if err != nil {
		return "", errors.Wrap(err, "request cache key")
	}

	var material []byte
	for _, part := range []string{apiKey, apiBase, reqKey} {
		// Length-prefix every part, so no two part lists encode alike.
		material = strconv.AppendInt(material, int64(len(part)), 10)
		material = append(material, ':')
		material = append(material, part...)
	}
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:]), nil
}

// requestResponseCacheKey derives the llmRespCache key of req for the request in
// gctx, which user sent: the caller's raw API key comes from GetRawUserToken and
// the upstream from user.APIBase.
//
// It returns an error when user is nil, the caller's key is unknown, or req cannot
// be encoded.
func requestResponseCacheKey(gctx *gin.Context, user *config.UserConfig, req *FrontendReq) (string, error) {
	if user == nil {
		return "", errors.New("response cache key needs the requesting user")
	}

	key, err := responseCacheKey(GetRawUserToken(gctx), user.APIBase, req)
	if err != nil {
		return "", errors.Wrap(err, "response cache key")
	}
	return key, nil
}

// loadCachedResponse returns the answer cached for req, which user sent in the
// request of gctx.
//
// It returns ok=false when responseCacheAllowed refuses req or nothing is cached
// for this caller key and upstream, and an error when the cache key cannot be
// derived.
func loadCachedResponse(gctx *gin.Context, user *config.UserConfig, req *FrontendReq) (answer string, ok bool, err error) {
	if !responseCacheAllowed(req) {
		return "", false, nil
	}

	key, err := requestResponseCacheKey(gctx, user, req)
	if err != nil {
		return "", false, errors.Wrap(err, "derive response cache key")
	}

	answer, ok = llmRespCache.Load(key)
	return answer, ok, nil
}

// storeCachedResponse caches answer for req, which user sent in the request of
// gctx, when responseCacheAllowed permits it; only the same caller key and
// upstream can read it back.
//
// It returns an error when the cache key cannot be derived.
func storeCachedResponse(gctx *gin.Context, user *config.UserConfig, req *FrontendReq, answer string) error {
	if !responseCacheAllowed(req) {
		return nil
	}

	key, err := requestResponseCacheKey(gctx, user, req)
	if err != nil {
		return errors.Wrap(err, "derive response cache key")
	}

	llmRespCache.Store(key, answer)
	return nil
}
