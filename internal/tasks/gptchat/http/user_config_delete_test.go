package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

// fakeUserConfigStore is an in-memory s3lib.ObjectVersionClient that tracks
// object versions per key for user config deletion tests.
type fakeUserConfigStore struct {
	mu          sync.Mutex
	objects     []minio.ObjectInfo
	listErr     error
	removeErr   error
	removeCalls []string // "key@version" for every RemoveObject call, in order
}

// PutObject drains reader and records a new version of objectName.
func (f *fakeUserConfigStore) PutObject(_ context.Context, _, objectName string, reader io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return minio.UploadInfo{}, errors.Wrap(err, "drain reader")
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects = append(f.objects, minio.ObjectInfo{
		Key:       objectName,
		VersionID: fmt.Sprintf("put-%d", len(f.objects)),
	})
	return minio.UploadInfo{Key: objectName}, nil
}

// ListObjects streams every stored entry whose key starts with opts.Prefix, or
// a single error entry when listErr is set.
func (f *fakeUserConfigStore) ListObjects(_ context.Context, _ string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	f.mu.Lock()
	snapshot := append([]minio.ObjectInfo(nil), f.objects...)
	listErr := f.listErr
	f.mu.Unlock()

	ch := make(chan minio.ObjectInfo, len(snapshot)+1)
	if listErr != nil {
		ch <- minio.ObjectInfo{Err: listErr}
	} else {
		for _, object := range snapshot {
			if strings.HasPrefix(object.Key, opts.Prefix) {
				ch <- object
			}
		}
	}
	close(ch)
	return ch
}

// RemoveObject deletes the entry matching objectName and opts.VersionID, or
// returns removeErr when it is set.
func (f *fakeUserConfigStore) RemoveObject(_ context.Context, _, objectName string, opts minio.RemoveObjectOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeCalls = append(f.removeCalls, objectName+"@"+opts.VersionID)
	if f.removeErr != nil {
		return f.removeErr
	}

	kept := f.objects[:0]
	for _, object := range f.objects {
		if object.Key != objectName || object.VersionID != opts.VersionID {
			kept = append(kept, object)
		}
	}
	f.objects = kept
	return nil
}

// TestDeleteUserConfigObjectsRemovesAllVersions verifies every version and
// delete marker of the caller's backup is removed, another user's backup is
// untouched, a repeated delete is an idempotent no-op, and the raw sync key
// never reaches the logs.
func TestDeleteUserConfigObjectsRemovesAllVersions(t *testing.T) {
	const (
		syncKey  = "sync-delete-me-0123456789"
		otherKey = "sync-someone-else-9876543210"
	)
	ownKey := userConfigS3Key(syncKey)
	otherObject := minio.ObjectInfo{Key: userConfigS3Key(otherKey), VersionID: "o1"}
	store := &fakeUserConfigStore{objects: []minio.ObjectInfo{
		{Key: ownKey, VersionID: "v1"},
		{Key: ownKey, VersionID: "v2"},
		{Key: ownKey, VersionID: "dm1", IsDeleteMarker: true},
		otherObject,
	}}
	logger, entries := embeddingTestLogger(t)

	deleted, err := deleteUserConfigObjects(context.Background(), logger, store, "bucket", syncKey)
	require.NoError(t, err)
	require.Equal(t, 3, deleted)
	require.ElementsMatch(t, []string{ownKey + "@v1", ownKey + "@v2", ownKey + "@dm1"}, store.removeCalls)
	require.Equal(t, []minio.ObjectInfo{otherObject}, store.objects)

	deleted, err = deleteUserConfigObjects(context.Background(), logger, store, "bucket", syncKey)
	require.NoError(t, err)
	require.Zero(t, deleted, "deleting an absent backup must succeed with 0")
	require.Len(t, store.removeCalls, 3)

	for _, entry := range entries.All() {
		require.NotContains(t, entry.Message, syncKey)
		for field, value := range entry.ContextMap() {
			require.NotContains(t, fmt.Sprint(value), syncKey, "log field %q leaked the sync key", field)
		}
	}
}

// TestDeleteUserConfigObjectsUnversionedObject verifies the live object of an
// unversioned bucket is removed with an empty version id.
func TestDeleteUserConfigObjectsUnversionedObject(t *testing.T) {
	const syncKey = "sync-unversioned-0123456789"
	ownKey := userConfigS3Key(syncKey)
	store := &fakeUserConfigStore{objects: []minio.ObjectInfo{{Key: ownKey}}}
	logger, _ := embeddingTestLogger(t)

	deleted, err := deleteUserConfigObjects(context.Background(), logger, store, "bucket", syncKey)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)
	require.Equal(t, []string{ownKey + "@"}, store.removeCalls)
	require.Empty(t, store.objects)
}

// TestDeleteUserConfigObjectsErrors verifies an empty sync key is rejected
// before touching storage and storage failures are propagated without
// exposing the sync key.
func TestDeleteUserConfigObjectsErrors(t *testing.T) {
	const syncKey = "sync-errors-0123456789"
	logger, _ := embeddingTestLogger(t)

	{
		store := &fakeUserConfigStore{}
		deleted, err := deleteUserConfigObjects(context.Background(), logger, store, "bucket", "")
		require.Error(t, err)
		require.Zero(t, deleted)
		require.Empty(t, store.removeCalls)
	}

	{
		boom := errors.New("list boom")
		store := &fakeUserConfigStore{listErr: boom}
		deleted, err := deleteUserConfigObjects(context.Background(), logger, store, "bucket", syncKey)
		require.ErrorIs(t, err, boom)
		require.NotContains(t, err.Error(), syncKey)
		require.Zero(t, deleted)
		require.Empty(t, store.removeCalls)
	}

	{
		boom := errors.New("remove boom")
		store := &fakeUserConfigStore{
			objects:   []minio.ObjectInfo{{Key: userConfigS3Key(syncKey), VersionID: "v1"}},
			removeErr: boom,
		}
		deleted, err := deleteUserConfigObjects(context.Background(), logger, store, "bucket", syncKey)
		require.ErrorIs(t, err, boom)
		require.NotContains(t, err.Error(), syncKey)
		require.Zero(t, deleted)
		require.Len(t, store.removeCalls, 1)
	}
}

// TestDeleteUserConfigRejectsMissingSyncKey verifies the handler rejects a
// missing or blank sync key header before reaching object storage.
func TestDeleteUserConfigRejectsMissingSyncKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, header := range []string{"", "   "} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodDelete, "/gptchat/user/config", nil)
		if header != "" {
			ctx.Request.Header.Set("X-LAISKY-SYNC-KEY", header)
		}

		DeleteUserConfig(ctx)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Contains(t, recorder.Body.String(), "empty apikey")
		require.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
	}
}
