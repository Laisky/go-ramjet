package s3

import (
	"context"
	"io"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"
)

// DefaultVersionsToKeep is the default number of most-recent object versions to
// retain for stable (reused) S3 keys.
//
// Some object stores (e.g. MinIO, which caps versions per object) reject writes
// with "You've exceeded the limit on the number of versions you can create on
// this object" once an object accumulates too many versions. Keeping only the
// most recent few versions avoids that cap while leaving a previous version as a
// safety net.
const DefaultVersionsToKeep = 2

// ObjectVersionClient is the subset of *minio.Client needed to cap object
// versions. *minio.Client satisfies it directly, so callers can pass their
// client as-is.
type ObjectVersionClient interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
}

// IsVersionLimitError reports whether err indicates the object store rejected a
// write because the object already holds the maximum number of versions.
func IsVersionLimitError(err error) bool {
	if err == nil {
		return false
	}

	resp := minio.ToErrorResponse(err)
	if resp.Code == "MaxVersionsExceeded" {
		return true
	}

	return strings.Contains(strings.ToLower(resp.Message),
		"limit on the number of versions you can create")
}

// KeepLatestObjectVersions removes historical versions of bucket/key, retaining
// only the keep most recently modified versions.
//
// It ignores delete markers and entries without a version id (e.g. the live
// object on an unversioned bucket) so those are never deleted, and is a no-op
// when keep <= 0.
func KeepLatestObjectVersions(
	ctx context.Context,
	logger glog.Logger,
	cli ObjectVersionClient,
	bucket, key string,
	keep int,
) error {
	if keep <= 0 {
		return nil
	}

	opts := minio.ListObjectsOptions{
		Prefix:       key,
		Recursive:    false,
		WithVersions: true,
	}

	versions := make([]minio.ObjectInfo, 0, keep+4)
	for object := range cli.ListObjects(ctx, bucket, opts) {
		if object.Err != nil {
			return errors.Wrap(object.Err, "list object versions")
		}
		if object.Key != key { // Prefix may match sibling keys; require exact match.
			continue
		}
		if object.VersionID == "" { // Unversioned bucket / live object; never delete.
			continue
		}
		if object.IsDeleteMarker {
			continue
		}
		versions = append(versions, object)
	}

	if len(versions) <= keep {
		return nil
	}

	sort.Slice(versions, func(i, j int) bool {
		if versions[i].LastModified.Equal(versions[j].LastModified) {
			return versions[i].VersionID > versions[j].VersionID
		}
		return versions[i].LastModified.After(versions[j].LastModified)
	})

	for _, version := range versions[keep:] {
		if err := cli.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{VersionID: version.VersionID}); err != nil {
			return errors.Wrapf(err, "remove object version %s", version.VersionID)
		}
		logger.Debug("removed old object version",
			zap.String("bucket", bucket),
			zap.String("object", key),
			zap.String("version", version.VersionID))
	}

	return nil
}

// PutObjectCappingVersions uploads reader to bucket/key while keeping the number
// of retained object versions at or below keep.
//
// It trims old versions before uploading to leave room for the version the
// upload creates, then trims once more afterwards so the retained set settles at
// keep. Trimming is best-effort: a trim failure is logged but does not fail the
// upload. If the upload is rejected because the object already holds the maximum
// number of versions and reader is an io.Seeker, it rewinds, trims again, and
// retries the upload once.
//
// keep <= 0 disables version trimming entirely; the upload is performed as a
// plain PutObject. That is the right choice for content-addressed keys, where a
// unique key per upload never accumulates versions and a version listing would
// be wasted work.
func PutObjectCappingVersions(
	ctx context.Context,
	logger glog.Logger,
	cli ObjectVersionClient,
	bucket, key string,
	reader io.Reader,
	size int64,
	opts minio.PutObjectOptions,
	keep int,
) (minio.UploadInfo, error) {
	if keep < 0 {
		keep = 0
	}

	// Leave room for the version the upcoming upload will create, so the
	// retained set settles at keep instead of keep+1.
	preTrimTarget := keep - 1
	if preTrimTarget < 0 {
		preTrimTarget = 0
	}

	if preTrimTarget > 0 {
		if err := KeepLatestObjectVersions(ctx, logger, cli, bucket, key, preTrimTarget); err != nil {
			logger.Warn("pre-trim object versions failed; uploading anyway",
				zap.String("bucket", bucket), zap.String("object", key), zap.Error(err))
		}
	}

	info, err := cli.PutObject(ctx, bucket, key, reader, size, opts)
	if err != nil && keep > 0 && IsVersionLimitError(err) {
		// A seekable source can be re-read, so trim again and retry once.
		if seeker, ok := reader.(io.Seeker); ok {
			if _, serr := seeker.Seek(0, io.SeekStart); serr == nil {
				logger.Warn("hit object version cap, trimming and retrying",
					zap.String("bucket", bucket), zap.String("object", key))
				if trimErr := KeepLatestObjectVersions(ctx, logger, cli, bucket, key, preTrimTarget); trimErr != nil {
					logger.Warn("trim after version-limit error failed",
						zap.String("bucket", bucket), zap.String("object", key), zap.Error(trimErr))
				}
				info, err = cli.PutObject(ctx, bucket, key, reader, size, opts)
			}
		}
	}
	if err != nil {
		return info, errors.Wrapf(err, "put object %q", key)
	}

	if keep > 0 {
		if err := KeepLatestObjectVersions(ctx, logger, cli, bucket, key, keep); err != nil {
			logger.Warn("post-trim object versions failed",
				zap.String("bucket", bucket), zap.String("object", key), zap.Error(err))
		}
	}

	return info, nil
}
