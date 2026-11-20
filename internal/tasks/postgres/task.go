// Package postgres implements automated PostgreSQL backup task.
package postgres

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/go-viper/mapstructure/v2"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/s3"
)

var logger = log.Logger.Named("postgres-backup")

// backupRunMu allows only one active backup run per process.
var backupRunMu sync.Mutex

const (
	defaultBackupHistoryLimit     = 14
	backupObjectDateLayout        = "20060102"
	backupRetentionCleanupTimeout = 10 * time.Minute
	gzipBackupSuffix              = ".gz"
	encryptedBackupSuffix         = ".gz.age"
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
	// BackupEncryptionPassword is distinct from the PostgreSQL connection password.
	BackupEncryptionPassword string `mapstructure:"backup_encryption_password"`
	// BackupEncryptionPasswordFile optionally supplies the passphrase via a mounted secret.
	BackupEncryptionPasswordFile string `mapstructure:"backup_encryption_password_file"`
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
func loadCfg(config gconfig.Config) *cfgBackup {
	cfg := &cfgBackup{
		Enable:      config.GetBool("tasks.postgres.enable"),
		IntervalSec: config.GetInt("tasks.postgres.interval"),
		S3: cfgS3{
			Enable:       config.GetBool("tasks.postgres.s3.enable"),
			Endpoint:     config.GetString("tasks.postgres.s3.endpoint"),
			AccessKey:    config.GetString("tasks.postgres.s3.access_key"),
			AccessSecret: config.GetString("tasks.postgres.s3.access_secret"),
			Bucket:       config.GetString("tasks.postgres.s3.bucket"),
			KeepLast:     normalizeBackupHistoryLimit(config.GetInt("tasks.postgres.s3.keep_last")),
		},
		UseTempFile: config.GetBool("tasks.postgres.use_temp_file"),
		TempDir:     config.GetString("tasks.postgres.temp_dir"),
	}

	// Older Ramjet versions ignore encrypted_dbs altogether. Keep databases
	// that must never fall back to plaintext out of the ordinary dbs list.
	cfg.DBs = slices.Concat(
		loadBackupDatabaseList(config, "dbs", false),
		loadBackupDatabaseList(config, "encrypted_dbs", true),
	)
	return cfg
}

// loadBackupDatabaseList loads one optional database list and rejects a
// malformed value without affecting the other list.
func loadBackupDatabaseList(config gconfig.Config, key string, requireEncryption bool) []cfgDB {
	switch raw := config.Get("tasks.postgres." + key).(type) {
	case nil:
		return nil
	case []any:
		return decodeBackupDatabases(raw, key, requireEncryption)
	case []map[string]any:
		entries := make([]any, len(raw))
		for i, entry := range raw {
			entries[i] = entry
		}
		return decodeBackupDatabases(entries, key, requireEncryption)
	default:
		logger.Error("postgres backup database setting must be a list", zap.String("list", key))
		return nil
	}
}

// decodeBackupDatabases decodes each database independently and reports invalid
// entries by index only, never logging values or decoder errors that echo them.
func decodeBackupDatabases(entries []any, list string, requireEncryption bool) []cfgDB {
	dbs := make([]cfgDB, 0, len(entries))
	for i, entry := range entries {
		fields, ok := entry.(map[string]any)
		if !ok {
			logger.Error("postgres backup entry must be a mapping", zap.String("list", list), zap.Int("entry", i+1))
			continue
		}
		var db cfgDB
		if err := mapstructure.WeakDecode(fields, &db); err != nil {
			logger.Error("invalid postgres backup entry", zap.String("list", list), zap.Int("entry", i+1))
			continue
		}
		// Do not coerce an invalid value or interpret a blank password source
		// as permission to upload plaintext. Omit both options to opt out.
		for key, value := range fields {
			if strings.EqualFold(key, "backup_encryption_password") || strings.EqualFold(key, "backup_encryption_password_file") {
				if val, isString := value.(string); !isString || strings.TrimSpace(val) == "" {
					logger.Error("postgres backup password source must be a non-empty string", zap.String("list", list), zap.Int("entry", i+1))
					ok = false
				}
			}
		}
		if db.BackupEncryptionPassword != "" && db.BackupEncryptionPasswordFile != "" {
			logger.Error("postgres backup entry must configure only one encryption password source", zap.String("list", list), zap.Int("entry", i+1))
			ok = false
		}
		if ok && requireEncryption && backupSuffix(db) != encryptedBackupSuffix {
			logger.Error("postgres encrypted backup entry requires an encryption password source", zap.String("list", list), zap.Int("entry", i+1))
			ok = false
		}
		if ok {
			dbs = append(dbs, db)
		}
	}
	return dbs
}

// backupSuffix selects a distinct name for the encrypted format so an existing
// plaintext object from the same day never suppresses a new encrypted backup.
func backupSuffix(db cfgDB) string {
	if db.BackupEncryptionPassword != "" || db.BackupEncryptionPasswordFile != "" {
		return encryptedBackupSuffix
	}
	return gzipBackupSuffix
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
	suffix := backupSuffix(db)
	filename = fmt.Sprintf("%s-%s%s", base, date, suffix)
	return keyPrefix + date + suffix, filename
}

// isManagedBackupKey reports whether key belongs to the configured backup series and naming scheme.
func isManagedBackupKey(keyPrefix, suffix, key string) bool {
	if !strings.HasPrefix(key, keyPrefix) || !strings.HasSuffix(key, suffix) {
		return false
	}

	datePart := strings.TrimSuffix(strings.TrimPrefix(key, keyPrefix), suffix)
	if len(datePart) != len(backupObjectDateLayout) {
		return false
	}

	if _, err := time.Parse(backupObjectDateLayout, datePart); err != nil {
		return false
	}

	return true
}

// selectExpiredBackupKeys picks the oldest managed backup keys that exceed keepLast.
func selectExpiredBackupKeys(keyPrefix, suffix string, keys []string, keepLast int) []string {
	keepLast = normalizeBackupHistoryLimit(keepLast)

	managedKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if isManagedBackupKey(keyPrefix, suffix, key) {
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
func cleanupExpiredBackups(ctx context.Context, s cfgS3, keyPrefix, suffix string) error {
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

	expiredKeys := selectExpiredBackupKeys(keyPrefix, suffix, keys, s.KeepLast)
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

		if err := cleanupExpiredBackups(ctx, s, keyPrefix, backupSuffix(db)); err != nil {
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

// runBackup loads current settings and backs up the configured databases to S3.
func runBackup() {
	if !backupRunMu.TryLock() {
		logger.Warn("postgres backup already running; skip overlapping tick")
		return
	}
	defer backupRunMu.Unlock()

	cfg := loadCfg(gconfig.Shared)
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

	backupDatabases(cfg, time.Now().UTC().Format(backupObjectDateLayout), backupOneDatabase)
}

// backupDatabases runs the backup pipelines independently for one UTC date.
func backupDatabases(cfg *cfgBackup, today string, backup func(context.Context, cfgDB, *cfgBackup, string, string) error) {
	dbs := cfg.DBs
	prefixes := make(map[string]int, len(dbs))
	for _, db := range dbs {
		prefix, _ := buildKeyPrefixAndBase(db)
		prefixes[prefix]++
	}
	logger.Info("postgres backup run start",
		zap.Bool("use_temp_file", cfg.UseTempFile),
		zap.String("temp_dir", cfg.TempDir),
		zap.String("s3_endpoint", cfg.S3.Endpoint),
		zap.String("s3_bucket", cfg.S3.Bucket),
		zap.Int("s3_keep_last", cfg.S3.KeepLast),
		zap.Int("dbs", len(dbs)))
	perBackupTimeout := 30 * time.Minute

	for i, db := range dbs {
		prefix, _ := buildKeyPrefixAndBase(db)
		if prefixes[prefix] > 1 {
			logger.Error("postgres backup entries must use distinct object prefixes", zap.Int("entry", i+1), zap.String("key_prefix", prefix))
			continue
		}
		key, fname := buildKeyAndFilename(db, today)
		ctx, cancel := context.WithTimeout(context.Background(), perBackupTimeout)
		err := backup(ctx, db, cfg, key, fname)
		cancel()
		if err != nil {
			logger.Error("backup failed", zap.String("object", key), zap.Error(err))
		}
	}
}

// backupOneDatabase uploads one database's archive unless that day's key exists.
// Only a successfully uploaded backup starts retention cleanup for its format.
func backupOneDatabase(ctx context.Context, db cfgDB, cfg *cfgBackup, key, fname string) error {
	logger.Info("start postgres backup",
		zap.String("file", fname),
		zap.String("db", db.Database),
		zap.String("pg_host", db.Host),
		zap.Int("pg_port", db.Port),
		zap.String("pg_user", db.User))
	if exists, err := s3ObjectExists(ctx, cfg.S3, cfg.S3.Bucket, key); err != nil {
		logger.Warn("cannot check existing backup; will proceed", zap.Error(err), zap.String("object", key))
	} else if exists {
		logger.Info("backup already exists; skip", zap.String("object", key))
		return nil
	}

	start := time.Now()
	var err error
	if cfg.UseTempFile {
		err = backupViaTempFile(ctx, db, cfg.S3, fname, key, cfg.TempDir)
	} else {
		err = streamBackupToS3(ctx, db, cfg.S3, key)
	}
	if err != nil {
		return errors.Wrap(err, "upload database backup")
	}
	startBackupRetentionCleanup(db, cfg.S3, key)
	logger.Info("uploaded to s3", zap.String("object", key), zap.String("cost", gutils.CostSecs(time.Since(start))))
	return nil
}

// bindPostgresBackupTask registers the scheduled postgres backup task when enabled.
func bindPostgresBackupTask() {
	logger.Info("bind postgres backup task...")
	cfg := loadCfg(gconfig.Shared)
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
