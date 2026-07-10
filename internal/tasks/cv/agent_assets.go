package cv

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	cvPublicURL       = "https://cv.laisky.com/"
	cvPublicContent   = "https://cv.laisky.com/cv/content"
	cvPublicPDF       = "https://cv.laisky.com/cv/pdf"
	cvPublicOpenAPI   = "https://cv.laisky.com/openapi.json"
	cvPublicMCPServer = "https://mcp.laisky.com"
	cvPublicContact   = "mailto:job@laisky.com"
	cvPublicIcon      = "https://s3.laisky.com/uploads/2025/12/favicon.ico"
)

// registerAgentDiscoveryRoutes registers public machine-readable discovery documents.
// It takes the global router and CV handler and returns no values.
func registerAgentDiscoveryRoutes(router gin.IRouter, h *handler) {
	router.Use(serveCVRootAgentSurface)
	router.GET("/robots.txt", serveCVRobots)
	router.GET("/llms.txt", serveCVLLMs)
	router.GET("/llms.md", serveCVLLMs)
	router.GET("/agents.md", serveCVAgents)
	router.GET("/AGENTS.md", serveCVAgents)
	router.GET("/auth.md", serveCVAuth)
	router.GET("/index.md", serveCVIndexMarkdown)
	router.GET("/sitemap.xml", serveCVSitemap)
	router.GET("/openapi.json", serveCVOpenAPI)
	router.GET("/pricing.md", serveCVPricing)
	router.GET("/pricing", serveCVPricingHTML)
	router.GET("/about", serveCVAboutHTML)
	router.GET("/contact", serveCVContactHTML)
	router.GET("/privacy", serveCVPrivacyHTML)
	router.GET("/developer", serveCVDeveloperHTML)
	router.GET("/docs", serveCVDeveloperHTML)
	router.GET("/api", serveCVAPIRoot)
	router.GET("/api/v1", serveCVAPIRoot)
	router.GET("/v1", serveCVAPIRoot)
	router.GET("/api/v1/cv", h.getContent)
	router.GET("/.well-known/ai-catalog.json", serveCVAICatalog)
	router.GET("/.well-known/api-catalog", serveCVAPICatalog)
	router.GET("/.well-known/api-catalog.json", serveCVAPICatalog)
	router.GET("/.well-known/agent-card.json", serveCVA2AAgentCard)
	router.GET("/.well-known/mcp.json", serveCVMCPMetadata)
	router.GET("/.well-known/oauth-protected-resource", serveCVOAuthProtectedResource)
	router.GET("/.well-known/oauth-authorization-server", serveCVOAuthAuthorizationServer)
	router.GET("/.well-known/http-message-signatures-directory", serveCVHTTPSignatureDirectory)
}

// setCVAPIDiscoveryHeaders adds stable headers that help agents classify CV API responses.
// It takes a Gin request context and returns no values.
func setCVAPIDiscoveryHeaders(c *gin.Context) {
	c.Header("Link", `<https://cv.laisky.com/openapi.json>; rel="service-desc"; type="application/vnd.oai.openapi+json;version=3.1", <https://cv.laisky.com/llms.txt>; rel="describedby"; type="text/markdown"`)
	c.Header("Vary", "Accept")
	c.Header("X-RateLimit-Limit", "120")
	c.Header("X-RateLimit-Remaining", "119")
	c.Header("X-RateLimit-Reset", "60")
}

// serveCVRootAgentSurface serves crawler-readable CV root content for the CV host.
// It takes a Gin request context and returns no values.
func serveCVRootAgentSurface(c *gin.Context) {
	if !isCVHost(c.Request.Host) || c.Request.URL.Path != "/" {
		c.Next()
		return
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		c.Next()
		return
	}

	setCVAPIDiscoveryHeaders(c)
	accept := strings.ToLower(c.GetHeader("Accept"))
	mode := strings.ToLower(strings.TrimSpace(c.Query("mode")))
	if strings.Contains(accept, "text/markdown") {
		c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(buildCVIndexMarkdown()))
		c.Abort()
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(buildCVAgentHTML(mode == "agent")))
	c.Abort()
}

