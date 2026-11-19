package postgres

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/s3"
)

// pipelineTestGuard bounds how long a pipeline test waits before declaring a hang.
const pipelineTestGuard = 10 * time.Second

// fakeUploadClient is an in-memory s3.ObjectVersionClient for streaming upload tests.
// PutObject consumes at most readLimit bytes (a negative limit drains to EOF) and then
// returns putErr, mimicking an upload that fails or stops reading part-way through.
// Consumed bytes are only counted unless keep is set, so endless producers cannot
// exhaust memory.
type fakeUploadClient struct {
	readLimit int64
	putErr    error
	keep      bool
	consumed  int64
	received  bytes.Buffer
}

// PutObject reads from r according to readLimit and returns the configured putErr.
func (f *fakeUploadClient) PutObject(_ context.Context, _, _ string, r io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	src := r
	if f.readLimit >= 0 {
		src = io.LimitReader(r, f.readLimit)
	}

	dst := io.Discard
	if f.keep {
		dst = &f.received
	}

	n, err := io.Copy(dst, src)
	f.consumed += n
	if err != nil {
		return minio.UploadInfo{}, errors.Wrap(err, "fake upload read")
	}
	if f.putErr != nil {
		return minio.UploadInfo{}, errors.Wrap(f.putErr, "fake upload")
	}

	return minio.UploadInfo{Size: n}, nil
}

// ListObjects returns an empty, closed listing so version trimming is a no-op.
func (f *fakeUploadClient) ListObjects(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	ch := make(chan minio.ObjectInfo)
	close(ch)
	return ch
}

// RemoveObject accepts every removal without doing anything.
func (f *fakeUploadClient) RemoveObject(_ context.Context, _, _ string, _ minio.RemoveObjectOptions) error {
	return nil
}

// stubStreamPipeline replaces the S3 client and the dump producer used by
// streamBackupToS3 for the duration of the test.
func stubStreamPipeline(t *testing.T, cli s3.ObjectVersionClient, dump func(context.Context, cfgDB, io.Writer) error) {
	t.Helper()
	origCli, origDump := newBackupS3Client, dumpDatabase
	t.Cleanup(func() { newBackupS3Client, dumpDatabase = origCli, origDump })

	newBackupS3Client = func(cfgS3) (s3.ObjectVersionClient, error) { return cli, nil }
	dumpDatabase = dump
}

// endlessDump writes dump chunks into w until a write fails, like pg_dump on a large
// database. It deliberately ignores ctx because a blocked pipe write cannot observe it.
func endlessDump(_ context.Context, _ cfgDB, w io.Writer) error {
	chunk := bytes.Repeat([]byte("row"), 16*1024)
	for {
		if _, err := w.Write(chunk); err != nil {
			return errors.Wrap(err, "write dump chunk")
		}
	}
}

// runWithGuard runs fn in a goroutine and fails the test if it does not return
// within pipelineTestGuard, which is how a pipe deadlock manifests.
func runWithGuard(t *testing.T, what string, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()

	select {
	case err := <-done:
		return err
	case <-time.After(pipelineTestGuard):
		require.FailNowf(t, "pipeline hang", "%s did not return within %s", what, pipelineTestGuard)
		return nil
	}
}

