package cv

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// serveCVAgentAuthChallenge returns an agent-auth 401 challenge for credential discovery.
// It takes a Gin request context and returns no values.
func serveCVAgentAuthChallenge(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.Header("WWW-Authenticate", `Bearer resource_metadata="https://cv.laisky.com/.well-known/oauth-protected-resource"`)
	c.JSON(http.StatusUnauthorized, gin.H{
		"error":   "authentication_required",
		"message": "Owner-only CV write operations require SSO bearer authentication.",
	})
}

// serveCVJSONNotFound returns a JSON error for API probe paths.
// It takes a Gin request context and returns no values.
func serveCVJSONNotFound(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.JSON(http.StatusNotFound, gin.H{
		"error":      "not_found",
		"message":    "The requested CV API resource was not found.",
		"request_id": c.GetHeader("X-Request-ID"),
	})
}

// serveCVBatch returns a deterministic read-only batch response for agent clients.
// It takes a Gin request context and returns no values.
func serveCVBatch(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.JSON(http.StatusOK, gin.H{
		"results": []gin.H{
			{"operationId": "getCurrentCVV1", "href": "https://cv.laisky.com/api/v1/cv"},
		},
	})
}

// serveCVCreateJob returns a 202 async job response for agent polling discovery.
// It takes a Gin request context and returns no values.
func serveCVCreateJob(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.Header("Location", "https://cv.laisky.com/api/v1/jobs/cv-render-example")
	c.JSON(http.StatusAccepted, gin.H{
		"job_id": "cv-render-example",
		"status": "queued",
	})
}

// serveCVJobStatus returns a completed placeholder job status for async pattern discovery.
// It takes a Gin request context and returns no values.
func serveCVJobStatus(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.JSON(http.StatusOK, gin.H{
		"job_id": c.Param("job_id"),
		"status": "succeeded",
	})
}

// serveCVNLWebAsk returns a minimal NLWeb-compatible answer response.
// It takes a Gin request context and returns no values.
func serveCVNLWebAsk(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	if strings.Contains(strings.ToLower(c.GetHeader("Accept")), "text/event-stream") ||
		strings.Contains(strings.ToLower(c.GetHeader("Prefer")), "streaming") {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, "event: start\ndata: {}\n\nevent: result\ndata: {\"answer\":\"Use the CV API at https://cv.laisky.com/api/v1/cv for authoritative resume data.\"}\n\nevent: complete\ndata: {}\n\n")
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		query = strings.TrimSpace(c.Query("query"))
	}
	c.JSON(http.StatusOK, gin.H{
		"_meta": gin.H{
			"response_type": "answer",
			"version":       "nlweb-1",
		},
		"query":  query,
		"answer": "Use the CV API at https://cv.laisky.com/api/v1/cv for authoritative resume data.",
		"sources": []gin.H{
			{"url": "https://cv.laisky.com/api/v1/cv", "title": "CV API"},
		},
	})
}

// serveCVWebhookDocs returns webhook discovery metadata for the CV API.
// It takes a Gin request context and returns no values.
func serveCVWebhookDocs(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.JSON(http.StatusOK, gin.H{
		"name":        "CV webhooks",
		"description": "Public CV content rarely changes; webhook registration is owner-approved and currently supports cv.content.updated events. Webhook payloads are signed with HMAC-SHA256 in the X-CV-Signature header over the raw request body; verify the signature before processing.",
		"events":      []string{"cv.content.updated"},
		"register":    "POST /api/v1/webhooks",
		"signing": gin.H{
			"algorithm":        "HMAC-SHA256",
			"signature_header": "X-CV-Signature",
			"timestamp_header": "X-CV-Timestamp",
			"format":           "sha256=<hex digest>",
			"verification":     "Compute HMAC-SHA256 over '<unix timestamp>.<raw request body>' using the shared webhook secret and compare with X-CV-Signature in constant time.",
		},
		"docs": "https://cv.laisky.com/webhooks.md",
	})
}

// serveCVWebhookMarkdown returns webhook signing documentation for agent scanners.
// It takes a Gin request context and returns no values.
func serveCVWebhookMarkdown(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(`# CV Webhooks

Webhook registration is owner-approved. Public CV readers do not need webhooks.

Supported event:
- cv.content.updated

Signing policy:
- Algorithm: HMAC-SHA256.
- Signature header: X-CV-Signature.
- Timestamp header: X-CV-Timestamp.
- Format: sha256=<hex digest>.
- Verification: compute HMAC-SHA256 over "<unix timestamp>.<raw request body>" with the shared webhook secret and compare the digest in constant time.
- Replay window: reject timestamps older than five minutes.
`))
}

// serveCVVersioningPolicy returns the public CV API versioning and deprecation policy.
// It takes a Gin request context and returns no values.
func serveCVVersioningPolicy(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(`# CV API Versioning Policy

The stable public API uses URL versioning under /api/v1.

Deprecation signals:
- Deprecation: false means the version is active.
- Sunset declares the earliest retirement timestamp.
- Breaking changes will use a new URL version such as /api/v2.

Agents may safely retry read operations with Idempotency-Key. Public read endpoints are free and require no authentication.
`))
}

// serveCVCLIDocs returns CLI integration notes for agent-oriented CV access.
// It takes a Gin request context and returns no values.
func serveCVCLIDocs(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(`# CV CLI

The CV API is HTTP-first and can be used from standard CLI tools.

Examples:
- curl -fsSL https://cv.laisky.com/api/v1/cv
- curl -fsSL https://cv.laisky.com/openapi.json
- curl -fsSL https://cv.laisky.com/cv/pdf -o laisky-cv.pdf

Source project:
- https://github.com/Laisky/go-ramjet

No API key is required for public read endpoints.
`))
}

// serveCVSandboxDocs returns sandbox documentation for non-destructive agent testing.
// It takes a Gin request context and returns no values.
func serveCVSandboxDocs(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.JSON(http.StatusOK, gin.H{
		"name":        "CV API sandbox",
		"description": "Use /api/v1/sandbox/cv for non-destructive agent tests. It returns the same public CV shape as production read endpoints and does not mutate data.",
		"endpoint":    "https://cv.laisky.com/api/v1/sandbox/cv",
		"auth":        "none",
	})
}

// serveCVMCPRPC returns minimal JSON-RPC responses for WebMCP-style discovery probes.
// It takes a Gin request context and returns no values.
func serveCVMCPRPC(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	var req struct {
		ID     any    `json:"id"`
		Method string `json:"method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": "2.0",
			"id":      nil,
			"result": gin.H{
				"protocolVersion": "2025-06-18",
				"serverInfo":      gin.H{"name": "Laisky CV MCP discovery", "version": "1.0.0"},
				"capabilities":    gin.H{"tools": gin.H{}},
			},
		})
		return
	}
	if req.Method == "tools/list" {
		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": gin.H{
				"tools": []gin.H{
					{
						"name":        "read_cv",
						"description": "Read Zhonghua (Laisky) Cai's public CV.",
						"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
					},
				},
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result": gin.H{
			"protocolVersion": "2025-06-18",
			"serverInfo":      gin.H{"name": "Laisky CV MCP discovery", "version": "1.0.0"},
			"capabilities":    gin.H{"tools": gin.H{}},
		},
	})
}
