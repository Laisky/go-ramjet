package postgres

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/library/s3"
)

// errStreamUploadReturned is the pipe error a still-running dump producer observes
// once the streaming upload has returned and nothing reads the pipe any more.
var errStreamUploadReturned = errors.New("s3 upload returned before the dump stream was fully consumed")

// pgDumpCommand builds the pg_dump command; it is a package variable so tests can
// substitute a fake process without a PostgreSQL installation.
var pgDumpCommand = exec.CommandContext

// dumpDatabase streams one gzip-compressed database dump into w; it is a package
// variable so tests can substitute a fake producer.
var dumpDatabase = dumpAndGzipToWriter

// newBackupS3Client returns the object store client used for streaming uploads; it
// is a package variable so tests can substitute an in-memory client.
var newBackupS3Client = func(s cfgS3) (s3.ObjectVersionClient, error) {
	cli, err := s3.GetCli(s.Endpoint, s.AccessKey, s.AccessSecret)
	if err != nil {
		return nil, errors.Wrap(err, "new s3 client")
	}

	return cli, nil
}

// dumpAndGzipToWriter runs pg_dump and writes gzip-compressed output into w.
//
// Parameters:
//   - ctx: Bounds the pg_dump process; cancellation kills it.
//   - db: Connection settings of the database to dump.
//   - w: Destination of the gzip-compressed dump.
//
// It returns an error when pg_dump cannot start, exits unsuccessfully, or the
// compressed output cannot be written to w. The pg_dump process is always waited
// for before returning, so it never lingers as a zombie.
func dumpAndGzipToWriter(ctx context.Context, db cfgDB, w io.Writer) error {
	// Build pg_dump command
	args := []string{
		"-h", db.Host,
		"-p", fmt.Sprintf("%d", db.Port),
		"-U", db.User,
		"-d", db.Database,
	}

	cmd := pgDumpCommand(ctx, "pg_dump", args...)
	// Pass password via env var for non-interactive
	env := os.Environ()
	if db.Password != "" {
		env = append(env, fmt.Sprintf("PGPASSWORD=%s", db.Password))
	}
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "StdoutPipe")
	}
	// Collect stderr concurrently (os/exec copies it in its own goroutine and Wait
	// waits for it). Reading it only after stdout hits EOF would deadlock once
	// pg_dump writes more than one pipe buffer of warnings.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	gz := gzip.NewWriter(w)
	defer func() { _ = gz.Close() }()

	if err := cmd.Start(); err != nil {
		logger.Error("failed to start pg_dump",
			zap.Strings("args", args),
			zap.Error(err))
		return errors.Wrap(err, "start pg_dump")
	}

	// Stream pg_dump stdout into gzip writer
	if n, err := io.Copy(gz, stdout); err != nil {
		// Terminate and reap the child; Kill alone leaves a zombie process behind.
		killErr := cmd.Process.Kill()
		waitErr := cmd.Wait()
		logger.Debug("pg_dump terminated after stream copy failure",
			zap.Int64("bytes", n),
			zap.NamedError("kill_error", killErr),
			zap.NamedError("wait_error", waitErr))
		return errors.Wrap(err, "copy dump -> gzip")
	}
	if err := gz.Close(); err != nil {
		killErr := cmd.Process.Kill()
		waitErr := cmd.Wait()
		logger.Debug("pg_dump terminated after gzip close failure",
			zap.NamedError("kill_error", killErr),
			zap.NamedError("wait_error", waitErr))
		return errors.Wrap(err, "close gzip")
	}

	waitErr := cmd.Wait()
	// stderr is complete only after Wait returns.
	if stderr.Len() > 0 {
		logger.Named("pg_dump").Warn("stderr", zap.String("msg", stderr.String()))
	}

	if waitErr != nil {
		// add exit code if possible
		ee := &exec.ExitError{}
		if errors.As(waitErr, &ee) {
			logger.Error("pg_dump exited with error", zap.Int("exit_code", ee.ExitCode()))
		}
		return errors.Wrap(waitErr, "wait pg_dump")
	}
	return nil
}

