package cv

import (
	"bytes"
	"context"
	"io"
	"path"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/minio/minio-go/v7"
)

// S3Client defines the minimal S3 client behavior used by CV storage.
type S3Client interface {
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error)
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
	ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
}

// S3ContentStore persists CV content in an S3-compatible object store.
type S3ContentStore struct {
	client         S3Client
	bucket         string
	contentKey     string
	defaultContent string
}

// S3PDFStore streams the CV PDF from an S3-compatible object store.
type S3PDFStore struct {
	client S3Client
	bucket string
	key    string
}

// minioClientAdapter adapts a minio client to the S3Client interface.
type minioClientAdapter struct {
	client *minio.Client
}

// NewMinioClientAdapter wraps a minio client with the S3Client interface.
func NewMinioClientAdapter(client *minio.Client) S3Client {
	return &minioClientAdapter{client: client}
}

// GetObject returns a reader for the requested S3 object.
func (m *minioClientAdapter) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error) {
	obj, err := m.client.GetObject(ctx, bucketName, objectName, opts)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// PutObject uploads an object to the S3 bucket.
func (m *minioClientAdapter) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return m.client.PutObject(ctx, bucketName, objectName, reader, objectSize, opts)
}

// StatObject retrieves metadata for the requested S3 object.
func (m *minioClientAdapter) StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	return m.client.StatObject(ctx, bucketName, objectName, opts)
}

// RemoveObject removes an object (or a specific version) from the S3 bucket.
func (m *minioClientAdapter) RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	return m.client.RemoveObject(ctx, bucketName, objectName, opts)
}

// ListObjects returns a channel of object info records for the provided bucket and options.
func (m *minioClientAdapter) ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	return m.client.ListObjects(ctx, bucketName, opts)
}

// NewS3ContentStore creates a content store backed by S3.
// It returns an error if required configuration is missing.
func NewS3ContentStore(client S3Client, bucket string, contentKey string, defaultContent string) (*S3ContentStore, error) {
	if client == nil {
		return nil, errors.WithStack(errors.New("s3 client is nil"))
	}
	if strings.TrimSpace(bucket) == "" {
		return nil, errors.WithStack(errors.New("s3 bucket is empty"))
	}
	if strings.TrimSpace(contentKey) == "" {
		return nil, errors.WithStack(errors.New("s3 content key is empty"))
	}

	return &S3ContentStore{
		client:         client,
		bucket:         bucket,
		contentKey:     contentKey,
		defaultContent: defaultContent,
	}, nil
}

// NewS3PDFStore creates a PDF store backed by S3.
// It returns an error if required configuration is missing.
func NewS3PDFStore(client S3Client, bucket string, key string) (*S3PDFStore, error) {
	if client == nil {
		return nil, errors.WithStack(errors.New("s3 client is nil"))
	}
	if strings.TrimSpace(bucket) == "" {
		return nil, errors.WithStack(errors.New("s3 bucket is empty"))
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.WithStack(errors.New("s3 pdf key is empty"))
	}

	return &S3PDFStore{
		client: client,
		bucket: bucket,
		key:    key,
	}, nil
}

// Load fetches CV content from S3, returning defaults when the object is missing.
func (s *S3ContentStore) Load(ctx context.Context) (payload ContentPayload, err error) {
	if err = ctx.Err(); err != nil {
		return ContentPayload{}, errors.Wrap(err, "context done")
	}

	info, statErr := s.client.StatObject(ctx, s.bucket, s.contentKey, minio.StatObjectOptions{})
	if statErr != nil {
		if isS3NoSuchKey(statErr) {
			return ContentPayload{
				Content:   s.defaultContent,
				UpdatedAt: nil,
				IsDefault: true,
			}, nil
		}
		return ContentPayload{}, errors.Wrap(statErr, "stat s3 content")
	}

	reader, err := s.client.GetObject(ctx, s.bucket, s.contentKey, minio.GetObjectOptions{})
	if err != nil {
		return ContentPayload{}, errors.Wrap(err, "get s3 content")
	}
	defer func() {
		if cerr := reader.Close(); cerr != nil && err == nil {
			err = errors.Wrap(cerr, "close s3 content reader")
		}
	}()

	body, err := io.ReadAll(reader)
	if err != nil {
		return ContentPayload{}, errors.Wrap(err, "read s3 content")
	}

	updatedAt := info.LastModified.UTC()
	return ContentPayload{
		Content:   string(body),
		UpdatedAt: &updatedAt,
		IsDefault: false,
	}, nil
}