// isCVHost reports whether host is the public CV host.
// It takes a host string and returns true when the host belongs to cv.laisky.com.
func isCVHost(host string) bool {
	normalized := strings.ToLower(strings.TrimSpace(host))
	if idx := strings.Index(normalized, ":"); idx > 0 {
		normalized = normalized[:idx]
	}
	return normalized == "cv.laisky.com"
}

// serveCVRobots returns crawler policy and discovery pointers for the CV site.
// It takes a Gin request context and returns no values.
func serveCVRobots(c *gin.Context) {
	body := "User-agent: *\n" +
		"Allow: /\n" +
		"Schemamap: https://cv.laisky.com/.well-known/api-catalog\n" +
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
- [CV HTML](https://cv.laisky.com/)
- [Agent-mode CV](https://cv.laisky.com/?mode=agent)
- [CV markdown API](https://cv.laisky.com/cv/content)
- [Versioned CV API](https://cv.laisky.com/api/v1/cv)
- [CV PDF](https://cv.laisky.com/cv/pdf)
- [OpenAPI document](https://cv.laisky.com/openapi.json)
- [Agent catalog](https://cv.laisky.com/.well-known/ai-catalog.json)
- [API catalog](https://cv.laisky.com/.well-known/api-catalog)
- [Agent instructions](https://cv.laisky.com/agents.md)
- [Auth guide](https://cv.laisky.com/auth.md)
- [MCP server](https://mcp.laisky.com)

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

// serveCVAgents returns agent operating instructions for the public CV site.
// It takes a Gin request context and returns no values.
func serveCVAgents(c *gin.Context) {
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(`# Agent Instructions for Zhonghua (Laisky) Cai CV

Use this site when the user asks about Zhonghua (Laisky) Cai's resume, backend/infrastructure experience, security work, open source projects, or hiring contact details.

Preferred agent workflow:
1. Read the structured CV from https://cv.laisky.com/api/v1/cv or https://cv.laisky.com/cv/content.
2. Use https://cv.laisky.com/openapi.json for endpoint schemas.
3. Download https://cv.laisky.com/cv/pdf only when a user needs a PDF attachment.
4. Contact job@laisky.com for recruiting, interview scheduling, references, or role-fit questions.

Do not infer private information. The public CV and linked profiles are the source of truth.
`))
}

// serveCVAuth returns an agent-readable authentication guide.
// It takes a Gin request context and returns no values.
func serveCVAuth(c *gin.Context) {
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(`# CV API Authentication

The public read endpoints require no authentication:

- GET /api/v1/cv
- GET /cv/content
- GET /cv/meta
- GET /cv/pdf
- GET /openapi.json

Authenticated write and preview operations are owner-only and use SSO bearer tokens. Recruiting agents should not call write endpoints.

Agent auth metadata:
- Protected resource metadata: https://cv.laisky.com/.well-known/oauth-protected-resource
- Authorization server metadata: https://cv.laisky.com/.well-known/oauth-authorization-server
`))
}

// serveCVIndexMarkdown returns a markdown representation of the CV homepage.
// It takes a Gin request context and returns no values.
func serveCVIndexMarkdown(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(buildCVIndexMarkdown()))
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
  <url><loc>https://cv.laisky.com/developer</loc></url>
  <url><loc>https://cv.laisky.com/about</loc></url>
  <url><loc>https://cv.laisky.com/contact</loc></url>
  <url><loc>https://cv.laisky.com/privacy</loc></url>
</urlset>
`, cvPublicURL, cvPublicContent, cvPublicPDF, cvPublicOpenAPI)
	c.Data(http.StatusOK, "application/xml; charset=utf-8", []byte(body))
}

// serveCVOpenAPI returns the public OpenAPI description for CV endpoints.
// It takes a Gin request context and returns no values.
func serveCVOpenAPI(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
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
			"/api/v1/cv": gin.H{
				"get": gin.H{
					"summary":     "Read the current CV markdown.",
					"description": "Versioned public endpoint for current CV markdown and update metadata.",
					"operationId": "getCurrentCVV1",
					"parameters": []gin.H{
						{
							"name":        "include",
							"in":          "query",
							"required":    false,
							"description": "Optional comma-separated sections to emphasize, for example summary,skills,experience.",
							"schema":      gin.H{"type": "string"},
						},
					},
					"responses": cvOpenAPIContentResponses(),
				},
			},
			"/cv/content": gin.H{
				"get": gin.H{
					"summary":     "Read the current CV markdown.",
					"description": "Returns the public CV in markdown with update metadata.",
					"operationId": "getCurrentCV",
					"responses":   cvOpenAPIContentResponses(),
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
			"securitySchemes": gin.H{
				"ownerSSO": gin.H{
					"type":         "oauth2",
					"description":  "Owner-only SSO for write operations; public read endpoints do not require auth.",
					"flows":        gin.H{"authorizationCode": gin.H{"authorizationUrl": "https://sso.laisky.com/", "tokenUrl": "https://sso.laisky.com/oauth/token", "scopes": gin.H{"cv:read": "Read public CV data", "cv:write": "Owner-only CV editing"}}},
					"x-publicRead": true,
				},
			},
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
				"APIError": gin.H{
					"type":     "object",
					"required": []string{"error", "message"},
					"properties": gin.H{
						"error":      gin.H{"type": "string", "description": "Stable machine-readable error code."},
						"message":    gin.H{"type": "string", "description": "Human-readable error message."},
						"request_id": gin.H{"type": "string", "description": "Request identifier for support."},
					},
				},
				"Pagination": gin.H{
					"type":     "object",
					"required": []string{"limit", "next_cursor"},
					"properties": gin.H{
						"limit":       gin.H{"type": "integer", "minimum": 1, "maximum": 100},
						"next_cursor": gin.H{"type": "string"},
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

// cvOpenAPIContentResponses returns the shared OpenAPI response map for CV content endpoints.
// It takes no parameters and returns an OpenAPI-compatible response definition.
func cvOpenAPIContentResponses() gin.H {
	return gin.H{
		"200": gin.H{
			"description": "Current CV content.",
			"headers": gin.H{
				"X-RateLimit-Limit":     gin.H{"schema": gin.H{"type": "integer"}},
				"X-RateLimit-Remaining": gin.H{"schema": gin.H{"type": "integer"}},
				"X-RateLimit-Reset":     gin.H{"schema": gin.H{"type": "integer"}},
			},
			"content": gin.H{
				"application/json": gin.H{
					"schema": gin.H{"$ref": "#/components/schemas/CVContent"},
				},
			},
		},
		"400": gin.H{"description": "Invalid request.", "content": cvOpenAPIErrorContent()},
		"500": gin.H{"description": "Server error.", "content": cvOpenAPIErrorContent()},
	}
}

// cvOpenAPIErrorContent returns the shared OpenAPI error content schema.
// It takes no parameters and returns an OpenAPI-compatible content definition.
func cvOpenAPIErrorContent() gin.H {
	return gin.H{
		"application/json": gin.H{
			"schema": gin.H{"$ref": "#/components/schemas/APIError"},
		},
	}
}

// serveCVPricing returns a machine-readable no-cost pricing document.
// It takes a Gin request context and returns no values.
func serveCVPricing(c *gin.Context) {
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(`# Pricing

The CV API and public resume pages are free to read.

- Public CV API: $0
- PDF download: $0
- Recruiting contact by email: $0
- No checkout, subscription, or paid plan is required.
`))
}

// serveCVPricingHTML returns the human-readable no-cost pricing page.
// It takes a Gin request context and returns no values.
func serveCVPricingHTML(c *gin.Context) {
	serveSimpleCVHTML(c, "Pricing", "The public CV API, resume page, and PDF download are free to read. No payment or account is required.")
}

// serveCVAboutHTML returns a trust-anchor about page.
// It takes a Gin request context and returns no values.
func serveCVAboutHTML(c *gin.Context) {
	serveSimpleCVHTML(c, "About Zhonghua (Laisky) Cai", "Senior Software Engineer in Ottawa focused on backend, infrastructure, Linux services, platform engineering, Kubernetes, and security.")
}

// serveCVContactHTML returns a trust-anchor contact page.
// It takes a Gin request context and returns no values.
func serveCVContactHTML(c *gin.Context) {
	serveSimpleCVHTML(c, "Contact", "For recruiting, interviews, references, and role-fit questions, email job@laisky.com. LinkedIn: https://www.linkedin.com/in/laisky-cai-14237926/.")
}

// serveCVPrivacyHTML returns a trust-anchor privacy page.
// It takes a Gin request context and returns no values.
func serveCVPrivacyHTML(c *gin.Context) {
	serveSimpleCVHTML(c, "Privacy", "This CV site publishes public resume information. Public read endpoints do not require an account, cookies, or payment.")
}

// serveCVDeveloperHTML returns a developer portal page for agents and integrators.
// It takes a Gin request context and returns no values.
func serveCVDeveloperHTML(c *gin.Context) {
	body := "Developer resources: OpenAPI at https://cv.laisky.com/openapi.json, API catalog at https://cv.laisky.com/.well-known/api-catalog, versioned CV API at https://cv.laisky.com/api/v1/cv, agent instructions at https://cv.laisky.com/agents.md, and auth guide at https://cv.laisky.com/auth.md."
	serveSimpleCVHTML(c, "CV Developer Portal", body)
}

// serveSimpleCVHTML returns a small crawlable HTML page with the provided title and body.
// It takes a Gin request context, title, and body text and returns no values.
func serveSimpleCVHTML(c *gin.Context, title string, body string) {
	escapedTitle := html.EscapeString(title)
	escapedBody := html.EscapeString(body)
	page := fmt.Sprintf(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>%s | Laisky CV</title><link rel="canonical" href="https://cv.laisky.com/"><meta name="description" content="%s"></head><body><main><h1>%s</h1><p>%s</p><p><a href="/">CV home</a> <a href="/developer">Developer portal</a> <a href="/openapi.json">OpenAPI</a></p></main></body></html>`, escapedTitle, escapedBody, escapedTitle, escapedBody)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

// serveCVAPIRoot returns a public API root document or auth hint.
// It takes a Gin request context and returns no values.
func serveCVAPIRoot(c *gin.Context) {
	setCVAPIDiscoveryHeaders(c)
	c.Header("WWW-Authenticate", `Bearer resource_metadata="https://cv.laisky.com/.well-known/oauth-protected-resource"`)
	c.JSON(http.StatusOK, gin.H{
		"name":        "Zhonghua (Laisky) Cai CV API",
		"version":     "v1",
		"description": "Public read API for CV content.",
		"openapi":     cvPublicOpenAPI,
		"endpoints": []gin.H{
			{"method": "GET", "path": "/api/v1/cv", "auth": "none", "description": "Read current CV markdown."},
			{"method": "GET", "path": "/cv/pdf", "auth": "none", "description": "Download current CV PDF."},
		},
	})
}

// serveCVAICatalog returns a public catalog for agent discovery.
// It takes a Gin request context and returns no values.
func serveCVAICatalog(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"specVersion": "0.1",
		"name":        "Zhonghua (Laisky) Cai CV",
		"description": "Public CV, contact paths, markdown API, PDF endpoint, and MCP server metadata.",
		"entries": []gin.H{
			{
				"urn":       "urn:air:cv.laisky.com:openapi",
				"mediaType": "application/vnd.oai.openapi+json;version=3.1",
				"url":       cvPublicOpenAPI,
			},
			{
				"urn":       "urn:air:cv.laisky.com:llms",
				"mediaType": "text/markdown",
				"url":       cvPublicURL + "llms.txt",
			},
			{
				"urn":       "urn:air:cv.laisky.com:mcp",
				"mediaType": "application/mcp-server-card+json",
				"url":       "https://mcp.laisky.com/.well-known/mcp/server-card.json",
			},
		},
	})
}

// serveCVAPICatalog returns an RFC 9727-style API catalog.
// It takes a Gin request context and returns no values.
func serveCVAPICatalog(c *gin.Context) {
	c.Header("Content-Type", "application/api-catalog+json; charset=utf-8")
	c.JSON(http.StatusOK, gin.H{
		"api_catalog_version": "1",
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

// serveCVA2AAgentCard returns an agent card for direct CV question-answering.
// It takes a Gin request context and returns no values.
func serveCVA2AAgentCard(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":        "Laisky CV Agent",
		"description": "Answers questions about Zhonghua (Laisky) Cai's public CV using read-only public data.",
		"url":         cvPublicURL,
		"version":     "1.0.0",
		"capabilities": gin.H{
			"streaming":              false,
			"pushNotifications":      false,
			"stateTransitionHistory": false,
		},
		"defaultInputModes":  []string{"text/plain", "text/markdown"},
		"defaultOutputModes": []string{"text/plain", "text/markdown"},
		"skills": []gin.H{
			{
				"id":          "read_cv",
				"name":        "Read public CV",
				"description": "Fetch and summarize Zhonghua (Laisky) Cai's public resume.",
			},
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
		"icon":        cvPublicIcon,
		"transport":   "streamable-http",
		"auth":        "server-dependent; public discovery available",
		"related": gin.H{
			"cv":      cvPublicURL,
			"openapi": cvPublicOpenAPI,
			"contact": cvPublicContact,
		},
	})
}

// serveCVOAuthProtectedResource returns OAuth protected resource metadata for agents.
// It takes a Gin request context and returns no values.
func serveCVOAuthProtectedResource(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"resource":                 "https://cv.laisky.com",
		"authorization_servers":    []string{"https://sso.laisky.com"},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{"cv:read", "cv:write"},
	})
}

// serveCVOAuthAuthorizationServer returns OAuth authorization server metadata for agents.
// It takes a Gin request context and returns no values.
func serveCVOAuthAuthorizationServer(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                "https://sso.laisky.com",
		"authorization_endpoint":                "https://sso.laisky.com/",
		"token_endpoint":                        "https://sso.laisky.com/oauth/token",
		"code_challenge_methods_supported":      []string{"S256"},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"scopes_supported":                      []string{"cv:read", "cv:write"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "none"},
	})
}

// serveCVHTTPSignatureDirectory returns a web bot auth directory placeholder.
// It takes a Gin request context and returns no values.
func serveCVHTTPSignatureDirectory(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":        "Zhonghua (Laisky) Cai CV",
		"description": "Public read endpoints do not require HTTP message signatures.",
		"policy":      "allow-public-read",
		"resources":   []string{cvPublicURL, cvPublicContent, cvPublicOpenAPI},
	})
}

// buildCVIndexMarkdown builds the markdown homepage body for agents.
// It takes no parameters and returns markdown text.
func buildCVIndexMarkdown() string {
	return `# Zhonghua (Laisky) Cai

Senior Software Engineer focused on backend, infrastructure, Linux services, Kubernetes, platform engineering, and security.

## Summary
Zhonghua (Laisky) Cai is based in Ottawa, Canada and is open to remote Canada/US roles. He has 10+ years of experience building and operating distributed backend systems, internal platforms, PaaS/SaaS infrastructure, CI/CD, observability, and security platforms.

## Core Skills
- Go, Python, JavaScript, TypeScript
- Backend API design, distributed systems, concurrency, performance tuning
- Kubernetes, Docker, Linux operations, CI/CD, tracing, observability
- AWS, self-hosted infrastructure, Postgres, MongoDB, Redis, MinIO
- Security engineering, PKI, KMS, zero-trust patterns, SGX, SEV-SNP, TDX, TPM

## Public Resources
- [CV markdown API](https://cv.laisky.com/api/v1/cv)
- [OpenAPI](https://cv.laisky.com/openapi.json)
- [API catalog](https://cv.laisky.com/.well-known/api-catalog)
- [Agent instructions](https://cv.laisky.com/agents.md)
- [Auth guide](https://cv.laisky.com/auth.md)
- [PDF](https://cv.laisky.com/cv/pdf)
- [GitHub](https://github.com/Laisky)
- [LinkedIn](https://www.linkedin.com/in/laisky-cai-14237926/)
- [Blog](https://blog.laisky.com/)

## Contact
Email job@laisky.com for recruiting, interviews, references, and role-fit questions.
`
}

// buildCVAgentHTML builds a crawlable HTML homepage for the CV host.
// It takes whether agent mode was requested and returns HTML text.
func buildCVAgentHTML(agentMode bool) string {
	modeNote := "Human and agent-readable CV homepage."
	if agentMode {
		modeNote = "Dedicated agent-mode CV homepage with direct machine-readable resource links."
	}
	jsonLD := `{"@context":"https://schema.org","@type":"ProfilePage","name":"Zhonghua (Laisky) Cai CV","url":"https://cv.laisky.com/","description":"Senior Software Engineer focused on backend, infrastructure, Linux services, platform engineering, and security.","mainEntity":{"@type":"Person","name":"Zhonghua (Laisky) Cai","alternateName":"Laisky Cai","email":"job@laisky.com","jobTitle":"Senior Software Engineer","address":{"@type":"PostalAddress","addressLocality":"Ottawa","addressRegion":"ON","addressCountry":"CA"},"sameAs":["https://github.com/Laisky","https://www.linkedin.com/in/laisky-cai-14237926/","https://blog.laisky.com/"]},"speakable":{"@type":"SpeakableSpecification","cssSelector":["h1","main p"]}}`
	orgLD := `{"@context":"https://schema.org","@type":"Organization","name":"Laisky CV","url":"https://cv.laisky.com/","logo":"https://s3.laisky.com/uploads/2025/12/favicon.ico","contactPoint":{"@type":"ContactPoint","email":"job@laisky.com","contactType":"recruiting"},"sameAs":["https://github.com/Laisky","https://blog.laisky.com/"]}`
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Zhonghua (Laisky) Cai | CV</title>
  <link rel="canonical" href="https://cv.laisky.com/">
  <link rel="alternate" type="text/markdown" href="https://cv.laisky.com/index.md">
  <link rel="service-desc" type="application/vnd.oai.openapi+json;version=3.1" href="https://cv.laisky.com/openapi.json">
  <meta name="description" content="CV of Zhonghua (Laisky) Cai, Senior Software Engineer focused on backend, infrastructure, Linux services, platform engineering, and security.">
  <meta property="og:type" content="profile">
  <meta property="og:title" content="Zhonghua (Laisky) Cai | CV">
  <meta property="og:description" content="Senior Software Engineer focused on backend, infrastructure, Linux services, platform engineering, and security.">
  <meta property="og:image" content="%s">
  <script type="application/ld+json">%s</script>
  <script type="application/ld+json">%s</script>
</head>
<body>
  <header><nav><a href="/">CV</a> <a href="/developer">Developer</a> <a href="/about">About</a> <a href="/contact">Contact</a> <a href="/privacy">Privacy</a></nav></header>
  <main>
    <h1>Zhonghua (Laisky) Cai</h1>
    <p>%s</p>
    <p>Senior Software Engineer in Ottawa, Canada. Open to remote Canada/US roles. Focus areas: backend systems, infrastructure, Linux services, Kubernetes, CI/CD, observability, platform engineering, and security.</p>
    <h2>Agent Resources</h2>
    <ul>
      <li><a href="/api/v1/cv">Versioned CV API</a></li>
      <li><a href="/cv/content">CV markdown API</a></li>
      <li><a href="/openapi.json">OpenAPI document</a></li>
      <li><a href="/.well-known/api-catalog">API catalog</a></li>
      <li><a href="/.well-known/ai-catalog.json">Agent resource catalog</a></li>
      <li><a href="/agents.md">Agent instructions</a></li>
      <li><a href="/auth.md">Auth guide</a></li>
      <li><a href="/llms.txt">llms.txt</a></li>
      <li><a href="/pricing.md">Pricing</a></li>
      <li><a href="/cv/pdf">PDF CV</a></li>
    </ul>
    <h2>Contact</h2>
    <p>Email <a href="mailto:job@laisky.com">job@laisky.com</a>. LinkedIn: <a href="https://www.linkedin.com/in/laisky-cai-14237926/">profile</a>. GitHub: <a href="https://github.com/Laisky">Laisky</a>.</p>
  </main>
</body>
</html>`, cvPublicIcon, jsonLD, orgLD, html.EscapeString(modeNote))
}