// streamBackupToS3 connects a pipe between pg_dump->gzip and S3 PutObject.
//
// Parameters:
//   - ctx: Bounds the whole backup; when it ends the pipe is failed so neither the
//     producer nor the uploader can block past it.
//   - db: Database to dump.
//   - s: S3 settings of the destination bucket.
//   - key: Destination object key.
//
// It returns an error when the upload or the dump fails. It never returns before
// the dump producer goroutine has exited.
func streamBackupToS3(ctx context.Context, db cfgDB, s cfgS3, key string) error {
	s3cli, err := newBackupS3Client(s)
	if err != nil {
		return errors.Wrap(err, "new s3 client")
	}

	pr, pw := io.Pipe()
	// When the backup context ends, fail the pipe so a blocked pipe read or write
	// returns even if the uploader or the producer does not observe ctx itself.
	// PipeWriter.CloseWithError always returns nil.
	stopCtxWatch := context.AfterFunc(ctx, func() {
		_ = pw.CloseWithError(errors.Wrap(context.Cause(ctx), "backup context done"))
	})
	defer stopCtxWatch()

	errCh := make(chan error, 1)
	go func() {
		// CloseWithError ensures the reader sees error if any
		err := dumpDatabase(ctx, db, pw)
		_ = pw.CloseWithError(err)
		errCh <- err
	}()

	// Size -1 enables streaming multipart upload
	logger.Info("s3 put (stream)", zap.String("bucket", s.Bucket), zap.String("key", key))
	info, putErr := s3.PutObjectCappingVersions(ctx, logger, s3cli, s.Bucket, key, pr, -1, minio.PutObjectOptions{
		ContentType:     "application/gzip",
		ContentEncoding: "gzip",
	}, s3.DefaultVersionsToKeep)

	// Nothing reads the pipe after the upload returns. Closing the read side makes
	// any pending or future producer write fail instead of blocking forever, which
	// would otherwise hang this backup and every later one in the run.
	// PipeReader.CloseWithError always returns nil.
	_ = pr.CloseWithError(errStreamUploadReturned)

	dumpErr := <-errCh // wait producer
	if putErr != nil {
		return errors.Wrap(putErr, "put object")
	}

	if dumpErr != nil {
		return errors.Wrap(dumpErr, "pg_dump pipeline")
	}

	logger.Info("backup completed",
		zap.String("object", key),
		zap.Int64("uploaded_size", info.Size),
		zap.String("etag", info.ETag))
	return nil
}

// backupViaTempFile writes dump->gzip to a temporary file, then uploads that file to S3.
// fname is the local filename (no slashes). key is the S3 object key.
func backupViaTempFile(ctx context.Context, db cfgDB, s cfgS3, fname, key, tempDir string) error {
	s3cli, err := s3.GetCli(s.Endpoint, s.AccessKey, s.AccessSecret)
	if err != nil {
		return errors.Wrap(err, "new s3 client")
	}

	if tempDir == "" {
		tempDir = os.TempDir()
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return errors.Wrap(err, "mkdir temp dir")
	}

	finalPath := tempDir + string(os.PathSeparator) + fname
	tmpf, err := os.CreateTemp(tempDir, fname+".*")
	if err != nil {
		return errors.Wrap(err, "create temp file")
	}
	tmpPath := tmpf.Name()
	_ = tmpf.Close()
	// reopen for writing through our pipeline
	wf, err := os.Create(tmpPath)
	if err != nil {
		return errors.Wrap(err, "open temp file for write")
	}
	logger.Info("dump to temp file", zap.String("tmp", tmpPath))
	if err := dumpAndGzipToWriter(ctx, db, wf); err != nil {
		_ = wf.Close()
		_ = os.Remove(tmpPath)
		return errors.Wrap(err, "dump to temp file")
	}
	if err := wf.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return errors.Wrap(err, "close temp file")
	}

	// rename to final path for easier inspection (best effort)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		logger.Warn("rename temp file failed; continue with tmp", zap.Error(err))
		finalPath = tmpPath
	}
	// ensure cleanup
	defer func() {
		if err := os.Remove(finalPath); err != nil {
			logger.Warn("cleanup temp file failed", zap.String("file", finalPath), zap.Error(err))
		} else {
			logger.Info("cleanup temp file", zap.String("file", finalPath))
		}
	}()

	// stat size for upload
	finfo, err := os.Stat(finalPath)
	if err != nil {
		return errors.Wrap(err, "stat temp file")
	}
	size := finfo.Size()
	rf, err := os.Open(finalPath)
	if err != nil {
		return errors.Wrap(err, "open temp file for read")
	}
	defer rf.Close()

	logger.Info("s3 put (file)", zap.String("bucket", s.Bucket), zap.String("key", key), zap.String("file", finalPath), zap.Int64("size", size))
	info, putErr := s3.PutObjectCappingVersions(ctx, logger, s3cli, s.Bucket, key, rf, size, minio.PutObjectOptions{
		ContentType:     "application/gzip",
		ContentEncoding: "gzip",
	}, s3.DefaultVersionsToKeep)
	if putErr != nil {
		return errors.Wrap(putErr, "put object")
	}
	logger.Info("backup completed (file)", zap.String("object", key), zap.Int64("uploaded_size", info.Size), zap.String("etag", info.ETag))
	return nil
}
