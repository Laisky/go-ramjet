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
	// isArchived reports whether a url already has an archived copy; tests replace it.
	isArchived = isUrlFinished
	// storeArchiveContent writes one rendered page to its archive object; tests replace it.
	storeArchiveContent = func(ctx context.Context, objkey string, content []byte) error {
		opt := minio.PutObjectOptions{}
		opt.Header().Add("Content-Type", "text/html")
		_, err := config.Instance.S3Cli.PutObject(ctx,
			config.Instance.S3.Bucket,
			objkey2path(objkey),
			bytes.NewReader(content),
			int64(len(content)),
			opt,
		)
		return errors.WithStack(err)
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

// SaveUrlContent queues url for archiving unless it is already pending or archived.
//
// A url is marked pending atomically before its archive lookup and stays marked
// while it is queued and until its task ends. Concurrent requests for one url
// therefore run one lookup and queue it at most once, instead of each issuing
// its own S3 HEAD or filling the shared queue with duplicates. Tasks record a
// successful archive before releasing the mark, so a caller that takes the mark
// afterwards sees the archive. It returns an error when the lookup fails or the
// queue is full (errArchiveQueueFull); a url that is not queued is not left marked.
// Every outcome is counted in archiveScheduleOutcomes.
func SaveUrlContent(ctx context.Context, item taskItem) error {
	logger := gmw.GetLogger(ctx).With(zap.String("url", item.url))

	if _, pending := runningUrls.LoadOrStore(item.url, struct{}{}); pending {
		archiveScheduleOutcomes.Add(schedulePending, 1)
		logger.Debug("url is running, skip")
		return nil
	}

	finished, err := isArchived(ctx, item.url)
	if err != nil {
		runningUrls.Delete(item.url)
		archiveScheduleOutcomes.Add(scheduleLookupFailed, 1)
		return errors.Wrapf(err, "check url %s", item.url)
	}
	if finished {
		runningUrls.Delete(item.url)
		archiveScheduleOutcomes.Add(scheduleArchived, 1)
		return nil
	}

	select {
	case taskChan <- item:
		archiveScheduleOutcomes.Add(scheduleQueued, 1)
		return nil
	default:
		runningUrls.Delete(item.url)
		archiveScheduleOutcomes.Add(scheduleQueueFull, 1)
		return errors.WithStack(errArchiveQueueFull)
	}
}

// LoadContentByUrl loads a user-selected url through the shared destination policy.
// Every hop, including redirects, must be a public destination; refusals carry an
// *egress.AdmissionError in the returned chain.
//
// It advertises gzip, as net/http's default transport does, and returns the
// decoded representation: the proxy relays the body without upstream headers, so
// encoded bytes would be unreadable. A content coding that cannot be decoded is
// refused with *egress.UnsupportedEncodingError, so another source can answer.
//
// Note: caller should close the returned io.ReadCloser
func LoadContentByUrl(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "new request %s", url)
	}
	req.Header.Set("Accept-Encoding", egress.AcceptEncoding)

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

	body, err := egress.DecodeContent(resp.Header, resp.Body)
	if err != nil {
		return nil, errors.Wrapf(errors.Join(err, resp.Body.Close()), "decode response for url %s", url)
	}
	return body, nil
}

// LoadContentFromS3 load content by url
//
// Note: caller should close the returned io.ReadCloser
func LoadContentFromS3(ctx context.Context, url string) (io.ReadCloser, error) {
	return openArchivedObject(ctx, httpcli, archiveObjectURL(url))
}

// archiveObjectURL returns the configured S3 URL of target's archived copy.
func archiveObjectURL(target string) string {
	return fmt.Sprintf("https://%s/%s/%s",
		config.Instance.S3.Endpoint,
		config.Instance.S3.Bucket,
		objkey2path(url2objkey(target)),
	)
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
// It preserves the three-minute fetch/storage budget. SaveUrlContent marks the url pending
// when queuing it; the task releases that mark when it ends and records a successful archive
// so the url is not queued again. Destination-policy refusals are input errors and are logged
// at WARN, below the alert level; every other failure, including resolver failures, remains
// an ERROR alert.
func saveURLContentTask(ctx context.Context, task taskItem) {
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
		if err = storeArchiveContent(taskCtx, objkey, content); err != nil {
			return errors.Wrapf(err, "save url %s", task.url)
		}
		isUrlFinishedCache.Store(task.url, true)

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

	objUrl := archiveObjectURL(url)
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
