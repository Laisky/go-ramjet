package blog

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/library/log"
)

type fakeS3Client struct {
	mu                   sync.Mutex
	key                  string
	versions             []minio.ObjectInfo
	maxVersions          int
	putCalls             int
	failVersionLimitOnce bool
}

func newFakeS3Client(key string, maxVersions int) *fakeS3Client {
	return &fakeS3Client{key: key, maxVersions: maxVersions}
}

func (f *fakeS3Client) seedVersions(count int) *fakeS3Client {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().Add(-time.Duration(count) * time.Hour)
	f.versions = make([]minio.ObjectInfo, 0, count)
	for i := 0; i < count; i++ {
		f.versions = append(f.versions, minio.ObjectInfo{
			Key:          f.key,
			VersionID:    fmt.Sprintf("seed-%d", i),
			LastModified: now.Add(time.Duration(i) * time.Minute),
		})
	}
	return f
}

func (f *fakeS3Client) PutObject(_ context.Context, bucketName, objectName string, _ io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putCalls++
	if f.failVersionLimitOnce {
		f.failVersionLimitOnce = false
		return minio.UploadInfo{}, minio.ErrorResponse{
			Code:    "MaxVersionsExceeded",
			Message: "You've exceeded the limit on the number of versions you can create on this object",
		}
	}
	if f.maxVersions > 0 && len(f.versions) >= f.maxVersions {
		return minio.UploadInfo{}, minio.ErrorResponse{
			Code:    "MaxVersionsExceeded",
			Message: "You've exceeded the limit on the number of versions you can create on this object",
		}
	}
	versionID := fmt.Sprintf("put-%d", f.putCalls)
	f.versions = append([]minio.ObjectInfo{{
		Key:          f.key,
		VersionID:    versionID,
		LastModified: time.Now(),
	}}, f.versions...)
	return minio.UploadInfo{VersionID: versionID}, nil
}

func (f *fakeS3Client) ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	f.mu.Lock()
	snapshot := make([]minio.ObjectInfo, len(f.versions))
	copy(snapshot, f.versions)
	f.mu.Unlock()

	ch := make(chan minio.ObjectInfo, len(snapshot))
	go func() {
		defer close(ch)
		for _, v := range snapshot {
			select {
			case ch <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

func (f *fakeS3Client) RemoveObject(_ context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for idx, v := range f.versions {
		if v.VersionID == opts.VersionID {
			f.versions = append(f.versions[:idx], f.versions[idx+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeS3Client) versionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.versions)
}

func (f *fakeS3Client) versionIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, len(f.versions))
	for i, v := range f.versions {
		ids[i] = v.VersionID
	}
	return ids
}

func TestKeepLatestS3ObjectVersions(t *testing.T) {
	ctx := context.Background()
	cli := newFakeS3Client("rss.xml", 0).seedVersions(5)
	logger := log.Logger.Named("test-keep-latest")

	if err := keepLatestS3ObjectVersions(ctx, logger, cli, "bucket", "rss.xml", 2); err != nil {
		t.Fatalf("keepLatestS3ObjectVersions returned error: %v", err)
	}

	if got := cli.versionCount(); got != 2 {
		t.Fatalf("expected 2 versions left, got %d", got)
	}
}

func TestPersistRSSObjectToS3PreTrim(t *testing.T) {
	ctx := context.Background()
	cli := newFakeS3Client("rss.xml", 3).seedVersions(3)
	worker := &RssWorker{logger: log.Logger}
	logger := worker.logger.Named("test-pre-trim")

	if err := worker.persistRSSObjectToS3(ctx, logger, cli, "bucket", "rss.xml", "<rss />", 3); err != nil {
		t.Fatalf("persistRSSObjectToS3 returned error: %v", err)
	}

	if got := cli.versionCount(); got != 3 {
		t.Fatalf("expected 3 versions retained, got %d", got)
	}
	if cli.putCalls != 1 {
		t.Fatalf("expected a single upload attempt but got %d", cli.putCalls)
	}
}

func TestPersistRSSObjectToS3RetriesOnLimit(t *testing.T) {
	ctx := context.Background()
	cli := newFakeS3Client("rss.xml", 10).seedVersions(1)
	cli.failVersionLimitOnce = true
	worker := &RssWorker{logger: log.Logger}
	logger := worker.logger.Named("test-retry")

	if err := worker.persistRSSObjectToS3(ctx, logger, cli, "bucket", "rss.xml", "<rss />", 3); err != nil {
		t.Fatalf("persistRSSObjectToS3 returned error: %v", err)
	}

	if cli.putCalls != 2 {
		t.Fatalf("expected retry to perform two upload attempts, got %d", cli.putCalls)
	}
	if got := cli.versionCount(); got > 3 {
		t.Fatalf("expected at most 3 versions retained, got %d", got)
	}
}
