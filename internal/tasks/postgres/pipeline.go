package postgres

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"filippo.io/age"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/library/s3"
)

// errStreamUploadReturned is the pipe error a still-running dump producer observes
// once the streaming upload has returned and nothing reads the pipe any more.
var errStreamUploadReturned = errors.New("s3 upload returned before the dump stream was fully consumed")

// streamBackupPartSize is the multipart part size of a streaming backup upload.
// The dump size is unknown, and without a part size minio-go sizes parts for a
// 5 TiB object and allocates one such part buffer (about 528 MiB) per upload, even
// for a tiny database. 64 MiB parts bound that buffer while still allowing
// 10000 x 64 MiB = 625 GiB of compressed dump.
const streamBackupPartSize = 64 << 20

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
		return errors.Wrapf(err, "start pg_dump for %s:%d/%s", db.Host, db.Port, db.Database)
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
			return errors.Wrapf(waitErr, "wait pg_dump (exit code %d)", ee.ExitCode())
		}
		return errors.Wrap(waitErr, "wait pg_dump")
	}
	return nil
}

// maxBackupPasswordFileBytes caps memory use for an accidentally misconfigured
// secret file before attempting the age KDF.
const maxBackupPasswordFileBytes = 4096

// readBackupPassword returns the configured passphrase, reading at most one
// small secret file. Errors avoid exposing the pathname or its contents.
func readBackupPassword(db cfgDB) (string, error) {
	if db.BackupEncryptionPassword != "" && db.BackupEncryptionPasswordFile != "" {
		return "", errors.New("configure only one encryption password source")
	}
	if db.BackupEncryptionPasswordFile == "" {
		return db.BackupEncryptionPassword, nil
	}
	info, err := os.Stat(db.BackupEncryptionPasswordFile)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("backup encryption password file must be a readable regular file")
	}
	f, err := os.Open(db.BackupEncryptionPasswordFile)
	if err != nil {
		return "", errors.New("cannot open backup encryption password file")
	}
	info, statErr := f.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		if closeErr := f.Close(); closeErr != nil {
			return "", errors.New("cannot close invalid backup encryption password file")
		}
		return "", errors.New("backup encryption password file must be a regular file")
	}
	data, readErr := io.ReadAll(io.LimitReader(f, maxBackupPasswordFileBytes+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil {
		return "", errors.New("cannot read or close backup encryption password file")
	}
	if len(data) > maxBackupPasswordFileBytes {
		return "", errors.New("backup encryption password file exceeds 4096 bytes")
	}
	passphrase := strings.TrimSuffix(string(data), "\n")
	if len(data) > 0 && data[len(data)-1] == '\n' {
		passphrase = strings.TrimSuffix(passphrase, "\r")
	}
	if strings.TrimSpace(passphrase) == "" {
		return "", errors.New("backup encryption password file is empty")
	}
	return passphrase, nil
}

// writeBackup emits a gzipped logical dump, wrapping it in standard age
// passphrase encryption when configured. It never finalizes a failed dump or
// retries it as plaintext.
func writeBackup(ctx context.Context, db cfgDB, dst io.Writer) error {
	if backupSuffix(db) == gzipBackupSuffix {
		return errors.Wrap(dumpDatabase(ctx, db, dst), "write gzip backup")
	}
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "initiate encrypted backup")
	}
	password, err := readBackupPassword(db)
	if err != nil {
		return errors.Wrap(err, "load backup encryption password")
	}
	recipient, err := age.NewScryptRecipient(password)
	if err != nil {
		return errors.Wrap(err, "prepare age backup encryption")
	}
	encrypted, err := age.Encrypt(dst, recipient)
	if err != nil {
		return errors.Wrap(err, "start age backup encryption")
	}
	if err := dumpDatabase(ctx, db, encrypted); err != nil {
		return errors.Wrap(err, "write encrypted backup")
	}
	return errors.Wrap(encrypted.Close(), "finish age backup encryption")
}

// backupUploadOptions returns object metadata matching the outer file format.
func backupUploadOptions(db cfgDB) minio.PutObjectOptions {
	if backupSuffix(db) == encryptedBackupSuffix {
		return minio.PutObjectOptions{ContentType: "application/octet-stream"}
	}
	return minio.PutObjectOptions{ContentType: "application/gzip", ContentEncoding: "gzip"}
}

