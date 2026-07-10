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
		"transports": []gin.H{
			{"type": "streamable-http", "url": "https://cv.laisky.com/mcp"},
			{"type": "server-card", "url": "https://cv.laisky.com/.well-known/mcp/server-card.json"},
			{"type": "streamable-http", "url": "https://mcp.laisky.com"},
			{"type": "server-card", "url": "https://mcp.laisky.com/.well-known/mcp/server-card.json"},
		},
		"auth":        "server-dependent; public discovery available",
		"server_card": "https://cv.laisky.com/.well-known/mcp/server-card.json",
		"related": gin.H{
			"cv":      cvPublicURL,
			"openapi": cvPublicOpenAPI,
			"contact": cvPublicContact,
			"repo":    "https://github.com/Laisky/go-ramjet",
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
		"agent_auth": gin.H{
			"register_uri":             "https://sso.laisky.com/",
			"identity_types_supported": []string{"anonymous", "user"},
		},
	})
}

// serveCVOAuthAuthorizationServer returns OAuth authorization server metadata for agents.
// It takes a Gin request context and returns no values.
func serveCVOAuthAuthorizationServer(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                "https://sso.laisky.com",
		"authorization_endpoint":                "https://sso.laisky.com/",
		"token_endpoint":                        "https://sso.laisky.com/oauth/token",
		"agent_auth_register_endpoint":          "https://sso.laisky.com/",
		"agent_auth_registration_endpoint":      "https://sso.laisky.com/",
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
		"keys": []gin.H{
			{
				"kty": "OKP",
				"crv": "Ed25519",
				"kid": "cv-public-read-placeholder-2026",
				"use": "sig",
				"x":   "11qYAYKxCrfVS_3XNvgc7vB8Z50to6dc0O3s6zK-T0Y",
				"nbf": 1767225600,
				"exp": 2114380799,
			},
		},
		"resources": []string{cvPublicURL, cvPublicContent, cvPublicOpenAPI},
	})
}
