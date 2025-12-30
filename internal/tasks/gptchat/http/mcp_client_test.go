package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTruncateForLog tests the string truncation function.
func TestTruncateForLog(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string no truncation",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length no truncation",
			input:    "helloworld",
			maxLen:   10,
			expected: "helloworld",
		},
		{
			name:     "long string truncated",
			input:    "hello world foo bar baz",
			maxLen:   10,
			expected: "hello worl...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "zero max uses default",
			input:    strings.Repeat("a", 300),
			maxLen:   0,
			expected: strings.Repeat("a", maxLogValueLen) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForLog(tt.input, tt.maxLen)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestTruncateMapForLog tests recursive map truncation.
func TestTruncateMapForLog(t *testing.T) {
	t.Run("nested map with long strings", func(t *testing.T) {
		longStr := strings.Repeat("x", 300)
		input := map[string]any{
			"short": "hello",
			"long":  longStr,
			"nested": map[string]any{
				"deep_long": longStr,
				"num":       42,
			},
			"array": []any{"short", longStr},
		}

		result := truncateMapForLog(input, 50)
		require.NotNil(t, result)

		m, ok := result.(map[string]any)
		require.True(t, ok)

		// Short string unchanged
		require.Equal(t, "hello", m["short"])

		// Long string truncated
		longResult, ok := m["long"].(string)
		require.True(t, ok)
		require.True(t, len(longResult) <= 54) // 50 + "..."
		require.True(t, strings.HasSuffix(longResult, "..."))

		// Nested map also truncated
		nested, ok := m["nested"].(map[string]any)
		require.True(t, ok)
		deepLong, ok := nested["deep_long"].(string)
		require.True(t, ok)
		require.True(t, strings.HasSuffix(deepLong, "..."))
		require.Equal(t, 42, nested["num"])

		// Array elements truncated
		arr, ok := m["array"].([]any)
		require.True(t, ok)
		require.Len(t, arr, 2)
		require.Equal(t, "short", arr[0])
		arrLong, ok := arr[1].(string)
		require.True(t, ok)
		require.True(t, strings.HasSuffix(arrLong, "..."))
	})

	t.Run("nil input", func(t *testing.T) {
		result := truncateMapForLog(nil, 50)
		require.Nil(t, result)
	})

	t.Run("primitive types", func(t *testing.T) {
		require.Equal(t, 123, truncateMapForLog(123, 50))
		require.Equal(t, true, truncateMapForLog(true, 50))
		require.Equal(t, 3.14, truncateMapForLog(3.14, 50))
	})
}

// TestProcessMCPResponse tests error detection in MCP responses.
func TestProcessMCPResponse(t *testing.T) {
	t.Run("successful response with content", func(t *testing.T) {
		resp := map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "Hello world"},
			},
		}
		result, err := processMCPResponse(resp)
		require.NoError(t, err)
		require.Contains(t, result, "Hello world")
	})

	t.Run("isError true", func(t *testing.T) {
		resp := map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "missing authorization bearer token"},
			},
			"isError": true,
		}
		result, err := processMCPResponse(resp)
		require.Error(t, err)
		require.Empty(t, result)
		require.Contains(t, err.Error(), "mcp tool error")
		require.Contains(t, err.Error(), "missing authorization bearer token")
	})

	t.Run("isError false", func(t *testing.T) {
		resp := map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "success"},
			},
			"isError": false,
		}
		result, err := processMCPResponse(resp)
		require.NoError(t, err)
		require.Contains(t, result, "success")
	})

	t.Run("error field present", func(t *testing.T) {
		resp := map[string]any{
			"error": map[string]any{
				"code":    -32600,
				"message": "Invalid request",
			},
		}
		result, err := processMCPResponse(resp)
		require.Error(t, err)
		require.Empty(t, result)
		require.Contains(t, err.Error(), "Invalid request")
	})

	t.Run("result field with nested isError", func(t *testing.T) {
		resp := map[string]any{
			"result": map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "error message"},
				},
				"isError": true,
			},
		}
		// Now processMCPResponse detects nested isError inside result
		result, err := processMCPResponse(resp)
		require.Error(t, err)
		require.Empty(t, result)
		require.Contains(t, err.Error(), "error message")
	})

	t.Run("nil input", func(t *testing.T) {
		result, err := processMCPResponse(nil)
		require.NoError(t, err)
		// nil map returns empty string
		require.Empty(t, result)
	})
}