// streamBackupToS3 pipes a backup through optional encryption to S3 PutObject.
//
// Parameters:
//   - ctx: Bounds the whole backup; when it ends the pipe is failed so neither the
//     producer nor the uploader can block past it.
//   - db: Database to dump.
//   - s: S3 settings of the destination bucket.
//   - key: Destination object key.
//
// It returns an error when the upload or the dump fails, joining both causes when
// both fail. It never returns before the dump producer goroutine has exited.
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

	// The dump gets its own context so it can be stopped once the upload returns.
	// Closing the pipe frees a producer blocked in a write, but not one whose
	// pg_dump is idle (e.g. waiting on a lock), which would otherwise run until
	// the per-backup timeout and delay the remaining databases.
	dumpCtx, cancelDump := context.WithCancel(ctx)
	defer cancelDump()

	errCh := make(chan error, 1)
	go func() {
		// CloseWithError ensures the reader sees error if any
		err := writeBackup(dumpCtx, db, pw)
		_ = pw.CloseWithError(err)
		errCh <- err
	}()

	// Size -1 enables streaming multipart upload; PartSize bounds its part buffer.
	logger.Info("s3 put (stream)", zap.String("bucket", s.Bucket), zap.String("key", key))
	opts := backupUploadOptions(db)
	opts.PartSize = streamBackupPartSize
	info, putErr := s3.PutObjectCappingVersions(ctx, logger, s3cli, s.Bucket, key, pr, -1, opts, s3.DefaultVersionsToKeep)

	// Nothing reads the pipe after the upload returns. Closing the read side makes
	// any pending or future producer write fail instead of blocking forever, which
	// would otherwise hang this backup and every later one in the run.
	// PipeReader.CloseWithError always returns nil.
	_ = pr.CloseWithError(errStreamUploadReturned)
	// A successful upload has read EOF, which the producer only sends after the
	// dump finished, so canceling here can only stop an unfinished dump.
	cancelDump()

	dumpErr := <-errCh // wait producer
	if err := errors.Join(
		errors.Wrap(putErr, "put object"),
		errors.Wrap(dumpErr, "pg_dump pipeline"),
	); err != nil {
		return errors.Wrap(err, "stream backup")
	}

	logger.Info("backup completed",
		zap.String("object", key),
		zap.Int64("uploaded_size", info.Size),
		zap.String("etag", info.ETag))
	return nil
}

// backupViaTempFile writes the complete archive to a private temporary file
// before uploading it. An encrypted backup never writes plaintext to disk.
func backupViaTempFile(ctx context.Context, db cfgDB, s cfgS3, fname, key, tempDir string) (returnErr error) {
	s3cli, err := newBackupS3Client(s)
	if err != nil {
		return errors.Wrap(err, "new s3 client")
	}

	if tempDir == "" {
		tempDir = os.TempDir()
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return errors.Wrap(err, "mkdir temp dir")
	}
	tmp, err := os.CreateTemp(tempDir, fname+".*")
	if err != nil {
		return errors.Wrap(err, "create temp file")
	}
	defer func() {
		returnErr = errors.Wrap(errors.Join(
			returnErr,
			errors.Wrap(tmp.Close(), "close temp backup file"),
			errors.Wrap(os.Remove(tmp.Name()), "remove temp backup file"),
		), "temp file backup")
	}()

	if err := writeBackup(ctx, db, tmp); err != nil {
		return errors.Wrap(err, "write temp backup file")
	}
	// Seek returns the size, then resets the same descriptor for upload. Keeping
	// the original randomly named descriptor avoids reopening a shared pathname.
	size, err := tmp.Seek(0, io.SeekEnd)
	if err != nil {
		return errors.Wrap(err, "measure temp backup file")
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return errors.Wrap(err, "rewind temp backup file")
	}

	logger.Info("s3 put (file)", zap.String("bucket", s.Bucket), zap.String("key", key), zap.Int64("size", size))
	info, putErr := s3.PutObjectCappingVersions(ctx, logger, s3cli, s.Bucket, key, tmp, size, backupUploadOptions(db), s3.DefaultVersionsToKeep)
	if putErr != nil {
		return errors.Wrap(putErr, "put object")
	}
	logger.Info("backup completed (file)", zap.String("object", key), zap.Int64("uploaded_size", info.Size), zap.String("etag", info.ETag))
	return nil
}
