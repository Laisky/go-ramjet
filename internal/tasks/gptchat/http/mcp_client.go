package http

import (
	"context"
	stdjson "encoding/json"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/zap"
)

// MCPCallOption contains optional parameters for callMCPTool.
type MCPCallOption struct {
	// FallbackAPIKey is used when server.APIKey is empty.
	// Typically this is the session's API key (user's token).
	FallbackAPIKey string
}

// callMCPTool executes a tool against a remote MCP server.
// When server.APIKey is empty, it falls back to opts.FallbackAPIKey if provided.
func callMCPTool(ctx context.Context, server *MCPServerConfig, toolName string, args string, opts *MCPCallOption) (string, error) {
	logger := gmw.GetLogger(ctx)

	if server == nil {
		return "", errors.New("nil mcp server")
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return "", errors.New("empty tool name")
	}

	// Determine API key: server's own key takes precedence, then fallback to session key.
	effectiveAPIKey := strings.TrimSpace(server.APIKey)
	if effectiveAPIKey == "" && opts != nil {
		effectiveAPIKey = strings.TrimSpace(opts.FallbackAPIKey)
	}

	if effectiveAPIKey == "" {
		logger.Debug("mcp tool call with no api key",
			zap.String("tool", name),
			zap.String("server_url", server.URL))
	} else {
		logger.Debug("mcp tool call with api key",
			zap.String("tool", name),
			zap.String("server_url", server.URL),
			zap.String("api_key_prefix", truncateForLog(effectiveAPIKey, 8)))
	}

	auths := mcpAuthCandidates(effectiveAPIKey)
	headers := http.Header{}
	headers.Set("content-type", "application/json")
	headers.Set("accept", "application/json, text/event-stream")

	// Normalize args.
	var parsedArgs any
	if strings.TrimSpace(args) == "" {
		parsedArgs = map[string]any{}
	} else {
		if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
			parsedArgs = map[string]any{"_raw": args}
		}
	}

	body := map[string]any{"name": name, "arguments": parsedArgs}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", errors.Wrap(err, "marshal tool call body")
	}

	// Log request body (truncated for safety)
	logger.Debug("mcp tool call request",
		zap.String("tool", name),
		zap.Any("args_truncated", truncateMapForLog(parsedArgs, maxLogValueLen)))

	// Fast path: a previous successful call pinned a transport+endpoint
	// for this server.URL. Avoid re-probing the 8 REST URLs and JSON-RPC
	// fallbacks (~2s wasted per call against servers like mcp.laisky.com).
	if cached, ok := loadMCPTransport(server.URL); ok {
		switch cached.Transport {
		case mcpTransportREST:
			if result, err, hit := tryCachedREST(ctx, server, cached.Endpoint, headers, auths, bodyBytes); hit {
				if err == nil {
					return result, nil
				}
				return "", err
			}
		case mcpTransportJSONRPC:
			result, rpcErr := callMCPToolJSONRPCAt(ctx, server, headers, auths, body, []string{cached.Endpoint})
			if rpcErr == nil {
				return result, nil
			}
			invalidateMCPTransport(server.URL)
			logger.Debug("cached json-rpc endpoint failed; falling back to full discovery",
				zap.String("endpoint", cached.Endpoint),
				zap.Error(rpcErr))
		}
	}

	var lastRestErr error

	// 1) Try REST-ish endpoints.
	for _, endpoint := range guessMCPToolCallURLs(server.URL, server.URLPrefix) {
		logger.Debug("mcp rest call attempt",
			zap.String("endpoint", endpoint))

		resp, callErr := doMCPPost(ctx, endpoint, headers, auths, bodyBytes)
		if callErr != nil {
			logger.Debug("mcp rest call failed",
				zap.String("endpoint", endpoint),
				zap.Error(callErr))
			lastRestErr = callErr
			continue
		}
		defer resp.Body.Close()
		obj, parseErr := fetchJSONOrSSE(resp)
		if parseErr != nil {
			return "", errors.Wrapf(parseErr, "parse mcp response from %s", endpoint)
		}

		// Log response (truncated)
		logger.Debug("mcp rest call response",
			zap.String("endpoint", endpoint),
			zap.Any("response_truncated", truncateMapForLog(obj, maxLogValueLen)))

		// Check for isError in response
		result, mcpErr := processMCPResponse(obj)
		if mcpErr != nil {
			logger.Debug("mcp tool returned error",
				zap.String("endpoint", endpoint),
				zap.Error(mcpErr))
			return "", mcpErr
		}
		storeMCPTransport(server.URL, mcpTransportREST, endpoint)
		return result, nil
	}

	// 2) Fallback to JSON-RPC.
	result, rpcErr := callMCPToolJSONRPC(ctx, server, headers, auths, body)
	if rpcErr != nil {
		// Include both errors for better debugging
		if lastRestErr != nil {
			return "", errors.Wrapf(rpcErr, "json-rpc failed (rest also failed: %v)", lastRestErr)
		}
		return "", errors.Wrap(rpcErr, "json-rpc failed")
	}
	return result, nil
}

