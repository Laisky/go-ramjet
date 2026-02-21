package memoryx

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

func TestBuildRuntimeKeysUserScope(t *testing.T) {
	conf := &config.OpenAI{MemoryProject: "gptchat"}
	user := &config.UserConfig{UserName: "alice", Token: "tenant-alice", OpenaiToken: "sk-shared"}

	keys := BuildRuntimeKeys(conf, user, http.Header{})
	require.Equal(t, "gptchat", keys.Project)
	require.Equal(t, "alice", keys.UserID)
	sum := sha256.Sum256([]byte("tenant-alice"))
	require.Equal(t, "ak-"+hex.EncodeToString(sum[:16]), keys.SessionID)
	require.NotEmpty(t, keys.TurnID)
}

func TestBuildRuntimeKeysFallbackToOpenAIToken(t *testing.T) {
	conf := &config.OpenAI{MemoryProject: "gptchat"}
	user := &config.UserConfig{UserName: "alice", OpenaiToken: "sk-abc"}

	keys := BuildRuntimeKeys(conf, user, http.Header{})
	sum := sha256.Sum256([]byte("sk-abc"))
	require.Equal(t, "ak-"+hex.EncodeToString(sum[:16]), keys.SessionID)
}

func TestBuildRuntimeKeysFallbackToUserIDWithoutAPIKey(t *testing.T) {
	conf := &config.OpenAI{MemoryProject: "gptchat"}
	user := &config.UserConfig{UserName: "alice"}
	header := http.Header{}

	keys := BuildRuntimeKeys(conf, user, header)
	require.Equal(t, "alice", keys.SessionID)
}
