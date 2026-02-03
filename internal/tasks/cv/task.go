// Package cv provides the CV task for managing resume content.
package cv

import (
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/s3"
)

const (
	defaultS3Prefix  = "private/cv"
	defaultS3Content = "cv.md"
	defaultS3PDF     = "cv.pdf"
)

type s3Config struct {
	enable       bool
	endpoint     string
	accessID     string
	accessSecret string
	bucket       string
	prefix       string
	contentKey   string
	pdfKey       string
}

// normalizeS3Endpoint converts an endpoint into a host without scheme.
func normalizeS3Endpoint(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimRight(trimmed, "/")
	if trimmed == "" {
		return "", errors.WithStack(errors.New("s3 endpoint is empty"))
	}
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", errors.Wrap(err, "parse s3 endpoint")
		}
		if parsed.Host == "" {
			return "", errors.WithStack(errors.New("s3 endpoint host is empty"))
		}
		return parsed.Host, nil
	}

	return trimmed, nil
}

// loadS3Config reads CV S3 configuration from shared config.
func loadS3Config() (s3Config, error) {
	cfg := s3Config{
		enable:       gconfig.Shared.GetBool("tasks.cv.s3.enable"),
		endpoint:     gconfig.Shared.GetString("tasks.cv.s3.endpoint"),
		accessID:     gconfig.Shared.GetString("tasks.cv.s3.access_key"),
		accessSecret: gconfig.Shared.GetString("tasks.cv.s3.access_secret"),
		bucket:       gconfig.Shared.GetString("tasks.cv.s3.bucket"),
		prefix:       gconfig.Shared.GetString("tasks.cv.s3.prefix"),
		contentKey:   gconfig.Shared.GetString("tasks.cv.s3.content_key"),
		pdfKey:       gconfig.Shared.GetString("tasks.cv.s3.pdf_key"),
	}

	if !cfg.enable {
		return cfg, errors.WithStack(errors.New("tasks.cv.s3.enable must be true"))
	}

	if cfg.prefix == "" {
		cfg.prefix = defaultS3Prefix
	}
	if cfg.contentKey == "" {
		cfg.contentKey = defaultS3Content
	}
	if cfg.pdfKey == "" {
		cfg.pdfKey = defaultS3PDF
	}

	if cfg.bucket == "" {
		return cfg, errors.WithStack(errors.New("tasks.cv.s3.bucket is empty"))
	}
	if cfg.accessID == "" || cfg.accessSecret == "" {
		return cfg, errors.WithStack(errors.New("tasks.cv.s3 access key or secret is empty"))
	}

	endpoint, err := normalizeS3Endpoint(cfg.endpoint)
	if err != nil {
		return cfg, err
	}
	cfg.endpoint = endpoint

	return cfg, nil
}

// bindTask registers the CV HTTP routes.
func bindTask() {
	cfg, err := loadS3Config()
	if err != nil {
		log.Logger.Panic("load cv s3 config", zap.Error(err))
	}

	cli, err := s3.GetCli(cfg.endpoint, cfg.accessID, cfg.accessSecret)
	if err != nil {
		log.Logger.Panic("create cv s3 client", zap.Error(err))
	}

	s3Client := NewMinioClientAdapter(cli)
	contentKey := joinObjectKey(cfg.prefix, cfg.contentKey)
	pdfKey := joinObjectKey(cfg.prefix, cfg.pdfKey)

	s3Store, err := NewS3ContentStore(s3Client, cfg.bucket, contentKey, defaultCVMarkdown)
	if err != nil {
		log.Logger.Panic("create cv s3 content store", zap.Error(err))
	}

	pdfStore, err := NewS3PDFStore(s3Client, cfg.bucket, pdfKey)
	if err != nil {
		log.Logger.Panic("create cv s3 pdf store", zap.Error(err))
	}

	pdfRenderer, err := NewCVPDFRenderer()
	if err != nil {
		log.Logger.Panic("create cv pdf renderer", zap.Error(err))
	}

	pdfService, err := NewPDFService(pdfRenderer, pdfStore)
	if err != nil {
		log.Logger.Panic("create cv pdf service", zap.Error(err))
	}

	bindHTTP(s3Store, pdfStore, pdfService)
	log.Logger.Info("bind cv task",
		zap.String("bucket", cfg.bucket),
		zap.String("prefix", cfg.prefix),
		zap.String("content_key", contentKey),
		zap.String("pdf_key", pdfKey))
}

// init registers the CV task with the task store.
func init() {
	store.TaskStore.Store("cv", bindTask)
}
