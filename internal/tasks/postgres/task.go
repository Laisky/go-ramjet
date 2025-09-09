// Package postgres implements automated PostgreSQL backup task.
package postgres

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	glog "github.com/Laisky/go-utils/v5/log"
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
	ObjectPrefix string // optional; if set, prefix object key
}

type cfgDB struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	Tables   []string // optional; if empty do full database
}

type cfgBackup struct {
	IntervalSec int
	DB          cfgDB
	S3          cfgS3
}

func loadCfg() *cfgBackup {
	return &cfgBackup{
		IntervalSec: gconfig.Shared.GetInt("tasks.postgres.interval"),
		DB: cfgDB{
			Host:     gconfig.Shared.GetString("tasks.postgres.db.host"),
			Port:     gconfig.Shared.GetInt("tasks.postgres.db.port"),
			User:     gconfig.Shared.GetString("tasks.postgres.db.user"),
			Password: gconfig.Shared.GetString("tasks.postgres.db.password"),
			Database: gconfig.Shared.GetString("tasks.postgres.db.database"),
			Tables:   gconfig.Shared.GetStringSlice("tasks.postgres.db.tables"),
		},
		S3: cfgS3{
			Enable:       gconfig.Shared.GetBool("tasks.postgres.s3.enable"),
			Endpoint:     gconfig.Shared.GetString("tasks.postgres.s3.endpoint"),
			AccessKey:    gconfig.Shared.GetString("tasks.postgres.s3.access_key"),
			AccessSecret: gconfig.Shared.GetString("tasks.postgres.s3.access_secret"),
			Bucket:       gconfig.Shared.GetString("tasks.postgres.s3.bucket"),
			ObjectPrefix: gconfig.Shared.GetString("tasks.postgres.s3.object_prefix"),
		},
	}
}

// runBackup performs pg_dump -> gzip -> local file, then uploads to S3 if enabled.
func runBackup() {
	cfg := loadCfg()
	if !cfg.S3.Enable {
		logger.Warn("s3 upload disabled; skip backup to avoid local storage")
		return
	}

	today := time.Now().Format("20060102")
	fname := fmt.Sprintf("postgre-backup-%s.gz", today)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	start := time.Now()
	if err := streamBackupToS3(ctx, cfg.DB, cfg.S3, fname); err != nil {
		logger.Error("backup failed", zap.Error(err))
		return
	}
	logger.Info("uploaded to s3", zap.String("object", fname), zap.String("cost", gutils.CostSecs(time.Since(start))))
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
	for _, t := range db.Tables {
		t = strings.TrimSpace(t)
		if t != "" {
			args = append(args, "-t", t)
		}
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
		return errors.Wrap(err, "start pg_dump")
	}

	// Stream pg_dump stdout into gzip writer
	if _, err := io.Copy(gz, stdout); err != nil {
		_ = cmd.Process.Kill() // ensure process terminated
		return errors.Wrap(err, "copy dump -> gzip")
	}
	if err := gz.Close(); err != nil {
		return errors.Wrap(err, "close gzip")
	}

	// Read stderr for logging if non-empty
	if bs, _ := io.ReadAll(stderr); len(bs) > 0 {
		glog.Shared.Named("pg_dump").Warn("stderr", zap.String("msg", string(bs)))
	}

	if err := cmd.Wait(); err != nil {
		return errors.Wrap(err, "wait pg_dump")
	}
	return nil
}

// streamBackupToS3 connects a pipe between pg_dump->gzip and S3 PutObject.
func streamBackupToS3(ctx context.Context, db cfgDB, s cfgS3, fname string) error {
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

	key := fname
	if p := strings.Trim(s.ObjectPrefix, "/"); p != "" {
		key = p + "/" + fname
	}

	// Size -1 enables streaming multipart upload
	_, putErr := s3cli.PutObject(ctx, s.Bucket, key, pr, -1, minio.PutObjectOptions{
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
	return nil
}

func bindPostgresBackupTask() {
	logger.Info("bind postgres backup task...")
	interval := loadCfg().IntervalSec
	if interval <= 0 {
		interval = 86400 // default 1 day
	}
	go store.TaskStore.TickerAfterRun(time.Duration(interval)*time.Second, runBackup)
}

func init() {
	store.TaskStore.Store("postgres_backup", bindPostgresBackupTask)
}