// tryCachedREST executes a single cached REST endpoint. The third return
// distinguishes "endpoint reached server (cache still valid)" from
// "transport-level failure that should invalidate the cache". When hit is
// false the cache has been dropped and the caller must re-probe.
func tryCachedREST(ctx context.Context, server *MCPServerConfig, endpoint string, headers http.Header, auths []string, bodyBytes []byte) (string, error, bool) {
	logger := gmw.GetLogger(ctx)
	resp, callErr := doMCPPost(ctx, endpoint, headers, auths, bodyBytes)
	if callErr != nil {
		invalidateMCPTransport(server.URL)
		logger.Debug("cached rest endpoint failed; falling back to full discovery",
			zap.String("endpoint", endpoint),
			zap.Error(callErr))
		return "", nil, false
	}
	defer resp.Body.Close()
	obj, parseErr := fetchJSONOrSSE(resp)
	if parseErr != nil {
		return "", errors.Wrapf(parseErr, "parse mcp response from %s", endpoint), true
	}
	logger.Debug("mcp rest call response (cached)",
		zap.String("endpoint", endpoint),
		zap.Any("response_truncated", truncateMapForLog(obj, maxLogValueLen)))
	result, mcpErr := processMCPResponse(obj)
	if mcpErr != nil {
		logger.Debug("mcp tool returned error",
			zap.String("endpoint", endpoint),
			zap.Error(mcpErr))
		return "", mcpErr, true
	}
	return result, nil, true
}

func callMCPToolJSONRPC(ctx context.Context, server *MCPServerConfig, baseHeaders http.Header, auths []string, params map[string]any) (string, error) {
	// Direct invocations (no REST attempt first) still benefit from the
	// cache: skip the endpoint sweep when we already know which one works.
	if cached, ok := loadMCPTransport(server.URL); ok && cached.Transport == mcpTransportJSONRPC {
		result, err := callMCPToolJSONRPCAt(ctx, server, baseHeaders, auths, params, []string{cached.Endpoint})
		if err == nil {
			return result, nil
		}
		invalidateMCPTransport(server.URL)
		gmw.GetLogger(ctx).Debug("cached json-rpc endpoint failed; falling back to full discovery",
			zap.String("endpoint", cached.Endpoint),
			zap.Error(err))
	}
	return callMCPToolJSONRPCAt(ctx, server, baseHeaders, auths, params, nil)
}

