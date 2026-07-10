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
		"description": "Public CV content rarely changes; webhook registration is owner-approved and currently supports cv.content.updated events.",
		"events":      []string{"cv.content.updated"},
		"register":    "POST /api/v1/webhooks",
	})
}
