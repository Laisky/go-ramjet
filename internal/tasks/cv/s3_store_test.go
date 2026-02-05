package cv

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
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
	mu            sync.Mutex
	objects       map[string]fakeS3Object
	putObjectOpts map[string]minio.PutObjectOptions
}

// newFakeS3Client creates an in-memory S3 client for tests.
func newFakeS3Client() *fakeS3Client {
	return &fakeS3Client{
		objects:       make(map[string]fakeS3Object),
		putObjectOpts: make(map[string]minio.PutObjectOptions),
	}
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

// PutObject stores an object in memory and captures the put options.
func (f *fakeS3Client) PutObject(_ context.Context, _ string, objectName string, reader io.Reader, _ int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
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
	f.putObjectOpts[objectName] = opts

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

// RemoveObject deletes an object from the in-memory store.
func (f *fakeS3Client) RemoveObject(_ context.Context, _ string, objectName string, _ minio.RemoveObjectOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.objects, objectName)
	delete(f.putObjectOpts, objectName)
	return nil
}

// ListObjects streams object info for keys matching the prefix.
func (f *fakeS3Client) ListObjects(_ context.Context, _ string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	ch := make(chan minio.ObjectInfo, len(f.objects))
	f.mu.Lock()
	for key, obj := range f.objects {
		if opts.Prefix != "" && key != opts.Prefix && !strings.HasPrefix(key, opts.Prefix) {
			continue
		}
		ch <- minio.ObjectInfo{
			Key:          obj.objectName,
			Size:         int64(len(obj.content)),
			LastModified: obj.updatedAt,
		}
	}
	f.mu.Unlock()
	close(ch)
	return ch
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

// TestS3PDFStoreSaveAndOpen verifies PDF payloads are stored and retrieved; it takes a testing.T and returns no values.
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

// TestS3PDFStoreSaveSetsCacheControl ensures PDF uploads include cache-control metadata; it takes a testing.T and returns no values.
func TestS3PDFStoreSaveSetsCacheControl(t *testing.T) {
	t.Parallel()

	client := newFakeS3Client()
	store, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)

	err = store.Save(context.Background(), []byte("%PDF-1.4 mock"))
	require.NoError(t, err)

	client.mu.Lock()
	opts, ok := client.putObjectOpts["cv.pdf"]
	client.mu.Unlock()

	require.True(t, ok)
	require.Equal(t, cvPDFCacheControl, opts.CacheControl)
	require.Equal(t, "application/pdf", opts.ContentType)
}

type versionLimitS3Client struct {
	putCalls          int
	removeCalls       int
	listCalls         int
	versions          []minio.ObjectInfo
	removedVersionIDs []string
}

// GetObject is unused in this test and returns a not found error.
func (v *versionLimitS3Client) GetObject(_ context.Context, _ string, _ string, _ minio.GetObjectOptions) (io.ReadCloser, error) {
	return nil, minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404}
}

// PutObject returns a version limit error on the first call and succeeds afterwards.
func (v *versionLimitS3Client) PutObject(_ context.Context, _ string, _ string, reader io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	v.putCalls++
	if v.putCalls == 1 {
		return minio.UploadInfo{}, errors.New("You've exceeded the limit on the number of versions you can create on this object")
	}
	_, err := io.ReadAll(reader)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	return minio.UploadInfo{Key: "cv.pdf"}, nil
}

// StatObject returns a not found error for this test client.
func (v *versionLimitS3Client) StatObject(_ context.Context, _ string, _ string, _ minio.StatObjectOptions) (minio.ObjectInfo, error) {
	return minio.ObjectInfo{}, minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404}
}

// RemoveObject counts removals and returns nil.
func (v *versionLimitS3Client) RemoveObject(_ context.Context, _ string, _ string, opts minio.RemoveObjectOptions) error {
	v.removeCalls++
	v.removedVersionIDs = append(v.removedVersionIDs, opts.VersionID)
	return nil
}

// ListObjects streams configured versions and counts the call.
func (v *versionLimitS3Client) ListObjects(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	v.listCalls++
	ch := make(chan minio.ObjectInfo, len(v.versions))
	for _, obj := range v.versions {
		ch <- obj
	}
	close(ch)
	return ch
}

// TestS3PDFStoreSaveRemovesVersionsOnLimit ensures version cleanup happens when S3 rejects new versions.
// It takes a testing.T and returns no values.
func TestS3PDFStoreSaveRemovesVersionsOnLimit(t *testing.T) {
	t.Parallel()

	client := &versionLimitS3Client{
		versions: []minio.ObjectInfo{
			{Key: "cv.pdf", VersionID: "v1"},
			{Key: "cv.pdf", VersionID: "v2", IsLatest: true},
		},
	}
	store, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)

	err = store.Save(context.Background(), []byte("%PDF-1.4 mock"))
	require.NoError(t, err)
	require.Equal(t, 2, client.putCalls)
	require.Equal(t, 2, client.listCalls)
	require.Equal(t, 3, client.removeCalls)
}

