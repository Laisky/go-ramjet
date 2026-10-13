package http

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	gcompress "github.com/Laisky/go-utils/v6/compress"
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

// TestUserSyncStoredRecordCompatibility checks a frozen original-pin record
// through the exact HKDF, AES-GCM, and gzip APIs used by cloud configuration sync.
// New output is read independently using only standard-library primitives.
func TestUserSyncStoredRecordCompatibility(t *testing.T) {
	const syncKey = "synthetic-sync-key-for-compatibility"
	const frozen = "8ca5bd37759179b064f7e8ce2fe1477fc8f41ee4b5da97af1422dd33d9a3b2e708408b2f0399cb78fe923cb06825646fa18a39f188531edbbca346ccb156dd9c994d630abfdad72c8b5f637e0b66a1367cc67180c1116ebb005e18908decfe2b70683f54ab42f1d8695fd9e816e4424f0714"
	const upgradedWriterRecord = "4ca3796b254e97b9dbab3c334b063cdfb7994279fb0acc6132811aeec4d56003d87a9eafff76056f5d57e09e9dc4b9ab0512f7a7fdb98159efe1146fb8093485d788e58be9337e7a57cf379284889347c011dd67e72d0004dee76fee964466ddbc5f10d5ac39145dec2ebdfd38cfccde78a4"
	expected := []byte(`{"theme":"synthetic-legacy","folders":[]}`)
	key, err := gcrypto.DeriveKeyByHKDF([]byte(syncKey), nil, 32)
	require.NoError(t, err)
	old, err := hex.DecodeString(frozen)
	require.NoError(t, err)
	var restored bytes.Buffer
	for _, record := range []string{frozen, upgradedWriterRecord} {
		encoded, err := hex.DecodeString(record)
		require.NoError(t, err)
		compressed, err := gcrypto.AEADDecrypt(key, encoded, nil)
		require.NoError(t, err)
		restored.Reset()
		require.NoError(t, gcompress.GzDecompress(bytes.NewReader(compressed), &restored))
		require.Equal(t, expected, restored.Bytes())
	}

	independentKey, err := hkdf.Key(sha256.New, []byte(syncKey), nil, "", 32)
	require.NoError(t, err)
	require.Equal(t, independentKey, key)
	block, err := aes.NewCipher(independentKey)
	require.NoError(t, err)
	independentCipher, err := cipher.NewGCM(block)
	require.NoError(t, err)
	for _, body := range [][]byte{expected, {}} {
		var zipped bytes.Buffer
		require.NoError(t, gcompress.GzCompress(bytes.NewReader(body), &zipped))
		written, err := gcrypto.AEADEncrypt(key, zipped.Bytes(), nil)
		require.NoError(t, err)
		zippedBytes, err := independentCipher.Open(nil, written[:independentCipher.NonceSize()], written[independentCipher.NonceSize():], nil)
		require.NoError(t, err)
		reader, err := gzip.NewReader(bytes.NewReader(zippedBytes))
		require.NoError(t, err)
		decoded, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		require.Equal(t, body, decoded)
	}
	wrongKey, err := gcrypto.DeriveKeyByHKDF([]byte(syncKey+"-wrong"), nil, 32)
	require.NoError(t, err)
	_, err = gcrypto.AEADDecrypt(wrongKey, old, nil)
	require.Error(t, err)
	restored.Reset()
	require.Error(t, gcompress.GzDecompress(bytes.NewReader([]byte("not gzip")), &restored))
}
