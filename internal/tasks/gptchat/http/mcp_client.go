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
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v5/json"
)

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

// callMCPTool executes a tool against a remote MCP server.
func callMCPTool(ctx context.Context, server *MCPServerConfig, toolName string, args string) (string, error) {
	if server == nil {
		return "", errors.New("nil mcp server")
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return "", errors.New("empty tool name")
	}

	auths := mcpAuthCandidates(server.APIKey)
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

	// 1) Try REST-ish endpoints.
	for _, endpoint := range guessMCPToolCallURLs(server.URL, server.URLPrefix) {
		resp, callErr := doMCPPost(ctx, endpoint, headers, auths, bodyBytes)
		if callErr != nil {
			continue
		}
		defer resp.Body.Close()
		obj, parseErr := fetchJSONOrSSE(resp)
		if parseErr != nil {
			return "", errors.Wrap(parseErr, "parse mcp response")
		}
		return stringifyMCPResult(obj), nil
	}

	// 2) Fallback to JSON-RPC.
	return callMCPToolJSONRPC(ctx, server, headers, auths, body)
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

func callMCPToolJSONRPC(ctx context.Context, server *MCPServerConfig, baseHeaders http.Header, auths []string, params map[string]any) (string, error) {
	endpointCandidates := guessJSONRPCEndpoints(server)
	if len(endpointCandidates) == 0 {
		return "", errors.New("no json-rpc endpoints")
	}

	methods := []string{"tools/call", "tools.call"}
	for _, endpoint := range endpointCandidates {
		if err := ensureMCPSession(ctx, server, endpoint, baseHeaders, auths); err != nil {
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
			resp, err := doMCPPost(ctx, endpoint, mcpSessionHeaders(server, baseHeaders), auths, body)
			if err != nil {
				continue
			}
			defer resp.Body.Close()
			obj, err := fetchJSONOrSSE(resp)
			if err != nil {
				return "", errors.Wrap(err, "parse rpc response")
			}
			if e, ok := obj["error"]; ok && e != nil {
				return "", errors.Errorf("mcp rpc error: %v", e)
			}
			if res, ok := obj["result"]; ok {
				return stringifyMCPResult(res), nil
			}
			return stringifyMCPResult(obj), nil
		}
	}

	return "", errors.New("failed to call mcp tool via json-rpc")
}

func guessJSONRPCEndpoints(server *MCPServerConfig) []string {
	base := strings.TrimRight(strings.TrimSpace(server.URL), "/")
	candidates := []string{}
	if base != "" {
		candidates = append(candidates, base)
	}
	origin := urlOrigin(base)
	if origin != "" {
		candidates = append(candidates, origin+"/")
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
	sid := strings.TrimSpace(server.MCPSessionID)
	if sid == "" {
		sid = "mcp-session-" + randomID("", 8)
		server.MCPSessionID = sid
	}
	h.Set("mcp-protocol-version", pv)
	h.Set("mcp-session-id", sid)
	return h
}

func ensureMCPSession(ctx context.Context, server *MCPServerConfig, endpoint string, baseHeaders http.Header, auths []string) error {
	if strings.TrimSpace(server.MCPSessionID) != "" {
		// Best-effort: treat presence of session ID as already initialized.
		return nil
	}

	h := mcpSessionHeaders(server, baseHeaders)
	initPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      0,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": server.MCPProtocolVersion,
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
	_ = resp.Body.Close()

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
