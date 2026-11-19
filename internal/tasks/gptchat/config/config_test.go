package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSetupConfigMemoryDefaults verifies SetupConfig fills current memory and web fetch defaults.
func TestSetupConfigMemoryDefaults(t *testing.T) {
	setConfig(t, "openai.token", "srv-token")
	setConfig(t, "openai.enable_memory", true)
	setConfig(t, "openai.memory_project", "")
	setConfig(t, "openai.memory_storage_mcp_url", "https://mcp.example.com")
	setConfig(t, "openai.memory_llm_timeout_seconds", 0)
	setConfig(t, "openai.memory_llm_max_output_tokens", 0)
	setConfig(t, "openai.web_fetch.jina.prefix", "")
	setConfig(t, "openai.web_fetch.defuddle.prefix", "")
	setConfig(t, "openai.web_fetch.scrapeless.api", "")
	setConfig(t, "openai.web_fetch.scrapeless.actor", "")
	setConfig(t, "openai.web_fetch.scrapeless.proxy_country", "")
	setConfig(t, "openai.web_fetch.scrapeless.enabled", false)
	setConfig(t, "openai.web_fetch.scrapeless.api_key", "")
	setConfig(t, "openai.web_fetch.firecrawl.api", "")
	setConfig(t, "openai.web_fetch.firecrawl.enabled", false)
	setConfig(t, "openai.web_fetch.firecrawl.api_key", "")

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
	require.Equal(t, "https://api.firecrawl.dev/v2/scrape", Config.WebFetch.Firecrawl.API)
}

// TestSetupConfigMemoryValidation verifies blank memory storage URL is rejected when memory is enabled.
func TestSetupConfigMemoryValidation(t *testing.T) {
	setConfig(t, "openai.token", "srv-token")
	setConfig(t, "openai.enable_memory", true)
	setConfig(t, "openai.memory_project", "gptchat")
	setConfig(t, "openai.memory_storage_mcp_url", "   ")
	setConfig(t, "openai.memory_llm_timeout_seconds", 15)
	setConfig(t, "openai.memory_llm_max_output_tokens", 512)
	setConfig(t, "openai.web_fetch.scrapeless.enabled", false)
	setConfig(t, "openai.web_fetch.scrapeless.api_key", "")

	err := SetupConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory_storage_mcp_url")
}

// TestSetupConfigScrapelessValidation verifies scrapeless requires an API key when enabled.
func TestSetupConfigScrapelessValidation(t *testing.T) {
	setConfig(t, "openai.token", "srv-token")
	setConfig(t, "openai.enable_memory", false)
	setConfig(t, "openai.memory_project", "go-ramjet-memory")
	setConfig(t, "openai.memory_storage_mcp_url", "https://mcp.example.com")
	setConfig(t, "openai.memory_llm_timeout_seconds", 15)
	setConfig(t, "openai.memory_llm_max_output_tokens", 512)
	setConfig(t, "openai.web_fetch.scrapeless.enabled", true)
	setConfig(t, "openai.web_fetch.scrapeless.api_key", "   ")

	err := SetupConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "scrapeless.api_key")
}

// TestSetupConfigFirecrawlValidation verifies firecrawl requires an API key when enabled.
func TestSetupConfigFirecrawlValidation(t *testing.T) {
	setConfig(t, "openai.token", "srv-token")
	setConfig(t, "openai.enable_memory", false)
	setConfig(t, "openai.memory_project", "go-ramjet-memory")
	setConfig(t, "openai.memory_storage_mcp_url", "https://mcp.example.com")
	setConfig(t, "openai.memory_llm_timeout_seconds", 15)
	setConfig(t, "openai.memory_llm_max_output_tokens", 512)
	setConfig(t, "openai.web_fetch.scrapeless.enabled", false)
	setConfig(t, "openai.web_fetch.scrapeless.api_key", "")
	setConfig(t, "openai.web_fetch.firecrawl.enabled", true)
	setConfig(t, "openai.web_fetch.firecrawl.api_key", "   ")

	err := SetupConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "firecrawl.api_key")
}

// baseVoiceConfig sets the unrelated keys SetupConfig requires so a voice test
// exercises only the voice defaults.
func baseVoiceConfig(t *testing.T) {
	t.Helper()
	setConfig(t, "openai.token", "srv-token")
	setConfig(t, "openai.enable_memory", false)
	setConfig(t, "openai.memory_project", "go-ramjet-memory")
	setConfig(t, "openai.web_fetch.scrapeless.enabled", false)
	setConfig(t, "openai.web_fetch.firecrawl.enabled", false)
}

