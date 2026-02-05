package http

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/json"
)

const (
	// maxLogValueLen is the maximum length for logged string values.
	// Values longer than this are truncated to prevent log bloat (e.g., base64 images).
	maxLogValueLen = 256
)

// truncateForLog truncates a string for logging, appending "..." if truncated.
func truncateForLog(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = maxLogValueLen
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// truncateMapForLog recursively truncates string values in a map for logging.
// It returns a new map without modifying the original.
func truncateMapForLog(v any, maxLen int) any {
	if maxLen <= 0 {
		maxLen = maxLogValueLen
	}
	return truncateValue(v, maxLen)
}

func truncateValue(v any, maxLen int) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		return truncateForLog(val, maxLen)
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, vv := range val {
			result[k] = truncateValue(vv, maxLen)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = truncateValue(item, maxLen)
		}
		return result
	case []byte:
		// Handle raw JSON bytes (e.g., json.RawMessage)
		var parsed any
		if err := json.Unmarshal(val, &parsed); err == nil {
			return truncateValue(parsed, maxLen)
		}
		s := string(val)
		return truncateForLog(s, maxLen)
	default:
		// For other types, try reflection for nested structures
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Map {
			result := make(map[string]any)
			iter := rv.MapRange()
			for iter.Next() {
				key := fmt.Sprintf("%v", iter.Key().Interface())
				result[key] = truncateValue(iter.Value().Interface(), maxLen)
			}
			return result
		}
		if rv.Kind() == reflect.Slice {
			result := make([]any, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				result[i] = truncateValue(rv.Index(i).Interface(), maxLen)
			}
			return result
		}
		return v
	}
}

// mcpAuthCandidates returns possible Authorization header values.
func mcpAuthCandidates(apiKey string) []string {
	k := strings.TrimSpace(apiKey)
	if k == "" {
		return nil
	}
	c := []string{"Bearer " + k, k}
	// Dedup.
	seen := make(map[string]struct{}, len(c))
	out := make([]string, 0, len(c))
	for _, v := range c {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func guessMCPToolCallURLs(rawURL, urlPrefix string) []string {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return nil
	}
	u = strings.TrimRight(u, "/")
	suffixes := []string{"/v1/tools/call", "/tools/call", "/v1/tool/call", "/tool/call", "/v1/tools/execute", "/tools/execute"}
	candidates := make([]string, 0, len(suffixes)+2)
	push := func(base string) {
		base = strings.TrimRight(strings.TrimSpace(base), "/")
		if base == "" {
			return
		}
		for _, s := range suffixes {
			if !strings.HasSuffix(base, s) {
				candidates = append(candidates, base+s)
			}
		}
		candidates = append(candidates, base)
	}

	push(u)
	if urlPrefix != "" {
		origin := urlOrigin(u)
		if origin != "" {
			push(joinURL(origin, urlPrefix))
		}
	}

	return uniqStrings(candidates)
}

func urlOrigin(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func joinURL(base, path string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	p := strings.TrimSpace(path)
	if b == "" {
		return ""
	}
	if p == "" {
		return b
	}
	if strings.HasPrefix(p, "/") {
		return b + p
	}
	return b + "/" + p
}

func uniqStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func randomID(prefix string, nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s%s", prefix, hex.EncodeToString(b))
}

// readFirstJSONFromSSE reads the first JSON payload from `data:` lines in an SSE stream.
func readFirstJSONFromSSE(r io.Reader, maxBytes int, timeout time.Duration) (map[string]any, error) {
	if maxBytes <= 0 {
		maxBytes = 512 * 1024
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	dataCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(io.LimitReader(r, int64(maxBytes)))
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, maxBytes)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" || payload == "[DONE]" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(payload), &obj); err != nil {
				continue
			}
			dataCh <- obj
			return
		}
		if err := scanner.Err(); err != nil {
			errCh <- errors.Wrap(err, "scan sse")
			return
		}
		errCh <- errors.New("no json found in sse")
	}()

	select {
	case obj := <-dataCh:
		return obj, nil
	case err := <-errCh:
		return nil, err
	case <-timer.C:
		return nil, errors.New("sse timeout")
	}
}

func fetchJSONOrSSE(resp *http.Response) (map[string]any, error) {
	ct := strings.ToLower(resp.Header.Get("content-type"))
	if strings.Contains(ct, "application/json") {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Wrap(err, "read json body")
		}
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, errors.Wrap(err, "unmarshal json")
		}
		return obj, nil
	}

	if strings.Contains(ct, "text/event-stream") {
		return readFirstJSONFromSSE(resp.Body, 512*1024, 10*time.Second)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read body")
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err == nil {
		return obj, nil
	}
	return map[string]any{"text": string(data)}, nil
}

