package http

import (
	"crypto/sha256"
	"encoding/hex"
)

// ramjetCredentialScope derives a private bearer-capability namespace from the
// complete canonical caller key. It does not attribute legacy data or establish
// an account identity. Its caller must first validate the original credentials.
func ramjetCredentialScope(key string) string {
	digest := sha256.Sum256([]byte("ramjet-private-key-capability-v2\x00" + key))
	return "ckv2-" + hex.EncodeToString(digest[:])
}
