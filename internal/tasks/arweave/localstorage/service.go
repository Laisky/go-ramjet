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
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/internal/tasks/arweave/config"
	gptTasks "github.com/Laisky/go-ramjet/internal/tasks/gptchat/tasks"
	"github.com/Laisky/go-ramjet/library/egress"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	// httpcli talks only to the configured S3 endpoint, never to user-selected targets.
	httpcli     *http.Client
	runningUrls sync.Map
	taskChan    = make(chan taskItem, 20)

	// targetNetwork supplies the resolver and dialer used for user-selected targets; tests replace it.
	targetNetwork egress.Dependencies
	// targetHTTPClient fetches user-selected targets; every hop, including redirects, is admitted and pinned.
	targetHTTPClient = egress.NewHTTPClient(30*time.Second, targetNetwork)
	// fetchArchiveContent renders one admitted target through the verified crawler.
	fetchArchiveContent = func(ctx context.Context, target string) ([]byte, error) {
		return gptTasks.FetchDynamicURLContent(ctx, target, gptTasks.WithMarkdownConversion("", false))
	}
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

// LoadContentByUrl loads a user-selected url through the shared destination policy.
// Every hop, including redirects, must be a public destination; refusals carry an
// *egress.AdmissionError in the returned chain.
//
// Note: caller should close the returned io.ReadCloser
func LoadContentByUrl(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "new request %s", url)
	}

	resp, err := targetHTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "do request %s", url)
	}

	if resp.StatusCode != http.StatusOK {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, errors.Wrapf(closeErr, "close unexpected status %d response for url %s", resp.StatusCode, url)
		}
		return nil, errors.Errorf("unexpected status code %d for url %s", resp.StatusCode, url)
	}

	return resp.Body, nil
}

// LoadContentFromS3 load content by url
//
// Note: caller should close the returned io.ReadCloser
func LoadContentFromS3(ctx context.Context, url string) (io.ReadCloser, error) {
	objUrl := fmt.Sprintf("https://%s/%s/%s",
		config.Instance.S3.Endpoint,
		config.Instance.S3.Bucket,
		objkey2path(url2objkey(url)),
	)
	return openArchivedObject(ctx, httpcli, objUrl)
}

// openArchivedObject GETs one archived object from objUrl with client and returns
// its body when the archive answers 200. Any other answer, including the 404 of
// an archive miss, is closed before the error is returned.
//
// Note: caller should close the returned io.ReadCloser
func openArchivedObject(ctx context.Context, client *http.Client, objUrl string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objUrl, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "new request %s", objUrl)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "do request %s", objUrl)
	}

	if resp.StatusCode != http.StatusOK {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, errors.Wrapf(closeErr, "close unexpected status %d response for url %s", resp.StatusCode, objUrl)
		}
		return nil, errors.Errorf("unexpected status code %d for url %s", resp.StatusCode, objUrl)
	}

	return resp.Body, nil
}

// RunSaveUrlContent run save url content
func RunSaveUrlContent(ctx context.Context) {
	for i := 0; i < 2; i++ {
		go func() {
			for task := range taskChan {
				saveURLContentTask(ctx, task)
			}
		}()
	}
}

// saveURLContentTask processes one queued archive item and records a single attributed failure.
// It preserves the three-minute fetch/storage budget and the existing duplicate suppression.
// Destination-policy refusals are input errors and are logged at WARN, below the alert level;
// every other failure, including resolver failures, remains an ERROR alert.
func saveURLContentTask(ctx context.Context, task taskItem) {
	if _, ok := runningUrls.Load(task.url); ok {
		return
	}
	runningUrls.Store(task.url, struct{}{})

	startedAt := time.Now()
	stage := "fetch"
	err := func() error {
		defer runningUrls.Delete(task.url)

		taskCtx, cancel := context.WithTimeout(ctx, time.Minute*3)
		defer cancel()

		content, err := fetchArchiveContent(taskCtx, task.url)
		if err != nil {
			return errors.Wrapf(err, "fetch url %s", task.url)
		}

		stage = "save"
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
		fields := []zap.Field{
			zap.String("stage", stage),
			zap.Duration("duration", time.Since(startedAt)),
			zap.String("url", task.url),
			zap.String("target_ref", log.ArchiveTargetRef(task.url)),
			zap.Error(err),
		}
		if egress.IsExpectedRefusal(err) {
			log.ArchiveLogger().Warn("failed to fetch and save url content", fields...)
			return
		}
		log.ArchiveLogger().Error("failed to fetch and save url content", fields...)
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