// processMCPResponse extracts the result from an MCP response and checks for errors.
// It returns an error if the response contains isError:true or an error field.
// It also handles JSON-RPC responses where isError might be nested inside result.
func processMCPResponse(obj map[string]any) (string, error) {
	if obj == nil {
		return "", nil // Return empty for nil input
	}

	// Check for isError field at top level (MCP error response format)
	if isErr, ok := obj["isError"]; ok {
		if b, isBool := isErr.(bool); isBool && b {
			// Extract error message from content
			errContent := stringifyMCPResult(obj)
			return "", errors.Errorf("mcp tool error: %s", errContent)
		}
	}

	// Check for JSON-RPC error field
	if errField, ok := obj["error"]; ok && errField != nil {
		return "", errors.Errorf("mcp error: %v", errField)
	}

	// Check for nested isError inside result (JSON-RPC format with MCP error inside)
	if res, ok := obj["result"]; ok {
		if resMap, isMap := res.(map[string]any); isMap {
			if isErr, ok := resMap["isError"]; ok {
				if b, isBool := isErr.(bool); isBool && b {
					errContent := stringifyMCPResult(resMap)
					return "", errors.Errorf("mcp tool error: %s", errContent)
				}
			}
		}
	}

	return stringifyMCPResult(obj), nil
}

func doMCPPost(ctx context.Context, endpoint string, baseHeaders http.Header, auths []string, body []byte) (*http.Response, error) {
	tryAuths := auths
	if len(tryAuths) == 0 {
		tryAuths = []string{""}
	}
	var lastErr error
	for _, auth := range tryAuths {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			lastErr = errors.Wrap(err, "new request")
			continue
		}
		for k, vs := range baseHeaders {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		if auth != "" {
			req.Header.Set("authorization", auth)
		}

		resp, err := httpcli.Do(req) //nolint:bodyclose
		if err != nil {
			lastErr = errors.Wrap(err, "do request")
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			lastErr = errors.Errorf("http %d", resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func guessJSONRPCEndpoints(server *MCPServerConfig) []string {
	base := strings.TrimRight(strings.TrimSpace(server.URL), "/")
	candidates := []string{}

	// Try the exact URL first (highest priority)
	if base != "" {
		candidates = append(candidates, base)
	}

	origin := urlOrigin(base)

	// Try root endpoint second (for servers like mcp.laisky.com that use /)
	if origin != "" {
		candidates = append(candidates, origin+"/")
	}

	// Then try common MCP endpoint paths as fallbacks
	if origin != "" {
		candidates = append(candidates,
			joinURL(origin, "/mcp"),       // Standard MCP endpoint
			joinURL(origin, "/mcp/tools"), // Alternative
		)
	}

	if origin != "" && server.URLPrefix != "" {
		candidates = append(candidates, joinURL(origin, server.URLPrefix))
	}

	return uniqStrings(candidates)
}

func mcpSessionHeaders(server *MCPServerConfig, base http.Header) http.Header {
	h := base.Clone()
	pv := strings.TrimSpace(server.MCPProtocolVersion)
	if pv == "" {
		pv = "2025-06-18"
		server.MCPProtocolVersion = pv
	}
	h.Set("mcp-protocol-version", pv)

	// Only set session ID if already initialized (retrieved from server response)
	sid := strings.TrimSpace(server.MCPSessionID)
	if sid != "" {
		h.Set("mcp-session-id", sid)
	}
	return h
}

func ensureMCPSession(ctx context.Context, server *MCPServerConfig, endpoint string, baseHeaders http.Header, auths []string) error {
	// If session ID already exists, we're already initialized
	if strings.TrimSpace(server.MCPSessionID) != "" {
		return nil
	}

	pv := strings.TrimSpace(server.MCPProtocolVersion)
	if pv == "" {
		pv = "2025-06-18"
		server.MCPProtocolVersion = pv
	}

	// Build headers for initialize request (no session ID yet)
	h := baseHeaders.Clone()
	h.Set("mcp-protocol-version", pv)

	initPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      0,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": pv,
			"capabilities": map[string]any{
				"sampling":    map[string]any{},
				"elicitation": map[string]any{},
				"roots":       map[string]any{"listChanged": true},
			},
			"clientInfo": map[string]any{"name": "go-ramjet-gptchat", "version": "0.0.0"},
		},
	}

	body, err := json.Marshal(initPayload)
	if err != nil {
		return errors.Wrap(err, "marshal initialize")
	}
	resp, err := doMCPPost(ctx, endpoint, h, auths, body)
	if err != nil {
		return errors.Wrap(err, "initialize")
	}

	// Capture session ID from response header
	serverSessionID := resp.Header.Get("mcp-session-id")
	if serverSessionID != "" {
		server.MCPSessionID = serverSessionID
	}
	_ = resp.Body.Close()

	// Send notifications/initialized with the session ID
	if server.MCPSessionID != "" {
		h.Set("mcp-session-id", server.MCPSessionID)
	}

	notify := map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}
	b2, err := json.Marshal(notify)
	if err != nil {
		return errors.Wrap(err, "marshal initialized")
	}
	resp2, err := doMCPPost(ctx, endpoint, h, auths, b2)
	if err != nil {
		return errors.Wrap(err, "notifications/initialized")
	}
	_ = resp2.Body.Close()

	return nil
}

func stringifyMCPResult(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return fmt.Sprintf("%v", x)
	case float64:
		return fmt.Sprintf("%v", x)
	case map[string]any:
		if res, ok := x["result"]; ok {
			return stringifyMCPResult(res)
		}
		if c, ok := x["content"]; ok {
			return stringifyMCPResult(c)
		}
		if e, ok := x["error"]; ok {
			return fmt.Sprintf("%v", e)
		}
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(b)
	case []any:
		parts := make([]string, 0, len(x))
		for _, it := range x {
			parts = append(parts, stringifyMCPResult(it))
		}
		return strings.Join(parts, "\n")
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(b)
	}
}
