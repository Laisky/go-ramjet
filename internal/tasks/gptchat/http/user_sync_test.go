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