// TestMCPAuthCandidatesFallback tests API key fallback mechanism.
func TestMCPAuthCandidatesFallback(t *testing.T) {
	t.Run("server API key takes precedence", func(t *testing.T) {
		auths := mcpAuthCandidates("server-key")
		require.NotEmpty(t, auths)

		found := false
		for _, a := range auths {
			if strings.Contains(a, "server-key") {
				found = true
				break
			}
		}
		require.True(t, found)
	})

	t.Run("empty key returns nil", func(t *testing.T) {
		auths := mcpAuthCandidates("")
		require.Empty(t, auths)
	})

	t.Run("whitespace only returns nil", func(t *testing.T) {
		auths := mcpAuthCandidates("   ")
		require.Empty(t, auths)
	})

	t.Run("already has Bearer prefix", func(t *testing.T) {
		auths := mcpAuthCandidates("Bearer sk-test")
		require.NotEmpty(t, auths)
		// Should include both the original and with Bearer prefix
		hasBearerFormat := false
		for _, a := range auths {
			if a == "Bearer Bearer sk-test" || a == "Bearer sk-test" {
				hasBearerFormat = true
				break
			}
		}
		require.True(t, hasBearerFormat)
	})
}

// TestCallMCPToolWithFallbackAPIKey tests the fallback API key mechanism.
func TestCallMCPToolWithFallbackAPIKey(t *testing.T) {
	// Setup HTTP client for tests
	if err := SetupHTTPCli(); err != nil {
		t.Skipf("failed to setup http client: %v", err)
	}

	// Create a test server that checks authorization
	authReceived := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authReceived = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if authReceived == "" {
			// Return error when no auth
			json.NewEncoder(w).Encode(map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "missing authorization bearer token"},
				},
				"isError": true,
			})
			return
		}
		// Return success
		json.NewEncoder(w).Encode(map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "success"},
			},
		})
	}))
	defer ts.Close()

	t.Run("uses server API key when provided", func(t *testing.T) {
		authReceived = ""
		server := &MCPServerConfig{
			URL:     ts.URL,
			APIKey:  "server-api-key",
			Enabled: true,
		}
		opts := &MCPCallOption{
			FallbackAPIKey: "fallback-key",
		}

		result, err := callMCPTool(context.Background(), server, "test_tool", `{"arg": "value"}`, opts)
		require.NoError(t, err)
		require.Contains(t, result, "success")
		require.Contains(t, authReceived, "server-api-key")
	})

	t.Run("uses fallback API key when server key empty", func(t *testing.T) {
		authReceived = ""
		server := &MCPServerConfig{
			URL:     ts.URL,
			APIKey:  "", // Empty server key
			Enabled: true,
		}
		opts := &MCPCallOption{
			FallbackAPIKey: "fallback-api-key",
		}

		result, err := callMCPTool(context.Background(), server, "test_tool", `{"arg": "value"}`, opts)
		require.NoError(t, err)
		require.Contains(t, result, "success")
		require.Contains(t, authReceived, "fallback-api-key")
	})

	t.Run("no API key results in error from server", func(t *testing.T) {
		authReceived = ""
		server := &MCPServerConfig{
			URL:     ts.URL,
			APIKey:  "",
			Enabled: true,
		}
		opts := &MCPCallOption{
			FallbackAPIKey: "", // No fallback either
		}

		_, err := callMCPTool(context.Background(), server, "test_tool", `{}`, opts)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing authorization")
	})

	t.Run("nil opts uses only server key", func(t *testing.T) {
		authReceived = ""
		server := &MCPServerConfig{
			URL:     ts.URL,
			APIKey:  "only-server-key",
			Enabled: true,
		}

		result, err := callMCPTool(context.Background(), server, "test_tool", `{}`, nil)
		require.NoError(t, err)
		require.Contains(t, result, "success")
		require.Contains(t, authReceived, "only-server-key")
	})
}

// TestCallMCPToolIsErrorHandling tests that isError responses are properly handled.
func TestCallMCPToolIsErrorHandling(t *testing.T) {
	// Setup HTTP client for tests
	if err := SetupHTTPCli(); err != nil {
		t.Skipf("failed to setup http client: %v", err)
	}

	t.Run("isError true returns error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "tool execution failed"},
				},
				"isError": true,
			})
		}))
		defer ts.Close()

		server := &MCPServerConfig{
			URL:     ts.URL,
			APIKey:  "test-key",
			Enabled: true,
		}

		_, err := callMCPTool(context.Background(), server, "failing_tool", `{}`, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "mcp tool error")
		require.Contains(t, err.Error(), "tool execution failed")
	})

	t.Run("isError false returns success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "operation succeeded"},
				},
				"isError": false,
			})
		}))
		defer ts.Close()

		server := &MCPServerConfig{
			URL:     ts.URL,
			APIKey:  "test-key",
			Enabled: true,
		}

		result, err := callMCPTool(context.Background(), server, "working_tool", `{}`, nil)
		require.NoError(t, err)
		require.Contains(t, result, "operation succeeded")
	})
}

