// Package s3 provides s3 client
package s3

import (
	"sync"

	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	once sync.Once
	rw   sync.RWMutex
	cli  *minio.Client
)

// GetCli get s3 client
func GetCli() *minio.Client {
	rw.RLock()
	if cli != nil {
		rw.RUnlock()
		return cli
	}

	rw.RUnlock()
	rw.Lock()
	defer rw.Unlock()
	if cli != nil { // double check
		return cli
	}

	once.Do(func() {
		var err error
		if cli, err = minio.New(
			config.Config.S3.Endpoint,
			&minio.Options{
				Creds: credentials.NewStaticV4(
					config.Config.S3.AccessID, config.Config.S3.AccessKey, ""),
				Secure: true,
			},
		); err != nil {
			log.Logger.Panic("new s3 client", zap.Error(err))
		}
	})

	return cli
}
