// Package s3 provides s3 client
package s3

import (
	"bytes"
	"context"
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/go-ramjet/library/s3"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

func TestGetCli(t *testing.T) {
	ctx := context.Background()

	err := gconfig.S.LoadFromFile("/opt/configs/go-ramjet/settings.yml")
	require.NoError(t, err)

	cli, err := s3.GetCli(
		gconfig.S.GetString("tasks.blog.rss.upload_to_s3.endpoint"),
		gconfig.S.GetString("tasks.blog.rss.upload_to_s3.access_key"),
		gconfig.S.GetString("tasks.blog.rss.upload_to_s3.access_secret"),
	)
	require.NoError(t, err)

	t.Run("upload", func(t *testing.T) {
		payload := []byte("hello")
		_, err := cli.PutObject(ctx,
			"public",
			"test-upload",
			bytes.NewReader(payload),
			int64(len(payload)),
			minio.PutObjectOptions{
				ContentType: "text/plain",
			},
		)
		require.NoError(t, err)
	})
}
