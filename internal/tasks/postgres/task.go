// Package postgres implements automated PostgreSQL backup task.
package postgres

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/s3"
)

var logger = log.Logger.Named("postgres-backup")

type cfgS3 struct {
	Enable       bool
	Endpoint     string
	AccessKey    string
	AccessSecret string
	Bucket       string
}

type cfgDB struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	// backup_file_prefix controls the generated S3 object key or filename prefix.
	// If it ends with '/', it's treated as an S3 key prefix (directory-like), and
	// the final object key will be: "<prefix><database>-YYYYMMDD.gz".
	// Otherwise, it's treated as the filename prefix (optionally with directories),
	// and the final object key will be: "<dir>/<prefix>-YYYYMMDD.gz".
	BackupFilePrefix string `mapstructure:"backup_file_prefix"`
}

type cfgBackup struct {
	Enable      bool
	IntervalSec int
	DBs         []cfgDB
	S3          cfgS3
	// UseTempFile switches the backup pipeline from stream->S3 to
	// dump->gzip->temporary file, then upload the file to S3.
	// This is useful for easier troubleshooting and when downstream
	// needs object size ahead of time.
	UseTempFile bool `mapstructure:"use_temp_file"`
	// TempDir specifies where to place temporary backup files when
	// UseTempFile is true. If empty, os.TempDir() will be used.
	TempDir string `mapstructure:"temp_dir"`
}

func loadCfg() *cfgBackup {
	cfg := &cfgBackup{
		Enable:      gconfig.Shared.GetBool("tasks.postgres.enable"),
		IntervalSec: gconfig.Shared.GetInt("tasks.postgres.interval"),
		S3: cfgS3{
			Enable:       gconfig.Shared.GetBool("tasks.postgres.s3.enable"),
			Endpoint:     gconfig.Shared.GetString("tasks.postgres.s3.endpoint"),
			AccessKey:    gconfig.Shared.GetString("tasks.postgres.s3.access_key"),
			AccessSecret: gconfig.Shared.GetString("tasks.postgres.s3.access_secret"),
			Bucket:       gconfig.Shared.GetString("tasks.postgres.s3.bucket"),
		},
		UseTempFile: gconfig.Shared.GetBool("tasks.postgres.use_temp_file"),
		TempDir:     gconfig.Shared.GetString("tasks.postgres.temp_dir"),
	}

	// Load multiple databases: tasks.postgres.dbs: [ {host,port,user,password,database,backup_file_prefix}, ... ]
	var dbs []cfgDB
	if err := gconfig.Shared.UnmarshalKey("tasks.postgres.dbs", &dbs); err == nil && len(dbs) > 0 {
		cfg.DBs = dbs
	}
	return cfg
}

// s3KeyFor builds the object key.
// cleanS3Path cleans a user-provided S3 key/path to use forward slashes
// and removes any leading slash.
func cleanS3Path(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.TrimPrefix(p, "/")
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return p
}

// buildKeyAndFilename computes the final S3 object key and a local filename (no slashes).
// Rules:
//   - If BackupFilePrefix ends with '/', treat it as directory prefix and use
//     '<database>-YYYYMMDD.gz' as the basename.
//   - Else, treat BackupFilePrefix as the basename (optionally with directory), and append
//     '-YYYYMMDD.gz'.
//   - Local filename never contains '/'.
func buildKeyAndFilename(db cfgDB, date string) (key string, filename string) {
	pfx := db.BackupFilePrefix
	if strings.HasSuffix(pfx, "/") {
		// Directory-like prefix
		dir := cleanS3Path(pfx)
		base := db.Database
		if strings.TrimSpace(base) == "" {
			base = "pg-backup"
		}
		filename = fmt.Sprintf("%s-%s.gz", base, date)
		if dir == "" {
			key = filename
		} else {
			key = dir + "/" + filename
		}
		return
	}

	// Filename-like prefix (may include directories)
	cleaned := cleanS3Path(pfx)
	dir, base := path.Split(cleaned)
	if base == "" {
		// No usable base in prefix; fall back to database name
		base = db.Database
		if strings.TrimSpace(base) == "" {
			base = "pg-backup"
		}
	}
	filename = fmt.Sprintf("%s-%s.gz", base, date)
	if dir == "" {
		key = filename
	} else {
		key = path.Clean(dir) + "/" + filename
	}
	return
}

// s3ObjectExists checks if object already exists.
func s3ObjectExists(ctx context.Context, s cfgS3, bucket, key string) (bool, error) {
	s3cli, err := s3.GetCli(s.Endpoint, s.AccessKey, s.AccessSecret)
	if err != nil {
		return false, errors.Wrap(err, "new s3 client")
	}

	_, err = s3cli.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err == nil {
		logger.Debug("s3 object exists", zap.String("bucket", bucket), zap.String("key", key))
		return true, nil
	}
	resp := minio.ToErrorResponse(err)
	if resp.StatusCode == 404 || strings.EqualFold(resp.Code, "NoSuchKey") {
		logger.Debug("s3 object not found", zap.String("bucket", bucket), zap.String("key", key))
		return false, nil
	}
	logger.Warn("failed to stat s3 object", zap.String("bucket", bucket), zap.String("key", key), zap.Error(err))
	return false, err
}

