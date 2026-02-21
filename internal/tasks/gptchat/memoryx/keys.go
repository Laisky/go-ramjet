package memoryx

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// RuntimeKeys stores stable identifiers used by one memory turn.
type RuntimeKeys struct {
	Project   string
	SessionID string
	UserID    string
	TurnID    string
}

// BuildRuntimeKeys builds memory runtime keys for one chat request.
//
// Parameters:
//   - conf: Global gptchat openai config.
//   - user: Authenticated user config.
//   - header: Current request headers.
//
// Returns:
//   - RuntimeKeys: Stable identifiers for one memory turn.
func BuildRuntimeKeys(conf *config.OpenAI, user *config.UserConfig, header http.Header) RuntimeKeys {
	_ = header
	keys := RuntimeKeys{UserID: strings.TrimSpace(user.UserName), TurnID: uuid.NewString()}
	keys.Project = strings.TrimSpace(conf.MemoryProject)
	if keys.Project == "" {
		keys.Project = "go-ramjet-memory"
	}

	keys.SessionID = sessionIDFromAPIKey(user)
	if keys.SessionID == "" {
		keys.SessionID = keys.UserID
	}

	return keys
}

// sessionIDFromAPIKey derives a stable memory session id from user tokens.
//
// Parameters:
//   - user: Authenticated user config.
//
// Returns:
//   - string: Stable hashed session id for memory isolation.
func sessionIDFromAPIKey(user *config.UserConfig) string {
	if user == nil {
		return ""
	}

	raw := strings.TrimSpace(user.Token)
	if raw == "" {
		raw = strings.TrimSpace(user.OpenaiToken)
	}
	if raw == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(raw))
	return "ak-" + hex.EncodeToString(sum[:16])
}