// TestStreamBackupToS3UploadStopsReadingDoesNotHang verifies that when the streaming
// upload returns before draining the pipe, the dump producer is unblocked and
// streamBackupToS3 reports a failure instead of blocking forever.
func TestStreamBackupToS3UploadStopsReadingDoesNotHang(t *testing.T) {
	cases := []struct {
		name    string
		putErr  error
		wantMsg string
	}{
		{name: "upload fails part-way", putErr: errors.New("upload rejected"), wantMsg: "upload rejected"},
		{name: "upload returns success without draining", putErr: nil, wantMsg: "pg_dump pipeline"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli := &fakeUploadClient{readLimit: 1024, putErr: tc.putErr}
			stubStreamPipeline(t, cli, endlessDump)

			err := runWithGuard(t, "streamBackupToS3", func() error {
				return streamBackupToS3(context.Background(), cfgDB{Database: "appdb"}, cfgS3{Bucket: "bucket"}, "appdb-20261119.gz")
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

// TestStreamBackupToS3UploadFailureStopsIdleDump verifies that a failed upload also
// stops a dump that is not writing (pg_dump waiting on a lock, for example), instead
// of waiting for the per-backup timeout, and that both errors are reported.
func TestStreamBackupToS3UploadFailureStopsIdleDump(t *testing.T) {
	cli := &fakeUploadClient{readLimit: 0, putErr: errors.New("upload rejected")}
	stubStreamPipeline(t, cli, func(ctx context.Context, _ cfgDB, _ io.Writer) error {
		<-ctx.Done() // idle until the dump is canceled
		return errors.Wrap(ctx.Err(), "pg_dump idle")
	})

	err := runWithGuard(t, "streamBackupToS3", func() error {
		return streamBackupToS3(context.Background(), cfgDB{Database: "appdb"}, cfgS3{Bucket: "bucket"}, "appdb-20261119.gz")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "upload rejected")
	require.Contains(t, err.Error(), "pg_dump idle", "the dump error must not be dropped")
	require.ErrorIs(t, err, context.Canceled)
}

// TestStreamBackupToS3ContextDeadlineUnblocksPipeline verifies that the per-backup
// deadline stops the stream even when neither side of the pipe observes ctx itself.
func TestStreamBackupToS3ContextDeadlineUnblocksPipeline(t *testing.T) {
	cli := &fakeUploadClient{readLimit: -1}
	stubStreamPipeline(t, cli, endlessDump)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := runWithGuard(t, "streamBackupToS3", func() error {
		return streamBackupToS3(ctx, cfgDB{Database: "appdb"}, cfgS3{Bucket: "bucket"}, "appdb-20261119.gz")
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestStreamBackupToS3Success verifies that a fully consumed dump uploads every byte
// and reports success.
func TestStreamBackupToS3Success(t *testing.T) {
	payload := bytes.Repeat([]byte("pg-dump-payload\n"), 64*1024)
	cli := &fakeUploadClient{readLimit: -1, keep: true}
	stubStreamPipeline(t, cli, func(_ context.Context, _ cfgDB, w io.Writer) error {
		if _, err := w.Write(payload); err != nil {
			return errors.Wrap(err, "write payload")
		}
		return nil
	})

	err := runWithGuard(t, "streamBackupToS3", func() error {
		return streamBackupToS3(context.Background(), cfgDB{Database: "appdb"}, cfgS3{Bucket: "bucket"}, "appdb-20261119.gz")
	})
	require.NoError(t, err)
	require.Equal(t, payload, cli.received.Bytes())
}

// stubPgDump makes dumpAndGzipToWriter run script under sh instead of pg_dump. The
// returned pointer receives the created command so tests can inspect its final state.
func stubPgDump(t *testing.T, script string) **exec.Cmd {
	t.Helper()
	orig := pgDumpCommand
	t.Cleanup(func() { pgDumpCommand = orig })

	created := new(*exec.Cmd)
	pgDumpCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		*created = exec.CommandContext(ctx, "sh", "-c", script)
		return *created
	}
	return created
}

// failingWriter rejects every write, like a pipe whose reader has gone away.
type failingWriter struct{}

// Write always fails.
func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("destination closed")
}

// TestDumpAndGzipToWriterReapsProcessOnWriteFailure verifies that when the dump
// destination fails, the killed pg_dump process is also waited for, so it does not
// linger as a zombie in the long-running server.
func TestDumpAndGzipToWriterReapsProcessOnWriteFailure(t *testing.T) {
	created := stubPgDump(t, "while :; do echo backup-row; done")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := runWithGuard(t, "dumpAndGzipToWriter", func() error {
		return dumpAndGzipToWriter(ctx, cfgDB{}, failingWriter{})
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "destination closed")
	require.NotNil(t, *created)
	require.NotNil(t, (*created).ProcessState, "killed pg_dump process was never reaped")
}

// TestDumpAndGzipToWriterDrainsLargeStderr verifies that a pg_dump writing more to
// stderr than a pipe buffer holds cannot deadlock the dump while stdout is copied.
func TestDumpAndGzipToWriterDrainsLargeStderr(t *testing.T) {
	// Only shell builtins are used so killing sh closes every pipe end.
	stubPgDump(t, `i=0; while [ $i -lt 6000 ]; do echo 0123456789012345678901234567890123456789012345678901234567890123 >&2; i=$((i+1)); done; printf dump-body`)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out bytes.Buffer
	err := runWithGuard(t, "dumpAndGzipToWriter", func() error {
		return dumpAndGzipToWriter(ctx, cfgDB{}, &out)
	})
	require.NoError(t, err)

	gz, err := gzip.NewReader(&out)
	require.NoError(t, err)
	body, err := io.ReadAll(gz)
	require.NoError(t, err)
	require.Equal(t, "dump-body", strings.TrimSpace(string(body)))
}