// TestStringifyMCPResult tests result stringification.
func TestStringifyMCPResult(t *testing.T) {
	t.Run("nil returns empty", func(t *testing.T) {
		require.Empty(t, stringifyMCPResult(nil))
	})

	t.Run("string passthrough", func(t *testing.T) {
		require.Equal(t, "hello", stringifyMCPResult("hello"))
	})

	t.Run("number formatting", func(t *testing.T) {
		require.Equal(t, "42", stringifyMCPResult(float64(42)))
		require.Equal(t, "3.14", stringifyMCPResult(3.14))
	})

	t.Run("bool formatting", func(t *testing.T) {
		require.Equal(t, "true", stringifyMCPResult(true))
		require.Equal(t, "false", stringifyMCPResult(false))
	})

	t.Run("map with result field", func(t *testing.T) {
		m := map[string]any{
			"result": "the result",
		}
		require.Equal(t, "the result", stringifyMCPResult(m))
	})

	t.Run("map with content field", func(t *testing.T) {
		m := map[string]any{
			"content": "the content",
		}
		require.Equal(t, "the content", stringifyMCPResult(m))
	})

	t.Run("map with error field", func(t *testing.T) {
		m := map[string]any{
			"error": "the error",
		}
		require.Contains(t, stringifyMCPResult(m), "the error")
	})

	t.Run("array of items", func(t *testing.T) {
		arr := []any{"line1", "line2", "line3"}
		result := stringifyMCPResult(arr)
		require.Contains(t, result, "line1")
		require.Contains(t, result, "line2")
		require.Contains(t, result, "line3")
	})

	t.Run("nested content array", func(t *testing.T) {
		m := map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "first"},
				map[string]any{"type": "text", "text": "second"},
			},
		}
		result := stringifyMCPResult(m)
		require.Contains(t, result, "first")
		require.Contains(t, result, "second")
	})
}

// TestAPIKeyFallbackPrecedence tests that server API key takes precedence over fallback.
func TestAPIKeyFallbackPrecedence(t *testing.T) {
	// Setup HTTP client for tests
	if err := SetupHTTPCli(); err != nil {
		t.Skipf("failed to setup http client: %v", err)
	}

	var receivedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
		})
	}))
	defer ts.Close()

	t.Run("server key used when both provided", func(t *testing.T) {
		receivedAuth = ""
		server := &MCPServerConfig{
			URL:     ts.URL,
			APIKey:  "server-specific-key",
			Enabled: true,
		}
		opts := &MCPCallOption{FallbackAPIKey: "fallback-key"}

		_, err := callMCPTool(context.Background(), server, "tool", `{}`, opts)
		require.NoError(t, err)
		require.Contains(t, receivedAuth, "server-specific-key")
		require.NotContains(t, receivedAuth, "fallback")
	})

	t.Run("fallback key used when server key whitespace", func(t *testing.T) {
		receivedAuth = ""
		server := &MCPServerConfig{
			URL:     ts.URL,
			APIKey:  "   ", // Whitespace
			Enabled: true,
		}
		opts := &MCPCallOption{FallbackAPIKey: "session-key"}

		_, err := callMCPTool(context.Background(), server, "tool", `{}`, opts)
		require.NoError(t, err)
		require.Contains(t, receivedAuth, "session-key")
	})
}

// TestMCPErrorResponseFormats tests different error response formats.
func TestMCPErrorResponseFormats(t *testing.T) {
	// Setup HTTP client for tests
	if err := SetupHTTPCli(); err != nil {
		t.Skipf("failed to setup http client: %v", err)
	}

	t.Run("error in content with isError true", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "Authentication failed: invalid token"},
				},
				"isError": true,
			})
		}))
		defer ts.Close()

		server := &MCPServerConfig{URL: ts.URL, APIKey: "key", Enabled: true}
		_, err := callMCPTool(context.Background(), server, "tool", `{}`, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Authentication failed")
	})

	t.Run("json-rpc error format", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"error": map[string]any{
					"code":    -32600,
					"message": "Invalid Request",
				},
			})
		}))
		defer ts.Close()

		server := &MCPServerConfig{URL: ts.URL, APIKey: "key", Enabled: true}
		_, err := callMCPTool(context.Background(), server, "tool", `{}`, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid Request")
	})

	t.Run("result with nested isError", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": "Permission denied"},
					},
					"isError": true,
				},
			})
		}))
		defer ts.Close()

		server := &MCPServerConfig{URL: ts.URL, APIKey: "key", Enabled: true}
		_, err := callMCPTool(context.Background(), server, "tool", `{}`, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Permission denied")
	})
}

