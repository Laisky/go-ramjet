package http

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"testing"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/stretchr/testify/require"
)

// TestUserSyncCiphertextCompatibility checks the nonce/ciphertext/tag envelope
// used by user cloud sync against an independent AES-256-GCM known-answer vector
// and a standard-library reader, including wrong-key and corruption controls.
func TestUserSyncCiphertextCompatibility(t *testing.T) {
	key := make([]byte, 32)
	stored, err := hex.DecodeString("000000000000000000000000cea7403d4d606b6e074ec5d3baf39d18d0d1c8a799996bf0265b98b5d48ab919")
	require.NoError(t, err)
	plaintext, err := gcrypto.AEADDecrypt(key, stored, nil)
	require.NoError(t, err)
	require.Equal(t, make([]byte, 16), plaintext)

	current, err := gcrypto.AEADEncrypt(key, []byte("synthetic cloud sync payload"), nil)
	require.NoError(t, err)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	reader, err := cipher.NewGCM(block)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(current), reader.NonceSize()+reader.Overhead())
	decoded, err := reader.Open(nil, current[:reader.NonceSize()], current[reader.NonceSize():], nil)
	require.NoError(t, err)
	require.Equal(t, []byte("synthetic cloud sync payload"), decoded)

	corrupted := bytes.Clone(stored)
	corrupted[len(corrupted)-1] ^= 1
	_, err = gcrypto.AEADDecrypt(key, corrupted, nil)
	require.Error(t, err)
	wrongKey := bytes.Repeat([]byte{1}, 32)
	_, err = gcrypto.AEADDecrypt(wrongKey, stored, nil)
	require.Error(t, err)
	_, err = gcrypto.AEADDecrypt(key, stored[:5], nil)
	require.Error(t, err)
}
