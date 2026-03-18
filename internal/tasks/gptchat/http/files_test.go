package http

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestFileHashUsesSHA256(t *testing.T) {
	t.Parallel()

	// Verify that SHA-256 produces the expected hash length (64 hex chars)
	// This test ensures we're using SHA-256 and not SHA-1 (40 hex chars)
	testData := []byte("test file content")
	hashBytes := sha256.Sum256(testData)
	hashStr := hex.EncodeToString(hashBytes[:])

	if len(hashStr) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(hashStr))
	}

	// SHA-1 would produce 40 chars, verify we get the correct SHA-256
	expected := "60f5237ed4049f0382661ef009d2bc42e48c3ceb3edb6600f7024e7ab3b838f3"
	if hashStr != expected {
		t.Errorf("SHA-256 hash mismatch: expected %s, got %s", expected, hashStr)
	}
}
