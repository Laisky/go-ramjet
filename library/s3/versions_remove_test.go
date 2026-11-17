package s3

import (
	"context"
	"errors"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

// keyed returns a copy of info whose key is replaced by key.
func keyed(key string, info minio.ObjectInfo) minio.ObjectInfo {
	info.Key = key
	return info
}

// TestRemoveAllObjectVersions_VersionedRemovesEverything verifies every version
// and delete marker of the exact key is removed while a prefix sibling survives.
func TestRemoveAllObjectVersions_VersionedRemovesEverything(t *testing.T) {
	sibling := keyed("k-sibling", ver("s1", 0, false))
	f := &fakeVersionClient{list: []minio.ObjectInfo{
		ver("v1", 0, false),
		ver("dm", 5, true),
		ver("v2", 10, false),
		sibling,
		ver("v3", 20, false),
	}}

	n, err := RemoveAllObjectVersions(context.Background(), testLogger(t), f, "b", "k")
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.ElementsMatch(t, []string{"v1", "dm", "v2", "v3"}, f.removed)
	require.Equal(t, []minio.ObjectInfo{sibling}, f.list, "sibling key sharing the prefix must be untouched")
}

// TestRemoveAllObjectVersions_UnversionedLiveObject verifies the live object of
// an unversioned bucket is removed with an empty version id.
func TestRemoveAllObjectVersions_UnversionedLiveObject(t *testing.T) {
	sibling := keyed("k/child", ver("", 0, false))
	f := &fakeVersionClient{list: []minio.ObjectInfo{ver("", 0, false), sibling}}

	n, err := RemoveAllObjectVersions(context.Background(), testLogger(t), f, "b", "k")
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, []string{""}, f.removed)
	require.Equal(t, []minio.ObjectInfo{sibling}, f.list, "sibling key sharing the prefix must be untouched")
}

// TestRemoveAllObjectVersions_NothingToRemove verifies a missing object is a
// successful no-op.
func TestRemoveAllObjectVersions_NothingToRemove(t *testing.T) {
	f := &fakeVersionClient{list: []minio.ObjectInfo{keyed("k2", ver("x1", 0, false))}}

	n, err := RemoveAllObjectVersions(context.Background(), testLogger(t), f, "b", "k")
	require.NoError(t, err)
	require.Zero(t, n)
	require.Empty(t, f.removed)
	require.Zero(t, f.removeCalls)

	empty := &fakeVersionClient{}
	n, err = RemoveAllObjectVersions(context.Background(), testLogger(t), empty, "b", "k")
	require.NoError(t, err)
	require.Zero(t, n)
	require.Zero(t, empty.removeCalls)
}

// TestRemoveAllObjectVersions_ListErrorPropagates verifies a listing failure is
// returned and nothing is removed.
func TestRemoveAllObjectVersions_ListErrorPropagates(t *testing.T) {
	boom := errors.New("list boom")
	f := &fakeVersionClient{listErr: boom}

	n, err := RemoveAllObjectVersions(context.Background(), testLogger(t), f, "b", "k")
	require.ErrorIs(t, err, boom)
	require.Contains(t, err.Error(), "list object versions")
	require.Zero(t, n)
	require.Zero(t, f.removeCalls)
}

// TestRemoveAllObjectVersions_RemoveErrorPropagates verifies a removal failure is
// returned together with the number of versions removed before it.
func TestRemoveAllObjectVersions_RemoveErrorPropagates(t *testing.T) {
	boom := errors.New("remove boom")
	f := &fakeVersionClient{
		list: []minio.ObjectInfo{
			ver("v1", 0, false),
			ver("v2", 10, false),
			ver("v3", 20, false),
		},
		removeErrAt: 2,
		removeErr:   boom,
	}

	n, err := RemoveAllObjectVersions(context.Background(), testLogger(t), f, "b", "k")
	require.ErrorIs(t, err, boom)
	require.Contains(t, err.Error(), "remove object version")
	require.Equal(t, 1, n)
	require.Equal(t, []string{"v1"}, f.removed)
	require.Equal(t, 2, f.removeCalls, "removal must stop at the first failure")
}
