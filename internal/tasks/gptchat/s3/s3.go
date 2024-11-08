// Package s3 provides s3 client
package s3

import (
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/s3"
)

// GetCli get s3 client
func GetCli() (*minio.Client, error) {
	return s3.GetCli(
		config.Config.S3.Endpoint,
		config.Config.S3.AccessID,
		config.Config.S3.AccessKey,
	)
}
