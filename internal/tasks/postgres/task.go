// Package postgres implements automated PostgreSQL backup task.
package postgres

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/s3"
)

var logger = log.Logger.Named("postgres-backup")

const (
	defaultBackupHistoryLimit     = 14
	backupObjectDateLayout        = "20060102"
	backupRetentionCleanupTimeout = 10 * time.Minute
)

type cfgS3 struct {
	Enable       bool
	Endpoint     string
	AccessKey    string
	AccessSecret string
	Bucket       string
	KeepLast     int `mapstructure:"keep_last"`
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

// loadCfg loads the postgres backup task configuration and applies local defaults.
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
			KeepLast:     normalizeBackupHistoryLimit(gconfig.Shared.GetInt("tasks.postgres.s3.keep_last")),
		},
		UseTempFile: gconfig.Shared.GetBool("tasks.postgres.use_temp_file"),
		TempDir:     gconfig.Shared.GetString("tasks.postgres.temp_dir"),
	}

	// Load multiple databases: tasks.postgres.dbs: [ {host,port,user,password,database,backup_file_prefix}, ... ]
	var dbs []cfgDB
	if err := gconfig.Shared.UnmarshalKey("tasks.postgres.dbs", &dbs); err != nil {
		return cfg
	}

	if len(dbs) > 0 {
		cfg.DBs = dbs
	}
	return cfg
}

// normalizeBackupHistoryLimit clamps the backup history limit to a supported default.
func normalizeBackupHistoryLimit(limit int) int {
	if limit < 1 {
		return defaultBackupHistoryLimit
	}

	return limit
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

// buildKeyPrefixAndBase derives the managed S3 key prefix and basename for one database backup series.
func buildKeyPrefixAndBase(db cfgDB) (keyPrefix string, base string) {
	pfx := strings.TrimSpace(db.BackupFilePrefix)
	if strings.HasSuffix(pfx, "/") {
		dir := cleanS3Path(pfx)
		base = strings.TrimSpace(db.Database)
		if base == "" {
			base = "pg-backup"
		}

		if dir == "" {
			return base + "-", base
		}

		return dir + "/" + base + "-", base
	}

	cleaned := cleanS3Path(pfx)
	dir, base := path.Split(cleaned)
	if strings.TrimSpace(base) == "" {
		base = strings.TrimSpace(db.Database)
		if base == "" {
			base = "pg-backup"
		}
	}

	if dir == "" {
		return base + "-", base
	}

	return path.Clean(dir) + "/" + base + "-", base
}

// buildKeyAndFilename computes the final S3 object key and a local filename (no slashes).
// Rules:
//   - If BackupFilePrefix ends with '/', treat it as directory prefix and use
//     '<database>-YYYYMMDD.gz' as the basename.
//   - Else, treat BackupFilePrefix as the basename (optionally with directory), and append
//     '-YYYYMMDD.gz'.
//   - Local filename never contains '/'.
func buildKeyAndFilename(db cfgDB, date string) (key string, filename string) {
	keyPrefix, base := buildKeyPrefixAndBase(db)
	filename = fmt.Sprintf("%s-%s.gz", base, date)
	return keyPrefix + date + ".gz", filename
}

// isManagedBackupKey reports whether key belongs to the configured backup series and naming scheme.
func isManagedBackupKey(keyPrefix, key string) bool {
	if !strings.HasPrefix(key, keyPrefix) || !strings.HasSuffix(key, ".gz") {
		return false
	}

	datePart := strings.TrimSuffix(strings.TrimPrefix(key, keyPrefix), ".gz")
	if len(datePart) != len(backupObjectDateLayout) {
		return false
	}

	if _, err := time.Parse(backupObjectDateLayout, datePart); err != nil {
		return false
	}

	return true
}

// selectExpiredBackupKeys picks the oldest managed backup keys that exceed keepLast.
func selectExpiredBackupKeys(keyPrefix string, keys []string, keepLast int) []string {
	keepLast = normalizeBackupHistoryLimit(keepLast)

	managedKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if isManagedBackupKey(keyPrefix, key) {
			managedKeys = append(managedKeys, key)
		}
	}

	if len(managedKeys) <= keepLast {
		return nil
	}

	slices.Sort(managedKeys)
	slices.Reverse(managedKeys)

	expiredKeys := make([]string, 0, len(managedKeys)-keepLast)
	expiredKeys = append(expiredKeys, managedKeys[keepLast:]...)
	return expiredKeys
}