// TestSetupConfigVoiceDefaults verifies the voice plugin and realtime model fall back
// to the server defaults when the deployment does not configure them.
func TestSetupConfigVoiceDefaults(t *testing.T) {
	baseVoiceConfig(t)
	setConfig(t, "openai.voice.plugin", "")
	setConfig(t, "openai.voice.realtime_model", "")

	require.NoError(t, SetupConfig())
	require.Equal(t, VoicePluginRealtime, Config.Voice.Plugin)
	require.Equal(t, "gpt-realtime-2.1-mini", Config.Voice.RealtimeModel)
}

// TestSetupConfigVoiceOverrides verifies a deployment can pick the other plugin and model.
func TestSetupConfigVoiceOverrides(t *testing.T) {
	baseVoiceConfig(t)
	setConfig(t, "openai.voice.plugin", "Whisper")
	setConfig(t, "openai.voice.realtime_model", "gpt-realtime-2.1")

	require.NoError(t, SetupConfig())
	// Case is normalized so a capitalized value in YAML still matches the client ids.
	require.Equal(t, VoicePluginWhisper, Config.Voice.Plugin)
	require.Equal(t, "gpt-realtime-2.1", Config.Voice.RealtimeModel)
}

// TestSetupConfigVoiceRejectsUnknownPlugin verifies a typo fails at boot rather than
// silently leaving the browser with no usable audio implementation.
func TestSetupConfigVoiceRejectsUnknownPlugin(t *testing.T) {
	baseVoiceConfig(t)
	setConfig(t, "openai.voice.plugin", "realtim")
	setConfig(t, "openai.voice.realtime_model", "")

	err := SetupConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "openai.voice.plugin")
}

// TestUserConfigCarriesServerVoiceSettings verifies every authenticated user receives the
// server's voice settings, overriding anything a per-user entry happens to hold.
func TestUserConfigCarriesServerVoiceSettings(t *testing.T) {
	baseVoiceConfig(t)
	setConfig(t, "openai.voice.plugin", "whisper")
	setConfig(t, "openai.voice.realtime_model", "gpt-realtime-2.1-mini")
	require.NoError(t, SetupConfig())

	user := &UserConfig{
		UserName:      "someone",
		Token:         "tok",
		AllowedModels: []string{"*"},
		Voice:         VoiceConfig{Plugin: "stale", RealtimeModel: "stale-model"},
	}
	require.NoError(t, user.Valid())
	require.Equal(t, VoicePluginWhisper, user.Voice.Plugin)
	require.Equal(t, "gpt-realtime-2.1-mini", user.Voice.RealtimeModel)
}

// TestUserConfigDefaultUserNameIsFrozen pins the default UserName derived from
// a configured user token, hex(SHA256(token))[:16], with a vector computed
// independently (Python hashlib).
//
// CodeQL go/weak-sensitive-data-hashing #68 flags this SHA-256 call as password
// hashing. It is not: the input is an API token (operator-issued in
// openai.user_tokens or resolved from the request), not a human-chosen
// password; the digest is never stored as a verifier nor compared to
// authenticate anyone; the 16-hex prefix is only a fallback, stable user
// identifier (quota, billing, logs) for entries without user_name. A slow KDF
// would add nothing, and any change would re-key existing per-user records.
func TestUserConfigDefaultUserNameIsFrozen(t *testing.T) {
	baseVoiceConfig(t)
	setConfig(t, "openai.voice.plugin", "")
	setConfig(t, "openai.voice.realtime_model", "")
	require.NoError(t, SetupConfig())

	derived := &UserConfig{Token: "synthetic-user-token", AllowedModels: []string{"*"}}
	require.NoError(t, derived.Valid())
	require.Equal(t, "a18124dce2d892dc", derived.UserName)

	explicit := &UserConfig{UserName: "alice", Token: "synthetic-user-token", AllowedModels: []string{"*"}}
	require.NoError(t, explicit.Valid())
	require.Equal(t, "alice", explicit.UserName, "an explicit user_name must win")
}

// TestSetupConfigWebFetchQueryForwarding checks the actual mapstructure wire
// setting. The query-disclosure policy defaults to false and requires an
// operator override in the worker's openai.web_fetch configuration.
func TestSetupConfigWebFetchQueryForwarding(t *testing.T) {
	baseVoiceConfig(t)
	setConfig(t, "openai.voice.plugin", "realtime")
	setConfig(t, "openai.voice.realtime_model", "")
	for _, enabled := range []bool{false, true} {
		setConfig(t, "openai.web_fetch.allow_query_forwarding", enabled)
		require.NoError(t, SetupConfig())
		require.Equal(t, enabled, Config.WebFetch.AllowQueryForwarding)
	}
}