// Save writes CV content to S3 and returns the updated metadata.
func (s *S3ContentStore) Save(ctx context.Context, content string) (ContentPayload, error) {
	if err := ctx.Err(); err != nil {
		return ContentPayload{}, errors.Wrap(err, "context done")
	}

	payload := []byte(content)
	_, err := s.client.PutObject(ctx,
		s.bucket,
		s.contentKey,
		bytes.NewReader(payload),
		int64(len(payload)),
		minio.PutObjectOptions{ContentType: "text/markdown; charset=utf-8"},
	)
	if err != nil {
		return ContentPayload{}, errors.Wrap(err, "put s3 content")
	}

	updatedAt := time.Now().UTC()
	return ContentPayload{
		Content:   content,
		UpdatedAt: &updatedAt,
		IsDefault: false,
	}, nil
}

// Open opens the CV PDF object from S3.
func (s *S3PDFStore) Open(ctx context.Context) (io.ReadCloser, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, errors.Wrap(err, "context done")
	}

	info, statErr := s.client.StatObject(ctx, s.bucket, s.key, minio.StatObjectOptions{})
	if statErr != nil {
		if isS3NoSuchKey(statErr) {
			return nil, 0, errors.WithStack(ErrObjectNotFound)
		}
		return nil, 0, errors.Wrap(statErr, "stat s3 pdf")
	}

	reader, err := s.client.GetObject(ctx, s.bucket, s.key, minio.GetObjectOptions{})
	if err != nil {
		if isS3NoSuchKey(err) {
			return nil, 0, errors.WithStack(ErrObjectNotFound)
		}
		return nil, 0, errors.Wrap(err, "get s3 pdf")
	}

	return reader, info.Size, nil
}

// Save uploads the CV PDF to S3.
func (s *S3PDFStore) Save(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "context done")
	}
	if len(payload) == 0 {
		return errors.WithStack(errors.New("pdf payload is empty"))
	}

	putErr := s.putPDFObject(ctx, payload)
	if putErr == nil {
		return nil
	}
	if !isVersionLimitErr(putErr) {
		return errors.Wrap(putErr, "put s3 pdf")
	}

	if err := s.deletePDFVersions(ctx); err != nil {
		return errors.Wrap(err, "delete pdf versions")
	}

	putErr = s.putPDFObject(ctx, payload)
	if putErr != nil {
		return errors.Wrap(putErr, "put s3 pdf after deleting versions")
	}

	return nil
}

// putPDFObject uploads the PDF payload to S3.
// It takes a context and payload bytes and returns an error if the upload fails.
func (s *S3PDFStore) putPDFObject(ctx context.Context, payload []byte) error {
	_, err := s.client.PutObject(ctx,
		s.bucket,
		s.key,
		bytes.NewReader(payload),
		int64(len(payload)),
		minio.PutObjectOptions{
			ContentType:  "application/pdf",
			CacheControl: cvPDFCacheControl,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// deletePDFVersions removes all known versions for the PDF object when versioning is enabled.
// It takes a context and returns an error when the cleanup fails.
func (s *S3PDFStore) deletePDFVersions(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "context done")
	}
	if s.client == nil {
		return errors.WithStack(errors.New("s3 client is nil"))
	}

	opts := minio.ListObjectsOptions{
		Prefix:       s.key,
		Recursive:    true,
		WithVersions: true,
	}

	for obj := range s.client.ListObjects(ctx, s.bucket, opts) {
		if obj.Err != nil {
			return errors.Wrap(obj.Err, "list pdf versions")
		}
		if obj.Key != s.key {
			continue
		}
		if strings.TrimSpace(obj.VersionID) == "" {
			continue
		}
		if err := s.client.RemoveObject(ctx, s.bucket, s.key, minio.RemoveObjectOptions{VersionID: obj.VersionID}); err != nil {
			return errors.Wrap(err, "remove pdf version")
		}
	}

	return nil
}

// isVersionLimitErr reports whether err is related to exceeding object version limits.
// It takes an error and returns true when the error indicates a version limit issue.
func isVersionLimitErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "limit on the number of versions")
}

// ErrObjectNotFound indicates an expected object is missing.
var ErrObjectNotFound = errors.New("object not found")

// isS3NoSuchKey checks whether an error represents a missing S3 object.
func isS3NoSuchKey(err error) bool {
	if err == nil {
		return false
	}

	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		if resp.Code == "NoSuchKey" || resp.StatusCode == 404 {
			return true
		}
		msg := strings.ToLower(resp.Message)
		if strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such key") {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such key")
}

// joinObjectKey joins an optional prefix with a key name.
func joinObjectKey(prefix string, name string) string {
	trimmedPrefix := strings.Trim(prefix, "/")
	trimmedName := strings.Trim(name, "/")
	if trimmedPrefix == "" {
		return trimmedName
	}
	return path.Join(trimmedPrefix, trimmedName)
}