// callMCPToolJSONRPCAt is the actual JSON-RPC call worker. When
// endpoints is nil it falls back to the full guessJSONRPCEndpoints sweep;
// when non-nil it walks only the supplied candidates (used by the
// transport cache fast path).
func callMCPToolJSONRPCAt(ctx context.Context, server *MCPServerConfig, baseHeaders http.Header, auths []string, params map[string]any, endpoints []string) (string, error) {
	logger := gmw.GetLogger(ctx)
	endpointCandidates := endpoints
	if endpointCandidates == nil {
		endpointCandidates = guessJSONRPCEndpoints(server)
	}
	if len(endpointCandidates) == 0 {
		return "", errors.New("no json-rpc endpoints")
	}

	var lastErr error
	methods := []string{"tools/call", "tools.call"}
	for _, endpoint := range endpointCandidates {
		if err := ensureMCPSession(ctx, server, endpoint, baseHeaders, auths); err != nil {
			logger.Debug("mcp session init failed",
				zap.String("endpoint", endpoint),
				zap.Error(err))
			lastErr = err
			continue
		}

		for _, method := range methods {
			payload := map[string]any{
				"jsonrpc": "2.0",
				"id":      randomID("rpc_", 8),
				"method":  method,
				"params":  params,
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return "", errors.Wrap(err, "marshal rpc payload")
			}

			logger.Debug("mcp json-rpc call",
				zap.String("endpoint", endpoint),
				zap.String("method", method),
				zap.Any("params_name", params["name"]))

			resp, err := doMCPPost(ctx, endpoint, mcpSessionHeaders(server, baseHeaders), auths, body)
			if err != nil {
				logger.Debug("mcp json-rpc post failed",
					zap.String("endpoint", endpoint),
					zap.String("method", method),
					zap.Error(err))
				lastErr = err
				continue
			}
			defer resp.Body.Close()
			obj, err := fetchJSONOrSSE(resp)
			if err != nil {
				return "", errors.Wrap(err, "parse rpc response")
			}

			// Log response (truncated)
			logger.Debug("mcp json-rpc response",
				zap.String("endpoint", endpoint),
				zap.String("method", method),
				zap.Any("response_truncated", truncateMapForLog(obj, maxLogValueLen)))

			if e, ok := obj["error"]; ok && e != nil {
				logger.Debug("mcp json-rpc returned error",
					zap.String("endpoint", endpoint),
					zap.Any("error", e))
				lastErr = errors.Errorf("mcp rpc error: %v", e)
				continue
			}

			// Extract result field if present
			resultObj := obj
			if res, ok := obj["result"]; ok {
				if resMap, isMap := res.(map[string]any); isMap {
					resultObj = resMap
				} else {
					// Non-map result, stringify directly
					storeMCPTransport(server.URL, mcpTransportJSONRPC, endpoint)
					return stringifyMCPResult(res), nil
				}
			}

			// Check for isError in result
			result, mcpErr := processMCPResponse(resultObj)
			if mcpErr != nil {
				logger.Debug("mcp json-rpc tool returned error",
					zap.String("endpoint", endpoint),
					zap.Error(mcpErr))
				return "", mcpErr
			}
			storeMCPTransport(server.URL, mcpTransportJSONRPC, endpoint)
			return result, nil
		}
	}

	if lastErr != nil {
		return "", errors.Wrap(lastErr, "failed to call mcp tool via json-rpc")
	}
	return "", errors.New("failed to call mcp tool via json-rpc")
}

// MCPToolDescriptor is one entry in an MCP server's tool list.
//
// Field names match the MCP spec (camelCase) so JSON unmarshal works
// directly against tools/list responses. InputSchema is held as raw JSON
// because it is forwarded verbatim to the upstream LLM's tool catalog.
type MCPToolDescriptor struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	InputSchema stdjson.RawMessage `json:"inputSchema,omitempty"`
}

