package cv

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// serveCVAICatalog returns a public catalog for agent discovery.
// It takes a Gin request context and returns no values.
func serveCVAICatalog(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"specVersion": "0.1",
		"name":        "Zhonghua (Laisky) Cai CV",
		"description": "Public CV, contact paths, markdown API, PDF endpoint, and MCP server metadata.",
		"entries": []gin.H{
			{
				"identifier":  "urn:air:cv.laisky.com:openapi",
				"urn":         "urn:air:cv.laisky.com:openapi",
				"displayName": "CV OpenAPI",
				"mediaType":   "application/vnd.oai.openapi+json;version=3.1",
				"media_type":  "application/vnd.oai.openapi+json;version=3.1",
				"type":        "application/vnd.oai.openapi+json;version=3.1",
				"mimeType":    "application/vnd.oai.openapi+json;version=3.1",
				"contentType": "application/vnd.oai.openapi+json;version=3.1",
				"url":         cvPublicOpenAPI,
			},
			{
				"identifier":  "urn:air:cv.laisky.com:llms",
				"urn":         "urn:air:cv.laisky.com:llms",
				"displayName": "CV llms.txt",
				"mediaType":   "text/markdown",
				"media_type":  "text/markdown",
				"type":        "text/markdown",
				"mimeType":    "text/markdown",
				"contentType": "text/markdown",
				"url":         cvPublicURL + "llms.txt",
			},
			{
				"identifier":  "urn:air:cv.laisky.com:mcp",
				"urn":         "urn:air:cv.laisky.com:mcp",
				"displayName": "Laisky MCP server card",
				"mediaType":   "application/mcp-server-card+json",
				"media_type":  "application/mcp-server-card+json",
				"type":        "application/mcp-server-card+json",
				"mimeType":    "application/mcp-server-card+json",
				"contentType": "application/mcp-server-card+json",
				"url":         "https://mcp.laisky.com/.well-known/mcp/server-card.json",
			},
		},
	})
}

// serveCVAPICatalog returns an RFC 9727-style API catalog.
// It takes a Gin request context and returns no values.
func serveCVAPICatalog(c *gin.Context) {
	c.Header("Content-Type", `application/linkset+json;profile="https://www.rfc-editor.org/info/rfc9727"; charset=utf-8`)
	c.JSON(http.StatusOK, gin.H{
		"api_catalog_version": "1",
		"linkset": []gin.H{
			{
				"anchor": cvPublicURL,
				"service-desc": []gin.H{
					{"href": cvPublicOpenAPI, "type": "application/vnd.oai.openapi+json;version=3.1"},
				},
				"describedby": []gin.H{
					{"href": cvPublicURL + "llms.txt", "type": "text/markdown"},
				},
				"item": []gin.H{
					{"href": cvPublicOpenAPI, "type": "application/vnd.oai.openapi+json;version=3.1"},
					{"href": cvPublicURL + "api/v1/cv", "type": "application/json"},
				},
			},
		},
		"apis": []gin.H{
			{
				"name":        "Zhonghua (Laisky) Cai CV API",
				"description": "Public read API for current CV markdown and PDF.",
				"api_uri":     cvPublicOpenAPI,
				"api_type":    "openapi",
				"auth":        "none for public read endpoints",
			},
		},
	})
}

// serveCVAPICatalogMarkdown returns a markdown summary of the API catalog.
// It takes a Gin request context and returns no values.
func serveCVAPICatalogMarkdown(c *gin.Context) {
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(`# CV API Catalog

- [OpenAPI](https://cv.laisky.com/openapi.json)
- [Versioned CV API](https://cv.laisky.com/api/v1/cv)
- [llms.txt](https://cv.laisky.com/llms.txt)
`))
}
