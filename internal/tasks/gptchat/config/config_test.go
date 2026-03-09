package config

import (
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/stretchr/testify/require"
)

// TestSetupConfigMemoryDefaults verifies SetupConfig fills current memory and web fetch defaults.
func TestSetupConfigMemoryDefaults(t *testing.T) {
	gconfig.Shared.Set("openai.token", "srv-token")
	gconfig.Shared.Set("openai.enable_memory", true)
	gconfig.Shared.Set("openai.memory_project", "")
	gconfig.Shared.Set("openai.memory_storage_mcp_url", "https://mcp.example.com")
	gconfig.Shared.Set("openai.memory_llm_timeout_seconds", 0)
	gconfig.Shared.Set("openai.memory_llm_max_output_tokens", 0)
	gconfig.Shared.Set("openai.web_fetch.jina.prefix", "")
	gconfig.Shared.Set("openai.web_fetch.defuddle.prefix", "")
	gconfig.Shared.Set("openai.web_fetch.scrapeless.api", "")
	gconfig.Shared.Set("openai.web_fetch.scrapeless.actor", "")
	gconfig.Shared.Set("openai.web_fetch.scrapeless.proxy_country", "")
	gconfig.Shared.Set("openai.web_fetch.scrapeless.enabled", false)
	gconfig.Shared.Set("openai.web_fetch.scrapeless.api_key", "")

	err := SetupConfig()
	require.NoError(t, err)
	require.Equal(t, "go-ramjet-memory", Config.MemoryProject)
	require.Equal(t, "https://mcp.example.com", Config.MemoryStorageMCPURL)
	require.Equal(t, 15, Config.MemoryLLMTimeoutSeconds)
	require.Equal(t, 512, Config.MemoryLLMMaxOutputTokens)
	require.Equal(t, "https://r.jina.ai/", Config.WebFetch.Jina.Prefix)
	require.Equal(t, "https://defuddle.md/", Config.WebFetch.Defuddle.Prefix)
	require.Equal(t, "https://api.scrapeless.com/api/v2/unlocker/request", Config.WebFetch.Scrapeless.API)
	require.Equal(t, "unlocker.webunlocker", Config.WebFetch.Scrapeless.Actor)
	require.Equal(t, "ANY", Config.WebFetch.Scrapeless.ProxyCountry)
}

// TestSetupConfigMemoryValidation verifies blank memory storage URL is rejected when memory is enabled.
func TestSetupConfigMemoryValidation(t *testing.T) {
	gconfig.Shared.Set("openai.token", "srv-token")
	gconfig.Shared.Set("openai.enable_memory", true)
	gconfig.Shared.Set("openai.memory_project", "gptchat")
	gconfig.Shared.Set("openai.memory_storage_mcp_url", "   ")
	gconfig.Shared.Set("openai.memory_llm_timeout_seconds", 15)
	gconfig.Shared.Set("openai.memory_llm_max_output_tokens", 512)
	gconfig.Shared.Set("openai.web_fetch.scrapeless.enabled", false)
	gconfig.Shared.Set("openai.web_fetch.scrapeless.api_key", "")

	err := SetupConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory_storage_mcp_url")
}

// TestSetupConfigScrapelessValidation verifies scrapeless requires an API key when enabled.
func TestSetupConfigScrapelessValidation(t *testing.T) {
	gconfig.Shared.Set("openai.token", "srv-token")
	gconfig.Shared.Set("openai.enable_memory", false)
	gconfig.Shared.Set("openai.memory_project", "go-ramjet-memory")
	gconfig.Shared.Set("openai.memory_storage_mcp_url", "https://mcp.example.com")
	gconfig.Shared.Set("openai.memory_llm_timeout_seconds", 15)
	gconfig.Shared.Set("openai.memory_llm_max_output_tokens", 512)
	gconfig.Shared.Set("openai.web_fetch.scrapeless.enabled", true)
	gconfig.Shared.Set("openai.web_fetch.scrapeless.api_key", "   ")

	err := SetupConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "scrapeless.api_key")
}
