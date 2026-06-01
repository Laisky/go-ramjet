package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/minio/minio-go/v7"
)

func testLogger(t *testing.T) glog.Logger {
	t.Helper()
	l, err := glog.NewConsoleWithName("s3-versions-test", glog.LevelInfo)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return l
}

// fakeVerBase is the reference time for synthetic version timestamps.
var fakeVerBase = time.Unix(1_700_000_000, 0)

// fakeVersionClient is a minimal in-memory ObjectVersionClient for tests.
type fakeVersionClient struct {
	list     []minio.ObjectInfo
	listErr  error
	putErrs  []error // returned on successive PutObject calls
	putCalls int
	removed  []string // version ids passed to RemoveObject, in order
}

func (f *fakeVersionClient) PutObject(_ context.Context, _, _ string, r io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	_, _ = io.Copy(io.Discard, r)
	idx := f.putCalls
	f.putCalls++
	if idx < len(f.putErrs) && f.putErrs[idx] != nil {
		return minio.UploadInfo{}, f.putErrs[idx]
	}
	// Simulate the object store creating a new, newest version on success.
	f.list = append(f.list, minio.ObjectInfo{
		Key:          "k",
		VersionID:    fmt.Sprintf("put%d", f.putCalls),
		LastModified: fakeVerBase.Add(time.Duration(f.putCalls) * time.Second),
	})
	return minio.UploadInfo{}, nil
}

func (f *fakeVersionClient) ListObjects(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	ch := make(chan minio.ObjectInfo)
	go func() {
		defer close(ch)
		if f.listErr != nil {
			ch <- minio.ObjectInfo{Err: f.listErr}
			return
		}
		for _, o := range f.list {
			ch <- o
		}
	}()
	return ch
}

func (f *fakeVersionClient) RemoveObject(_ context.Context, _, _ string, opts minio.RemoveObjectOptions) error {
	f.removed = append(f.removed, opts.VersionID)
	kept := make([]minio.ObjectInfo, 0, len(f.list))
	for _, o := range f.list {
		if o.VersionID != opts.VersionID {
			kept = append(kept, o)
		}
	}
	f.list = kept
	return nil
}

// ver builds an ObjectInfo for key "k". A larger ageSeconds means older. All
// versions produced here predate fakeVerBase, so a version appended by a
// successful PutObject is always the newest.
func ver(id string, ageSeconds int, deleteMarker bool) minio.ObjectInfo {
	return minio.ObjectInfo{
		Key:            "k",
		VersionID:      id,
		LastModified:   fakeVerBase.Add(time.Duration(-ageSeconds) * time.Second),
		IsDeleteMarker: deleteMarker,
	}
}

// nonSeekableReader wraps an io.Reader so it does NOT satisfy io.Seeker.
type nonSeekableReader struct{ r io.Reader }

func (n nonSeekableReader) Read(p []byte) (int, error) { return n.r.Read(p) }

// bytesReader returns a seekable, re-readable reader (so retry-on-limit can rewind).
func bytesReader(s string) io.Reader { return bytes.NewReader([]byte(s)) }