// cleanupExpiredBackups removes managed S3 backup objects that exceed the configured history limit.
func cleanupExpiredBackups(ctx context.Context, s cfgS3, keyPrefix string) error {
	s3cli, err := s3.GetCli(s.Endpoint, s.AccessKey, s.AccessSecret)
	if err != nil {
		return errors.Wrap(err, "new s3 client")
	}

	keys := make([]string, 0, s.KeepLast+1)
	for obj := range s3cli.ListObjects(ctx, s.Bucket, minio.ListObjectsOptions{Prefix: keyPrefix, Recursive: true}) {
		if obj.Err != nil {
			return errors.Wrapf(obj.Err, "list objects with prefix %q", keyPrefix)
		}

		keys = append(keys, obj.Key)
	}

	expiredKeys := selectExpiredBackupKeys(keyPrefix, keys, s.KeepLast)
	if len(expiredKeys) == 0 {
		logger.Debug("postgres backup retention within limit",
			zap.String("bucket", s.Bucket),
			zap.String("key_prefix", keyPrefix),
			zap.Int("keep_last", s.KeepLast),
			zap.Int("objects", len(keys)))
		return nil
	}

	logger.Info("cleanup expired postgres backups",
		zap.String("bucket", s.Bucket),
		zap.String("key_prefix", keyPrefix),
		zap.Int("keep_last", s.KeepLast),
		zap.Int("delete_count", len(expiredKeys)))
	for _, expiredKey := range expiredKeys {
		if err := s3cli.RemoveObject(ctx, s.Bucket, expiredKey, minio.RemoveObjectOptions{}); err != nil {
			return errors.Wrapf(err, "remove object %q", expiredKey)
		}

		logger.Info("deleted expired postgres backup",
			zap.String("bucket", s.Bucket),
			zap.String("object", expiredKey))
	}

	return nil
}

// startBackupRetentionCleanup launches background retention cleanup for one backup series.
func startBackupRetentionCleanup(db cfgDB, s cfgS3, uploadedKey string) {
	keyPrefix, _ := buildKeyPrefixAndBase(db)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), backupRetentionCleanupTimeout)
		defer cancel()

		if err := cleanupExpiredBackups(ctx, s, keyPrefix); err != nil {
			logger.Warn("postgres backup retention cleanup failed",
				zap.String("bucket", s.Bucket),
				zap.String("uploaded_object", uploadedKey),
				zap.String("key_prefix", keyPrefix),
				zap.Int("keep_last", s.KeepLast),
				zap.Error(err))
		}
	}()
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
	if resp.StatusCode == http.StatusNotFound || strings.EqualFold(resp.Code, "NoSuchKey") {
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

	today := time.Now().UTC().Format(backupObjectDateLayout)
	logger.Info("postgres backup run start",
		zap.Bool("use_temp_file", cfg.UseTempFile),
		zap.String("temp_dir", cfg.TempDir),
		zap.String("s3_endpoint", cfg.S3.Endpoint),
		zap.String("s3_bucket", cfg.S3.Bucket),
		zap.Int("s3_keep_last", cfg.S3.KeepLast),
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

			startBackupRetentionCleanup(db, cfg.S3, key)
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
		ee := &exec.ExitError{}
		if errors.As(err, &ee) {
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
	info, putErr := s3.PutObjectCappingVersions(ctx, logger, s3cli, s.Bucket, key, pr, -1, minio.PutObjectOptions{
		ContentType:     "application/gzip",
		ContentEncoding: "gzip",
	}, s3.DefaultVersionsToKeep)

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

// bindPostgresBackupTask registers the scheduled postgres backup task when enabled.
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
		zap.Int("s3_keep_last", cfg.S3.KeepLast),
		zap.String("s3_endpoint", cfg.S3.Endpoint),
		zap.String("s3_bucket", cfg.S3.Bucket))
	go store.TaskStore.TickerAfterRun(time.Duration(interval)*time.Second, runBackup)
}

// init registers the postgres backup task in the shared task store.
func init() {
	store.TaskStore.Store("postgres", bindPostgresBackupTask)
}