// runBackup performs pg_dump -> gzip -> local file, then uploads to S3 if enabled.
func runBackup() {
	cfg := loadCfg()
	if !cfg.Enable {
		logger.Warn("postgres backup disabled by config; skip")
		return
	}

	if !cfg.S3.Enable {
		logger.Warn("s3 upload disabled; skip backup to avoid local storage")
		return
	}

	// Require DB list
	dbs := cfg.DBs
	if len(dbs) == 0 {
		logger.Warn("no postgres databases configured; skip")
		return
	}

	today := time.Now().Format("20060102")
	logger.Info("postgres backup run start",
		zap.Bool("use_temp_file", cfg.UseTempFile),
		zap.String("temp_dir", cfg.TempDir),
		zap.String("s3_endpoint", cfg.S3.Endpoint),
		zap.String("s3_bucket", cfg.S3.Bucket),
		zap.Int("dbs", len(dbs)))
	perBackupTimeout := 30 * time.Minute

	for _, db := range dbs {
		// Per-db context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), perBackupTimeout)
		func() {
			defer cancel()

			// Compute S3 key and local filename
			key, fname := buildKeyAndFilename(db, today)

			logger.Info("start postgres backup",
				zap.String("file", fname),
				zap.String("db", db.Database),
				zap.String("pg_host", db.Host),
				zap.Int("pg_port", db.Port),
				zap.String("pg_user", db.User))

			// Skip if today's backup for this DB already exists in S3
			if exists, err := s3ObjectExists(ctx, cfg.S3, cfg.S3.Bucket, key); err != nil {
				logger.Warn("cannot check existing backup; will proceed", zap.Error(err), zap.String("object", key))
			} else if exists {
				logger.Info("backup already exists; skip", zap.String("object", key))
				return
			}

			start := time.Now()
			var err error
			if cfg.UseTempFile {
				err = backupViaTempFile(ctx, db, cfg.S3, fname, key, cfg.TempDir)
			} else {
				err = streamBackupToS3(ctx, db, cfg.S3, key)
			}
			if err != nil {
				logger.Error("backup failed", zap.String("object", key), zap.Error(err))
				return
			}
			logger.Info("uploaded to s3", zap.String("object", key), zap.String("cost", gutils.CostSecs(time.Since(start))))
		}()
	}
}

// dumpAndGzipToWriter runs pg_dump and writes gzip-compressed output into w.
func dumpAndGzipToWriter(ctx context.Context, db cfgDB, w io.Writer) error {
	// Build pg_dump command
	args := []string{
		"-h", db.Host,
		"-p", fmt.Sprintf("%d", db.Port),
		"-U", db.User,
		"-d", db.Database,
	}

	cmd := exec.CommandContext(ctx, "pg_dump", args...)
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
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errors.Wrap(err, "StderrPipe")
	}

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
		_ = cmd.Process.Kill() // ensure process terminated
		logger.Error("stream copy failed", zap.Int64("bytes", n), zap.Error(err))
		return errors.Wrap(err, "copy dump -> gzip")
	}
	if err := gz.Close(); err != nil {
		return errors.Wrap(err, "close gzip")
	}

	// Read stderr for logging if non-empty
	if bs, _ := io.ReadAll(stderr); len(bs) > 0 {
		logger.Named("pg_dump").Warn("stderr", zap.String("msg", string(bs)))
	}

	if err := cmd.Wait(); err != nil {
		// add exit code if possible
		if ee, ok := err.(*exec.ExitError); ok {
			logger.Error("pg_dump exited with error", zap.Int("exit_code", ee.ExitCode()))
		}
		return errors.Wrap(err, "wait pg_dump")
	}
	return nil
}

// streamBackupToS3 connects a pipe between pg_dump->gzip and S3 PutObject.
func streamBackupToS3(ctx context.Context, db cfgDB, s cfgS3, key string) error {
	s3cli, err := s3.GetCli(s.Endpoint, s.AccessKey, s.AccessSecret)
	if err != nil {
		return errors.Wrap(err, "new s3 client")
	}

	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		// CloseWithError ensures the reader sees error if any
		err := dumpAndGzipToWriter(ctx, db, pw)
		_ = pw.CloseWithError(err)
		errCh <- err
	}()

	// Size -1 enables streaming multipart upload
	logger.Info("s3 put (stream)", zap.String("bucket", s.Bucket), zap.String("key", key))
	info, putErr := s3cli.PutObject(ctx, s.Bucket, key, pr, -1, minio.PutObjectOptions{
		ContentType:     "application/gzip",
		ContentEncoding: "gzip",
	})

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
	info, putErr := s3cli.PutObject(ctx, s.Bucket, key, rf, size, minio.PutObjectOptions{
		ContentType:     "application/gzip",
		ContentEncoding: "gzip",
	})
	if putErr != nil {
		return errors.Wrap(putErr, "put object")
	}
	logger.Info("backup completed (file)", zap.String("object", key), zap.Int64("uploaded_size", info.Size), zap.String("etag", info.ETag))
	return nil
}

func bindPostgresBackupTask() {
	logger.Info("bind postgres backup task...")
	cfg := loadCfg()
	if !cfg.Enable {
		logger.Info("postgres backup disabled by config; not binding task")
		return
	}
	interval := cfg.IntervalSec
	if interval <= 0 {
		interval = 86400 // default 1 day
	}
	logger.Info("postgres backup config",
		zap.Int("interval_sec", interval),
		zap.Int("db_count", len(cfg.DBs)),
		zap.Bool("use_temp_file", cfg.UseTempFile),
		zap.String("temp_dir", cfg.TempDir),
		zap.Bool("s3_enable", cfg.S3.Enable),
		zap.String("s3_endpoint", cfg.S3.Endpoint),
		zap.String("s3_bucket", cfg.S3.Bucket))
	go store.TaskStore.TickerAfterRun(time.Duration(interval)*time.Second, runBackup)
}

func init() {
	store.TaskStore.Store("postgres", bindPostgresBackupTask)
}
