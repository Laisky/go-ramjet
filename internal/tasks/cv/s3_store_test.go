package cv

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

type fakeS3Object struct {
	content    []byte
	updatedAt  time.Time
	objectName string
}

type fakeS3Client struct {
	mu      sync.Mutex
	objects map[string]fakeS3Object
}

// newFakeS3Client creates an in-memory S3 client for tests.
func newFakeS3Client() *fakeS3Client {
	return &fakeS3Client{objects: make(map[string]fakeS3Object)}
}

// GetObject returns a reader for an object stored in memory.
func (f *fakeS3Client) GetObject(_ context.Context, _ string, objectName string, _ minio.GetObjectOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	obj, ok := f.objects[objectName]
	if !ok {
		return nil, minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404}
	}
	return io.NopCloser(bytes.NewReader(obj.content)), nil
}

// PutObject stores an object in memory.
func (f *fakeS3Client) PutObject(_ context.Context, _ string, objectName string, reader io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	payload, err := io.ReadAll(reader)
	if err != nil {
		return minio.UploadInfo{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.objects[objectName] = fakeS3Object{
		content:    payload,
		updatedAt:  time.Now().UTC(),
		objectName: objectName,
	}

	return minio.UploadInfo{Key: objectName}, nil
}

// StatObject returns metadata for an in-memory object.
func (f *fakeS3Client) StatObject(_ context.Context, _ string, objectName string, _ minio.StatObjectOptions) (minio.ObjectInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	obj, ok := f.objects[objectName]
	if !ok {
		return minio.ObjectInfo{}, minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404}
	}

	return minio.ObjectInfo{
		Key:          obj.objectName,
		Size:         int64(len(obj.content)),
		LastModified: obj.updatedAt,
	}, nil
}

// TestS3ContentStoreLoadDefault verifies default content is returned when the object is missing.
func TestS3ContentStoreLoadDefault(t *testing.T) {
	t.Parallel()

	client := newFakeS3Client()
	store, err := NewS3ContentStore(client, "bucket", "cv.md", "default")
	require.NoError(t, err)

	payload, err := store.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, "default", payload.Content)
	require.True(t, payload.IsDefault)
	require.Nil(t, payload.UpdatedAt)
}

// TestS3ContentStoreSaveAndLoad verifies S3 content can be saved and read back.
func TestS3ContentStoreSaveAndLoad(t *testing.T) {
	t.Parallel()

	client := newFakeS3Client()
	store, err := NewS3ContentStore(client, "bucket", "cv.md", "default")
	require.NoError(t, err)

	saved, err := store.Save(context.Background(), "hello")
	require.NoError(t, err)
	require.False(t, saved.IsDefault)
	require.NotNil(t, saved.UpdatedAt)

	loaded, err := store.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, "hello", loaded.Content)
	require.False(t, loaded.IsDefault)
	require.NotNil(t, loaded.UpdatedAt)
	require.WithinDuration(t, *saved.UpdatedAt, *loaded.UpdatedAt, time.Second)
}

// TestCompositeContentStoreFallback verifies fallback to secondary repository when primary returns default content.
func TestCompositeContentStoreFallback(t *testing.T) {
	t.Parallel()

	primaryClient := newFakeS3Client()
	primaryStore, err := NewS3ContentStore(primaryClient, "bucket", "cv.md", "default")
	require.NoError(t, err)

	secondaryClient := newFakeS3Client()
	secondaryStore, err := NewS3ContentStore(secondaryClient, "bucket", "cv.md", "default")
	require.NoError(t, err)

	_, err = secondaryStore.Save(context.Background(), "secondary content")
	require.NoError(t, err)

	composite, err := NewCompositeContentStore(primaryStore, secondaryStore)
	require.NoError(t, err)

	payload, err := composite.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, "secondary content", payload.Content)
	require.False(t, payload.IsDefault)
}

// TestS3PDFStoreSaveAndOpen verifies PDF payloads are stored and retrieved.
func TestS3PDFStoreSaveAndOpen(t *testing.T) {
	t.Parallel()

	client := newFakeS3Client()
	store, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)

	payload := []byte("%PDF-1.4 mock")
	err = store.Save(context.Background(), payload)
	require.NoError(t, err)

	reader, size, err := store.Open(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(len(payload)), size)

	loaded, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, payload, loaded)
	require.NoError(t, reader.Close())
}
