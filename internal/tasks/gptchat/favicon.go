package gptchat

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	//go:embed templates/static/favicon.ico
	gptchatFavicon []byte
)

// registerFaviconRoutes registers routes that serve the gptchat favicon.
//
// Args:
//   - grp: router group mounted at the task prefix (e.g. "/gptchat").
func registerFaviconRoutes(grp *gin.RouterGroup) {
	grp.GET("/favicon.ico", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, "image/x-icon", gptchatFavicon)
	})
}
