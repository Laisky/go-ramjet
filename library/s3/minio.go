// Package s3 provides s3 client
package s3

import (
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	once sync.Once
	rw   sync.RWMutex
	cli  *minio.Client
	// errInit keeps the failure of the one-time client creation, so later calls
	// report it instead of returning a nil client with a nil error.
	errInit error
)

// GetCli returns the process-wide S3 client, creating it on the first call from
// endpoint, accessID and accessKey.
//
// The client is created once per process: later calls return it whatever arguments
// they pass. When that creation fails, this and every later call return the
// creation error and a nil client.
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

	once.Do(func() {
		cli, errInit = minio.New(
			endpoint,
			&minio.Options{
				Creds: credentials.NewStaticV4(
					accessID, accessKey, ""),
				Secure: true,
			},
		)
	})
	if errInit != nil {
		return nil, errors.Wrap(errInit, "new minio client")
	}

	return cli, nil
}
