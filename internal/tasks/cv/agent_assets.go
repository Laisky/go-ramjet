package cv

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	cvPublicURL       = "https://cv.laisky.com/"
	cvPublicContent   = "https://cv.laisky.com/cv/content"
	cvPublicPDF       = "https://cv.laisky.com/cv/pdf"
	cvPublicOpenAPI   = "https://cv.laisky.com/openapi.json"
	cvPublicMCPServer = "https://mcp.laisky.com"
	cvPublicContact   = "mailto:job@laisky.com"
)

// registerAgentDiscoveryRoutes registers public machine-readable discovery documents.
// It takes the global router and returns no values.
func registerAgentDiscoveryRoutes(router gin.IRouter) {
	router.GET("/robots.txt", serveCVRobots)
	router.GET("/llms.txt", serveCVLLMs)
	router.GET("/sitemap.xml", serveCVSitemap)
	router.GET("/openapi.json", serveCVOpenAPI)
	router.GET("/.well-known/ai-catalog.json", serveCVAICatalog)
	router.GET("/.well-known/mcp.json", serveCVMCPMetadata)
}

// serveCVRobots returns crawler policy and discovery pointers for the CV site.
// It takes a Gin request context and returns no values.
func serveCVRobots(c *gin.Context) {
	body := "User-agent: *\n" +
		"Allow: /\n" +
		"Sitemap: https://cv.laisky.com/sitemap.xml\n" +
		"LLMs: https://cv.laisky.com/llms.txt\n"
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

// serveCVLLMs returns an llms.txt summary for agent crawlers.
// It takes a Gin request context and returns no values.
func serveCVLLMs(c *gin.Context) {
	body := `# Zhonghua (Laisky) Cai CV

> Senior Software Engineer focused on backend, infrastructure, Linux services, platform engineering, and security.

This site is the public CV for Zhonghua (Laisky) Cai in Ottawa, Canada. It is intended for recruiters, hiring managers, and AI agents that need a reliable summary of experience, skills, and contact paths.

## Primary pages
- CV HTML: https://cv.laisky.com/
- CV markdown API: https://cv.laisky.com/cv/content
- CV PDF: https://cv.laisky.com/cv/pdf
- OpenAPI document: https://cv.laisky.com/openapi.json
- Agent catalog: https://cv.laisky.com/.well-known/ai-catalog.json
- MCP server: https://mcp.laisky.com

## Contact
- Email: job@laisky.com
- LinkedIn: https://www.linkedin.com/in/laisky-cai-14237926/
- GitHub: https://github.com/Laisky
- Blog: https://blog.laisky.com/

## Agent guidance
- Use the markdown API for structured CV extraction.
- Use the PDF endpoint when a human-readable attachment is required.
- Use the OpenAPI document to inspect available public CV endpoints.
- The MCP server is public and provides general Laisky tools; it is separate from the CV content API.
`
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

// serveCVSitemap returns a minimal sitemap for public CV crawl targets.
// It takes a Gin request context and returns no values.
func serveCVSitemap(c *gin.Context) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s</loc></url>
  <url><loc>%s</loc></url>
  <url><loc>%s</loc></url>
  <url><loc>%s</loc></url>
</urlset>
`, cvPublicURL, cvPublicContent, cvPublicPDF, cvPublicOpenAPI)
	c.Data(http.StatusOK, "application/xml; charset=utf-8", []byte(body))
}

// serveCVOpenAPI returns the public OpenAPI description for CV endpoints.
// It takes a Gin request context and returns no values.
func serveCVOpenAPI(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"openapi": "3.1.0",
		"info": gin.H{
			"title":       "Zhonghua (Laisky) Cai CV API",
			"version":     "1.0.0",
			"description": "Public CV endpoints for agents and recruiting workflows.",
			"contact": gin.H{
				"name":  "Zhonghua (Laisky) Cai",
				"email": "job@laisky.com",
				"url":   cvPublicURL,
			},
		},
		"servers": []gin.H{{"url": "https://cv.laisky.com"}},
		"paths": gin.H{
			"/cv/content": gin.H{
				"get": gin.H{
					"summary":     "Read the current CV markdown.",
					"description": "Returns the public CV in markdown with update metadata.",
					"operationId": "getCurrentCV",
					"responses": gin.H{
						"200": gin.H{
							"description": "Current CV content.",
							"content": gin.H{
								"application/json": gin.H{
									"schema": gin.H{"$ref": "#/components/schemas/CVContent"},
								},
							},
						},
					},
				},
			},
			"/cv/pdf": gin.H{
				"get": gin.H{
					"summary":     "Download the current CV as PDF.",
					"description": "Returns the rendered public CV PDF when available.",
					"operationId": "downloadCurrentCVPDF",
					"parameters": []gin.H{
						{
							"name":        "ts",
							"in":          "query",
							"required":    false,
							"description": "Optional cache-busting timestamp.",
							"schema":      gin.H{"type": "string"},
						},
					},
					"responses": gin.H{
						"200": gin.H{
							"description": "Current CV PDF.",
							"content": gin.H{
								"application/pdf": gin.H{
									"schema": gin.H{"type": "string", "format": "binary"},
								},
							},
						},
						"404": gin.H{"description": "PDF is not available yet."},
					},
				},
			},
			"/cv/meta": gin.H{
				"get": gin.H{
					"summary":     "Read page metadata for the CV site.",
					"description": "Returns resolved favicon and Open Graph image metadata.",
					"operationId": "getCVPageMeta",
					"responses": gin.H{
						"200": gin.H{
							"description": "Resolved CV page metadata.",
							"content": gin.H{
								"application/json": gin.H{
									"schema": gin.H{"$ref": "#/components/schemas/CVPageMeta"},
								},
							},
						},
					},
				},
			},
		},
		"components": gin.H{
			"schemas": gin.H{
				"CVContent": gin.H{
					"type":     "object",
					"required": []string{"content", "is_default"},
					"properties": gin.H{
						"content":    gin.H{"type": "string", "description": "CV markdown."},
						"updated_at": gin.H{"type": "string", "format": "date-time"},
						"is_default": gin.H{"type": "boolean"},
					},
				},
				"CVPageMeta": gin.H{
					"type":     "object",
					"required": []string{"favicon", "og_image"},
					"properties": gin.H{
						"favicon":  gin.H{"type": "string", "format": "uri"},
						"og_image": gin.H{"type": "string", "format": "uri"},
					},
				},
			},
		},
		"externalDocs": gin.H{
			"description": "Agent-oriented CV summary.",
			"url":         "https://cv.laisky.com/llms.txt",
		},
	})
}

// serveCVAICatalog returns a public catalog for agent discovery.
// It takes a Gin request context and returns no values.
func serveCVAICatalog(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"schema_version": "1.0",
		"name":           "Zhonghua (Laisky) Cai CV",
		"description":    "Public CV, contact paths, markdown API, PDF endpoint, and MCP server metadata.",
		"url":            cvPublicURL,
		"contact": gin.H{
			"email":    "job@laisky.com",
			"linkedin": "https://www.linkedin.com/in/laisky-cai-14237926/",
			"github":   "https://github.com/Laisky",
		},
		"docs": gin.H{
			"llms_txt": cvPublicURL + "llms.txt",
			"openapi":  cvPublicOpenAPI,
			"sitemap":  cvPublicURL + "sitemap.xml",
		},
		"apis": []gin.H{
			{
				"name":        "CV markdown API",
				"type":        "openapi",
				"url":         cvPublicOpenAPI,
				"read_only":   true,
				"auth":        "none for GET /cv/content, /cv/pdf, and /cv/meta",
				"description": "Public read endpoints for the current CV content.",
			},
		},
		"mcp_servers": []gin.H{
			{
				"name":        "Laisky MCP Server",
				"url":         cvPublicMCPServer,
				"description": "Public MCP server for general Laisky tools and agent workflows.",
			},
		},
		"human_handoff": gin.H{
			"email": cvPublicContact,
		},
	})
}

// serveCVMCPMetadata returns MCP discovery metadata for the CV domain.
// It takes a Gin request context and returns no values.
func serveCVMCPMetadata(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":        "Laisky MCP Server",
		"description": "Public MCP server associated with Laisky services and agent workflows.",
		"url":         cvPublicMCPServer,
		"transport":   "streamable-http",
		"auth":        "server-dependent; public discovery available",
		"related": gin.H{
			"cv":      cvPublicURL,
			"openapi": cvPublicOpenAPI,
			"contact": cvPublicContact,
		},
	})
}
