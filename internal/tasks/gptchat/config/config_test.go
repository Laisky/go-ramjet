package config

import (
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/stretchr/testify/require"
)

func TestSetupConfigMemoryDefaults(t *testing.T) {
	gconfig.Shared.Set("openai.token", "srv-token")
	gconfig.Shared.Set("openai.enable_memory", true)
	gconfig.Shared.Set("openai.memory_project", "")
	gconfig.Shared.Set("openai.memory_storage_mcp_url", "https://mcp.example.com")
	gconfig.Shared.Set("openai.memory_llm_timeout_seconds", 0)
	gconfig.Shared.Set("openai.memory_llm_max_output_tokens", 0)

	err := SetupConfig()
	require.NoError(t, err)
	require.Equal(t, "gptchat", Config.MemoryProject)
	require.Equal(t, "https://mcp.example.com", Config.MemoryStorageMCPURL)
	require.Equal(t, 15, Config.MemoryLLMTimeoutSeconds)
	require.Equal(t, 512, Config.MemoryLLMMaxOutputTokens)
}

func TestSetupConfigMemoryValidation(t *testing.T) {
	gconfig.Shared.Set("openai.token", "srv-token")
	gconfig.Shared.Set("openai.enable_memory", true)
	gconfig.Shared.Set("openai.memory_project", "gptchat")
	gconfig.Shared.Set("openai.memory_storage_mcp_url", "")
	gconfig.Shared.Set("openai.memory_llm_timeout_seconds", 15)
	gconfig.Shared.Set("openai.memory_llm_max_output_tokens", 512)

	err := SetupConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory_storage_mcp_url")
}
