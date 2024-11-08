// Package s3 provides s3 client
package s3

import (
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	once sync.Once
	rw   sync.RWMutex
	cli  *minio.Client
)

// GetCli get s3 client
func GetCli(
	endpoint, accessID, accessKey string,
) (*minio.Client, error) {
	rw.RLock()
	if cli != nil {
		rw.RUnlock()
		return cli, nil
	}

	rw.RUnlock()
	rw.Lock()
	defer rw.Unlock()
	if cli != nil { // double check
		return cli, nil
	}

	var err error
	once.Do(func() {
		cli, err = minio.New(
			endpoint,
			&minio.Options{
				Creds: credentials.NewStaticV4(
					accessID, accessKey, ""),
				Secure: true,
			},
		)
	})

	return cli, err
}
