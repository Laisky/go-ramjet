package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CreateDeepResearchHandler returns a retirement response for cached clients without accepting or enqueuing new jobs.
func CreateDeepResearchHandler(c *gin.Context) {
	deepResearchRetired(c)
}

// GetDeepResearchStatusHandler returns a retirement response for legacy job polling without contacting llm-storm or reading task data.
func GetDeepResearchStatusHandler(c *gin.Context) {
	deepResearchRetired(c)
}

// deepResearchRetired writes the HTTP 410 compatibility response to c and stops further handlers.
func deepResearchRetired(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.AbortWithStatusJSON(http.StatusGone, gin.H{
		"code":  "deep_research_retired",
		"error": "The llm-storm Deep Research feature has been retired. Existing saved conversation messages remain available in chat history.",
	})
}
