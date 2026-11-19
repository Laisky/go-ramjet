package http

import (
	"errors"
	"io"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

type errReader struct {
	err error
}

// Read implements io.Reader and always returns the configured error.
func (r errReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

// TestIsS3NoSuchKey ensures we can reliably detect missing-key errors.
func TestIsS3NoSuchKey(t *testing.T) {
	t.Parallel()

	req := require.New(t)

	err := minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404, Message: "The specified key does not exist."}
	req.True(isS3NoSuchKey(err))
	req.True(isS3NoSuchKey(errors.New("The specified key does not exist.")))
}

// TestReadAllOrNotFound ensures missing-key reads are treated as empty cloud state.
func TestReadAllOrNotFound(t *testing.T) {
	t.Parallel()

	req := require.New(t)

	{
		data, ok, err := readAllOrNotFound(errReader{err: minio.ErrorResponse{Code: "NoSuchKey", Message: "The specified key does not exist."}})
		req.NoError(err)
		req.False(ok)
		req.Nil(data)
	}

	{
		data, ok, err := readAllOrNotFound(io.LimitReader(&zeroReader{}, 0))
		req.NoError(err)
		req.True(ok)
		req.Equal([]byte{}, data)
	}

	{
		boom := errors.New("boom")
		data, ok, err := readAllOrNotFound(errReader{err: boom})
		req.Error(err)
		req.False(ok)
		req.Nil(data)
	}
}

type zeroReader struct{}

// Read implements io.Reader and immediately returns EOF.
func (zeroReader) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

// TestSyncKeyFingerprint ensures fingerprints are stable and safe to log.
func TestSyncKeyFingerprint(t *testing.T) {
	t.Parallel()

	req := require.New(t)
	fp1 := syncKeyFingerprint("sync-example")
	fp2 := syncKeyFingerprint("sync-example")
	req.Equal(fp1, fp2)
	req.Len(fp1, 12)
}

// TestSyncKeyDerivedIdentifiersAreFrozen pins the S3 object key and the log
// fingerprint derived from a sync key, hex(SHA256(key)) and its 12-hex prefix,
// with vectors computed independently (Python hashlib).
//
// CodeQL go/weak-sensitive-data-hashing #79 (userConfigS3Key) and #80
// (syncKeyFingerprint) flag SHA-256 as password hashing. Neither is a password
// verifier: the digest only names the user's backup object (and tags logs with
// a 12-hex prefix), and the server never compares it to authenticate. The sync
// key is a bearer secret; current clients generate it from a CSPRNG (`sync-` +
// 32 base-36 chars, ~165 bits), older ones from Math.random, and the server
// accepts any non-empty key. The same key also feeds the fast HKDF that
// encrypts the backup, so a slow hash of the object name alone would not
// protect a weak key; it would only make every sync request expensive, and
// changing the derivation would orphan every existing cloud backup.
func TestSyncKeyDerivedIdentifiersAreFrozen(t *testing.T) {
	t.Parallel()

	const syncKey = "sync-0123456789abcdefghijklmnopqrstuv"
	require.Equal(t,
		"user-configs/3b31719986183dbbaa6787163d5ca5adbbb76880882430a70b4a3b8d0c4a5ae9",
		userConfigS3Key(syncKey))
	require.Equal(t, "3b3171998618", syncKeyFingerprint(syncKey))
}