// TestS3ContentStoreSaveRemovesVersionsOnLimit ensures content cleanup happens when S3 rejects new versions.
// It takes a testing.T and returns no values.
func TestS3ContentStoreSaveRemovesVersionsOnLimit(t *testing.T) {
	t.Parallel()

	client := &versionLimitS3Client{
		versions: []minio.ObjectInfo{
			{Key: "cv.md", VersionID: "v1"},
			{Key: "cv.md", VersionID: "v2", IsLatest: true},
			{Key: "other.md", VersionID: "v3"},
		},
	}
	store, err := NewS3ContentStore(client, "bucket", "cv.md", "default")
	require.NoError(t, err)

	payload, err := store.Save(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, "hello", payload.Content)
	require.NotNil(t, payload.UpdatedAt)
	require.Equal(t, 2, client.putCalls)
	require.Equal(t, 2, client.listCalls)
	require.Equal(t, 3, client.removeCalls)
}

type precleanS3Client struct {
	putCalls          int
	removeCalls       int
	listCalls         int
	versions          []minio.ObjectInfo
	removedVersionIDs []string
}

// GetObject is unused in this test and returns a not found error.
func (p *precleanS3Client) GetObject(_ context.Context, _ string, _ string, _ minio.GetObjectOptions) (io.ReadCloser, error) {
	return nil, minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404}
}

// PutObject succeeds and counts calls.
func (p *precleanS3Client) PutObject(_ context.Context, _ string, _ string, reader io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	p.putCalls++
	_, err := io.ReadAll(reader)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	return minio.UploadInfo{Key: "cv.pdf"}, nil
}

// StatObject returns a not found error for this test client.
func (p *precleanS3Client) StatObject(_ context.Context, _ string, _ string, _ minio.StatObjectOptions) (minio.ObjectInfo, error) {
	return minio.ObjectInfo{}, minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404}
}

// RemoveObject counts removals and records version IDs.
func (p *precleanS3Client) RemoveObject(_ context.Context, _ string, _ string, opts minio.RemoveObjectOptions) error {
	p.removeCalls++
	p.removedVersionIDs = append(p.removedVersionIDs, opts.VersionID)
	return nil
}

// ListObjects streams configured versions and counts the call.
func (p *precleanS3Client) ListObjects(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	p.listCalls++
	ch := make(chan minio.ObjectInfo, len(p.versions))
	for _, obj := range p.versions {
		ch <- obj
	}
	close(ch)
	return ch
}

// TestS3ContentStorePrecleanNonCurrentVersions verifies non-current content versions are removed before upload.
// It takes a testing.T and returns no values.
func TestS3ContentStorePrecleanNonCurrentVersions(t *testing.T) {
	t.Parallel()

	client := &precleanS3Client{
		versions: []minio.ObjectInfo{
			{Key: "cv.md", VersionID: "v1"},
			{Key: "cv.md", VersionID: "v2", IsLatest: true},
			{Key: "other.md", VersionID: "v3"},
		},
	}
	store, err := NewS3ContentStore(client, "bucket", "cv.md", "default")
	require.NoError(t, err)

	_, err = store.Save(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, 1, client.putCalls)
	require.Equal(t, 1, client.listCalls)
	require.Equal(t, 1, client.removeCalls)
	require.Equal(t, []string{"v1"}, client.removedVersionIDs)
}

// TestS3PDFStorePrecleanNonCurrentVersions verifies non-current pdf versions are removed before upload.
// It takes a testing.T and returns no values.
func TestS3PDFStorePrecleanNonCurrentVersions(t *testing.T) {
	t.Parallel()

	client := &precleanS3Client{
		versions: []minio.ObjectInfo{
			{Key: "cv.pdf", VersionID: "v1"},
			{Key: "cv.pdf", VersionID: "v2", IsLatest: true},
			{Key: "other.pdf", VersionID: "v3"},
		},
	}
	store, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)

	err = store.Save(context.Background(), []byte("%PDF-1.4 mock"))
	require.NoError(t, err)
	require.Equal(t, 1, client.putCalls)
	require.Equal(t, 1, client.listCalls)
	require.Equal(t, 1, client.removeCalls)
	require.Equal(t, []string{"v1"}, client.removedVersionIDs)
}