func TestKeepLatestObjectVersions_TrimsExcess(t *testing.T) {
	f := &fakeVersionClient{list: []minio.ObjectInfo{
		ver("v1", 0, false),
		ver("v2", 10, false),
		ver("v3", 20, false),
		ver("v4", 30, false),
	}}

	if err := KeepLatestObjectVersions(context.Background(), testLogger(t), f, "b", "k", 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(f.removed) != 2 {
		t.Fatalf("expected 2 removals, got %v", f.removed)
	}
	want := map[string]bool{"v3": true, "v4": true}
	for _, id := range f.removed {
		if !want[id] {
			t.Errorf("unexpected removal %q", id)
		}
	}
}

func TestKeepLatestObjectVersions_NoopUnderKeep(t *testing.T) {
	f := &fakeVersionClient{list: []minio.ObjectInfo{ver("v1", 0, false), ver("v2", 10, false)}}

	if err := KeepLatestObjectVersions(context.Background(), testLogger(t), f, "b", "k", 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.removed) != 0 {
		t.Fatalf("expected no removals, got %v", f.removed)
	}
}

func TestKeepLatestObjectVersions_KeepZeroNoop(t *testing.T) {
	f := &fakeVersionClient{list: []minio.ObjectInfo{ver("v1", 0, false), ver("v2", 10, false)}}

	if err := KeepLatestObjectVersions(context.Background(), testLogger(t), f, "b", "k", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.removed) != 0 {
		t.Fatalf("expected no removals for keep=0, got %v", f.removed)
	}
}

func TestKeepLatestObjectVersions_SkipsMarkersAndEmpty(t *testing.T) {
	f := &fakeVersionClient{list: []minio.ObjectInfo{
		ver("v1", 0, false),
		ver("", 5, false),  // empty version id: must be ignored
		ver("dm", 8, true), // delete marker: must be ignored
		ver("v2", 10, false),
		ver("v3", 20, false),
	}}

	if err := KeepLatestObjectVersions(context.Background(), testLogger(t), f, "b", "k", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, id := range f.removed {
		if id == "" || id == "dm" {
			t.Errorf("must not remove marker/empty version: %q", id)
		}
	}
	if len(f.removed) != 2 {
		t.Fatalf("expected 2 removals, got %v", f.removed)
	}
}

func TestKeepLatestObjectVersions_ListErrorPropagates(t *testing.T) {
	f := &fakeVersionClient{listErr: fmt.Errorf("boom")}

	if err := KeepLatestObjectVersions(context.Background(), testLogger(t), f, "b", "k", 2); err == nil {
		t.Fatal("expected an error when listing versions fails")
	}
	if len(f.removed) != 0 {
		t.Fatalf("must not remove anything when listing fails, got %v", f.removed)
	}
}

func TestIsVersionLimitError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"code", minio.ErrorResponse{Code: "MaxVersionsExceeded"}, true},
		{"message", minio.ErrorResponse{Message: "You've exceeded the limit on the number of versions you can create on this object"}, true},
		{"other", minio.ErrorResponse{Code: "NoSuchKey", Message: "does not exist"}, false},
	}
	for _, c := range cases {
		if got := IsVersionLimitError(c.err); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestPutObjectCappingVersions_SuccessfulUpload(t *testing.T) {
	f := &fakeVersionClient{list: []minio.ObjectInfo{
		ver("v1", 0, false), ver("v2", 10, false), ver("v3", 20, false),
	}}

	if _, err := PutObjectCappingVersions(context.Background(), testLogger(t), f, "b", "k", bytesReader("payload"), 7, minio.PutObjectOptions{}, 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.putCalls != 1 {
		t.Fatalf("expected exactly 1 put call, got %d", f.putCalls)
	}
	// Pre-trim to keep-1 (=1) before upload, then upload adds 1 => exactly 2 retained.
	if len(f.list) != 2 {
		t.Fatalf("expected final retained versions == 2, got %d (%v)", len(f.list), f.list)
	}
}

func TestPutObjectCappingVersions_RetriesOnVersionLimit(t *testing.T) {
	limitErr := minio.ErrorResponse{
		Code:    "MaxVersionsExceeded",
		Message: "you've exceeded the limit on the number of versions you can create on this object",
	}
	f := &fakeVersionClient{
		list:    []minio.ObjectInfo{ver("v1", 0, false), ver("v2", 10, false), ver("v3", 20, false)},
		putErrs: []error{limitErr, nil}, // first upload fails, retry succeeds
	}

	// bytesReader returns an io.Seeker (SectionReader), so retry is possible.
	_, err := PutObjectCappingVersions(context.Background(), testLogger(t), f, "b", "k", bytesReader("payload"), 7, minio.PutObjectOptions{}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.putCalls != 2 {
		t.Fatalf("expected 2 put calls (1 fail + 1 retry), got %d", f.putCalls)
	}
	if len(f.removed) == 0 {
		t.Fatalf("expected old versions to be trimmed")
	}
	if len(f.list) != 2 {
		t.Fatalf("expected final retained versions == 2 after retry, got %d (%v)", len(f.list), f.list)
	}
}

func TestPutObjectCappingVersions_NonSeekableNoRetry(t *testing.T) {
	limitErr := minio.ErrorResponse{Code: "MaxVersionsExceeded"}
	f := &fakeVersionClient{
		list:    []minio.ObjectInfo{ver("v1", 0, false), ver("v2", 10, false)},
		putErrs: []error{limitErr}, // only one failure queued; a retry would consume a 2nd (nil) call
	}

	reader := nonSeekableReader{r: bytesReader("payload")}
	_, err := PutObjectCappingVersions(context.Background(), testLogger(t), f, "b", "k", reader, 7, minio.PutObjectOptions{}, 2)
	if err == nil {
		t.Fatal("expected error to propagate when a non-seekable upload hits the version limit")
	}
	if f.putCalls != 1 {
		t.Fatalf("expected exactly 1 put call (no retry for non-seekable reader), got %d", f.putCalls)
	}
}

func TestPutObjectCappingVersions_KeepZeroSkipsTrim(t *testing.T) {
	f := &fakeVersionClient{list: []minio.ObjectInfo{
		ver("v1", 0, false), ver("v2", 10, false), ver("v3", 20, false),
	}}

	if _, err := PutObjectCappingVersions(context.Background(), testLogger(t), f, "b", "k", bytesReader("payload"), 7, minio.PutObjectOptions{}, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.putCalls != 1 {
		t.Fatalf("expected exactly 1 put call, got %d", f.putCalls)
	}
	if len(f.removed) != 0 {
		t.Fatalf("keep=0 must not trim any versions, got removals %v", f.removed)
	}
}