// TestTruncateValueEdgeCases tests edge cases in value truncation.
func TestTruncateValueEdgeCases(t *testing.T) {
	t.Run("deeply nested structure", func(t *testing.T) {
		input := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": map[string]any{
						"long_value": strings.Repeat("x", 500),
					},
				},
			},
		}

		result := truncateMapForLog(input, 50)
		require.NotNil(t, result)

		// Navigate to the deep value
		m := result.(map[string]any)
		l1 := m["level1"].(map[string]any)
		l2 := l1["level2"].(map[string]any)
		l3 := l2["level3"].(map[string]any)
		longVal := l3["long_value"].(string)
		require.True(t, len(longVal) <= 54)
		require.True(t, strings.HasSuffix(longVal, "..."))
	})

	t.Run("mixed array types", func(t *testing.T) {
		input := []any{
			"short",
			strings.Repeat("long", 100),
			42,
			true,
			map[string]any{"nested": strings.Repeat("deep", 100)},
		}

		result := truncateMapForLog(input, 50)
		require.NotNil(t, result)
		arr := result.([]any)
		require.Len(t, arr, 5)

		// Short string unchanged
		require.Equal(t, "short", arr[0])

		// Long string truncated
		longStr := arr[1].(string)
		require.True(t, strings.HasSuffix(longStr, "..."))

		// Numbers and bools unchanged
		require.Equal(t, 42, arr[2])
		require.Equal(t, true, arr[3])

		// Nested map truncated
		nestedMap := arr[4].(map[string]any)
		nestedVal := nestedMap["nested"].(string)
		require.True(t, strings.HasSuffix(nestedVal, "..."))
	})

	t.Run("bytes slice", func(t *testing.T) {
		// Simulate json.RawMessage
		longJSON := []byte(`{"key": "` + strings.Repeat("x", 500) + `"}`)
		result := truncateMapForLog(longJSON, 50)
		require.NotNil(t, result)

		// Should be parsed and truncated
		m, ok := result.(map[string]any)
		require.True(t, ok)
		key := m["key"].(string)
		require.True(t, strings.HasSuffix(key, "..."))
	})
}

// TestCallMCPToolNilInputs tests nil and edge case inputs.
func TestCallMCPToolNilInputs(t *testing.T) {
	t.Run("nil server", func(t *testing.T) {
		_, err := callMCPTool(context.Background(), nil, "tool", `{}`, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "nil mcp server")
	})

	t.Run("empty tool name", func(t *testing.T) {
		server := &MCPServerConfig{URL: "http://example.com", Enabled: true}
		_, err := callMCPTool(context.Background(), server, "", `{}`, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty tool name")
	})

	t.Run("whitespace tool name", func(t *testing.T) {
		server := &MCPServerConfig{URL: "http://example.com", Enabled: true}
		_, err := callMCPTool(context.Background(), server, "   ", `{}`, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty tool name")
	})
}

// TestProcessMCPResponseEdgeCases tests edge cases in response processing.
func TestProcessMCPResponseEdgeCases(t *testing.T) {
	t.Run("isError non-boolean", func(t *testing.T) {
		// isError is a string, not boolean - should not be treated as error
		resp := map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "data"}},
			"isError": "true", // String, not bool
		}
		result, err := processMCPResponse(resp)
		require.NoError(t, err)
		require.NotEmpty(t, result)
	})

	t.Run("error field null", func(t *testing.T) {
		resp := map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "success"}},
			"error":   nil,
		}
		result, err := processMCPResponse(resp)
		require.NoError(t, err)
		require.Contains(t, result, "success")
	})

	t.Run("empty content array", func(t *testing.T) {
		resp := map[string]any{
			"content": []any{},
		}
		result, err := processMCPResponse(resp)
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("both isError false and error nil", func(t *testing.T) {
		resp := map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "all good"}},
			"isError": false,
			"error":   nil,
		}
		result, err := processMCPResponse(resp)
		require.NoError(t, err)
		require.Contains(t, result, "all good")
	})
}
