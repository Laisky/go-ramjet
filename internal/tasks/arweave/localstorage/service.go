package localstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/internal/tasks/arweave/config"
	gpthttp "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
	"github.com/Laisky/go-ramjet/library/log"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"
)

var (
	httpcli     *http.Client
	runningUrls sync.Map
	taskChan    = make(chan taskItem, 1)
)

func init() {
	var err error
	httpcli, err = gutils.NewHTTPClient(
		gutils.WithHTTPClientTimeout(30 * time.Second),
	)
	if err != nil {
		log.Logger.Panic("new http client", zap.Error(err))
	}
}

type taskItem struct {
	url string
}

// SaveUrlContent save url content to s3
func SaveUrlContent(ctx context.Context, item taskItem) error {
	logger := gmw.GetLogger(ctx).With(zap.String("url", item.url))

	// check is running
	if _, ok := runningUrls.Load(item.url); ok {
		logger.Debug("url is running, skip")
		return nil
	}

	// check is finished
	if finished, err := isUrlFinished(ctx, item.url); err != nil {
		return errors.Wrapf(err, "check url %s", item.url)
	} else if finished {
		return nil
	}

	// put task
	select {
	case taskChan <- item:
	default:
		return errors.New("taskChan is full, please wait for a while")
	}

	return nil
}

// LoadContentByUrl load content by url
//
// Note: caller should close the returned io.ReadCloser
func LoadContentByUrl(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "new request %s", url)
	}

	resp, err := httpcli.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "do request %s", url)
	}
	// defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("unexpected status code %d for url %s", resp.StatusCode, url)
	}

	return resp.Body, nil
}

// LoadContentFromS3 load content by url
//
// Note: caller should close the returned io.ReadCloser
func LoadContentFromS3(ctx context.Context, url string) (io.ReadCloser, error) {
	objkey := url2objkey(url)
	objUrl := fmt.Sprintf("https://%s/%s/%s",
		config.Instance.S3.Endpoint,
		config.Instance.S3.Bucket,
		objkey2path(objkey),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objUrl, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "new request %s", objUrl)
	}

	resp, err := httpcli.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "do request %s", objUrl)
	}
	// defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("unexpected status code %d for url %s", resp.StatusCode, objUrl)
	}

	return resp.Body, nil
}

// RunSaveUrlContent run save url content
func RunSaveUrlContent(ctx context.Context) {
	for i := 0; i < 2; i++ {
		go func() {
			for task := range taskChan {
				if _, ok := runningUrls.Load(task.url); ok {
					continue
				}
				runningUrls.Store(task.url, struct{}{})

				err := func() error {
					defer runningUrls.Delete(task.url)

					taskCtx, cancel := context.WithTimeout(ctx, time.Minute*3)
					defer cancel()

					content, err := gpthttp.FetchDynamicURLContent(taskCtx,
						task.url,
						gpthttp.WithDuration(time.Second*30),
					)
					if err != nil {
						return errors.Wrapf(err, "fetch url %s", task.url)
					}

					objkey := url2objkey(task.url)
					opt := minio.PutObjectOptions{}
					opt.Header().Add("Content-Type", "text/html")
					_, err = config.Instance.S3Cli.PutObject(taskCtx,
						config.Instance.S3.Bucket,
						objkey2path(objkey),
						bytes.NewReader(content),
						int64(len(content)),
						opt,
					)
					if err != nil {
						return errors.Wrapf(err, "save url %s", task.url)
					}

					log.Logger.Info("save url content",
						zap.String("url", task.url),
						zap.String("objkey", objkey))
					return nil
				}()
				if err != nil {
					log.Logger.Error("failed to fetch and save url content",
						zap.String("url", task.url),
						zap.Error(err),
					)
				}
			}
		}()
	}
}

var isUrlFinishedCache = gutils.NewExpCache[bool](context.Background(), time.Minute*10)

// isUrlFinished is a helper to check if url is finished
func isUrlFinished(ctx context.Context, url string) (bool, error) {
	// check cache
	if res, ok := isUrlFinishedCache.Load(url); ok {
		return res, nil
	}

	objUrl := fmt.Sprintf("https://%s/%s/%s",
		config.Instance.S3.Endpoint,
		config.Instance.S3.Bucket,
		objkey2path(url2objkey(url)),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, objUrl, nil)
	if err != nil {
		return false, errors.Wrapf(err, "new request %s", objUrl)
	}

	resp, err := httpcli.Do(req)
	if err != nil {
		return false, errors.Wrapf(err, "do request %s", objUrl)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		// update cache
		isUrlFinishedCache.Store(url, true)

		return true, nil
	} else if resp.StatusCode != http.StatusNotFound {
		return false, errors.Errorf("unexpected status code %d for url %s", resp.StatusCode, objUrl)
	}

	return false, nil
}

func url2objkey(url string) string {
	hashed := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hashed[:])
}

func objkey2path(objkey string) string {
	return fmt.Sprintf("arweave/urlcache/%s/%s/%s", objkey[:2], objkey[2:4], objkey)
}