// DiscoverMCPTools fetches the tool catalog from an MCP server via the
// JSON-RPC `tools/list` method (with a `tools.list` fallback for older
// servers). It mirrors the call-side authentication and endpoint-guessing
// of callMCPTool so identical credentials and URL forms work for both.
//
// Consumed by the agent loop's curated-belt builder (agentx/tools); see
// proposal §3.2 and §5.1. The proxy path does not call this.
func DiscoverMCPTools(ctx context.Context, server *MCPServerConfig, opts *MCPCallOption) ([]MCPToolDescriptor, error) {
	logger := gmw.GetLogger(ctx)
	if server == nil {
		return nil, errors.New("nil mcp server")
	}

	effectiveAPIKey := strings.TrimSpace(server.APIKey)
	if effectiveAPIKey == "" && opts != nil {
		effectiveAPIKey = strings.TrimSpace(opts.FallbackAPIKey)
	}

	auths := mcpAuthCandidates(effectiveAPIKey)
	baseHeaders := http.Header{}
	baseHeaders.Set("content-type", "application/json")
	baseHeaders.Set("accept", "application/json, text/event-stream")

	// Prefer the cached JSON-RPC endpoint when one is known, otherwise
	// sweep the candidate list. REST cache hits don't apply here because
	// tools/list has no REST equivalent in this client.
	var endpointCandidates []string
	if cached, ok := loadMCPTransport(server.URL); ok && cached.Transport == mcpTransportJSONRPC {
		endpointCandidates = []string{cached.Endpoint}
	} else {
		endpointCandidates = guessJSONRPCEndpoints(server)
	}
	if len(endpointCandidates) == 0 {
		return nil, errors.New("no json-rpc endpoints derivable from server url")
	}

	var lastErr error
	methods := []string{"tools/list", "tools.list"}
	for _, endpoint := range endpointCandidates {
		if err := ensureMCPSession(ctx, server, endpoint, baseHeaders, auths); err != nil {
			logger.Debug("mcp session init failed for discovery",
				zap.String("endpoint", endpoint),
				zap.Error(err))
			lastErr = err
			continue
		}

		for _, method := range methods {
			payload := map[string]any{
				"jsonrpc": "2.0",
				"id":      randomID("rpc_", 8),
				"method":  method,
				"params":  map[string]any{"_meta": map[string]any{"progressToken": 1}},
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return nil, errors.Wrap(err, "marshal tools/list payload")
			}

			resp, err := doMCPPost(ctx, endpoint, mcpSessionHeaders(server, baseHeaders), auths, body)
			if err != nil {
				logger.Debug("mcp tools/list post failed",
					zap.String("endpoint", endpoint),
					zap.String("method", method),
					zap.Error(err))
				lastErr = err
				continue
			}
			//nolint:errcheck // best-effort close; the helper drains on error paths
			defer resp.Body.Close()

			obj, err := fetchJSONOrSSE(resp)
			if err != nil {
				return nil, errors.Wrap(err, "parse tools/list response")
			}

			if e, ok := obj["error"]; ok && e != nil {
				lastErr = errors.Errorf("mcp tools/list rpc error: %v", e)
				continue
			}

			tools, perr := extractToolListFromResponse(obj)
			if perr != nil {
				lastErr = perr
				continue
			}
			logger.Debug("mcp tools/list ok",
				zap.String("endpoint", endpoint),
				zap.String("method", method),
				zap.Int("tool_count", len(tools)))
			storeMCPTransport(server.URL, mcpTransportJSONRPC, endpoint)
			return tools, nil
		}
	}

	if lastErr != nil {
		// If we got here via the cached fast path, drop the entry so a
		// retry re-discovers the working endpoint.
		if len(endpointCandidates) == 1 {
			invalidateMCPTransport(server.URL)
		}
		return nil, errors.Wrap(lastErr, "failed to discover mcp tools")
	}
	return nil, errors.New("failed to discover mcp tools")
}

// extractToolListFromResponse normalizes the multiple shapes MCP servers
// return for tools/list: { result: { tools: [...] } }, { tools: [...] },
// or a bare array. Mirrors the frontend's normalizeMCPToolListResponse.
func extractToolListFromResponse(obj map[string]any) ([]MCPToolDescriptor, error) {
	var raw any = obj
	if res, ok := obj["result"]; ok && res != nil {
		raw = res
	}
	if m, isMap := raw.(map[string]any); isMap {
		if t, ok := m["tools"]; ok {
			raw = t
		}
	}

	// Re-marshal then unmarshal — the simplest way to type a heterogeneous slice.
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, errors.Wrap(err, "marshal tool list intermediate")
	}
	var tools []MCPToolDescriptor
	if err := json.Unmarshal(buf, &tools); err != nil {
		return nil, errors.Wrap(err, "unmarshal tool list (unexpected shape)")
	}
	return tools, nil
}
