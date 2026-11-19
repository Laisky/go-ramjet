package localstorage

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

// useArchiveSeams replaces the archive lookup, fetch and store for one test, routes
// archive logs to a local observer, and starts from an empty queue and pending set.
func useArchiveSeams(t *testing.T, archived func(context.Context, string) (bool, error),
	fetch func(context.Context, string) ([]byte, error), store func(context.Context, string, []byte) error) *observer.ObservedLogs {
	t.Helper()
	useArchiveTestConfig()
	requireNoProxyGoroutines(t)
	local, entries := observer.New(zapcore.DebugLevel)
	logger, err := glog.New(glog.WithName("go-ramjet"), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return local
	})))
	require.NoError(t, err)

	oldLogger, oldArchived, oldFetch, oldStore := log.Logger, isArchived, fetchArchiveContent, storeArchiveContent
	log.Logger, isArchived, fetchArchiveContent, storeArchiveContent = logger, archived, fetch, store
	drainArchiveTasks()
	runningUrls.Clear()
	t.Cleanup(func() {
		drainArchiveTasks()
		runningUrls.Clear()
		log.Logger, isArchived, fetchArchiveContent, storeArchiveContent = oldLogger, oldArchived, oldFetch, oldStore
	})
	return entries
}

// notArchived reports every url as missing from the archive.
func notArchived(context.Context, string) (bool, error) { return false, nil }

// TestSaveUrlContentQueuesEachURLOnce guards against the check-then-act race
// between "is it running?" and "queue it": concurrent proxy requests for one
// url must queue it once, instead of filling the shared queue with duplicates
// and refusing unrelated urls with "taskChan is full".
func TestSaveUrlContentQueuesEachURLOnce(t *testing.T) {
	useArchiveSeams(t, notArchived, nil, nil)

	const callers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	var failures atomic.Int32
	for range callers {
		wg.Go(func() {
			<-start
			if err := SaveUrlContent(context.Background(), taskItem{url: "https://public.example/page"}); err != nil {
				failures.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	require.Zero(t, failures.Load(), "duplicate requests must not be refused as a full queue")
	require.Equal(t, 1, drainArchiveTasks(), "a url must be queued at most once while pending")
}

// TestSaveUrlContentRequeuesAfterTaskOrFullQueue releases a url's pending mark
// when its task ends or when it could not be queued, so it can be archived later.
func TestSaveUrlContentRequeuesAfterTaskOrFullQueue(t *testing.T) {
	useArchiveSeams(t, notArchived, func(context.Context, string) ([]byte, error) {
		return nil, errors.New("synthetic fetch failure")
	}, nil)
	const target = "https://public.example/page"

	require.NoError(t, SaveUrlContent(context.Background(), taskItem{url: target}))
	require.NoError(t, SaveUrlContent(context.Background(), taskItem{url: target}), "a pending url is skipped, not refused")
	saveURLContentTask(context.Background(), <-taskChan)
	require.NoError(t, SaveUrlContent(context.Background(), taskItem{url: target}), "a failed task releases its url")
	require.Equal(t, 1, drainArchiveTasks())
	runningUrls.Clear()

	for i := range cap(taskChan) {
		require.NoError(t, SaveUrlContent(context.Background(), taskItem{url: target + "?fill=" + string(rune('a'+i))}))
	}
	require.Error(t, SaveUrlContent(context.Background(), taskItem{url: target}), "a full queue still refuses new urls")
	drainArchiveTasks()
	require.NoError(t, SaveUrlContent(context.Background(), taskItem{url: target}), "a refused url is not left marked as pending")
	require.Equal(t, 1, drainArchiveTasks())
}

// TestSaveURLContentTaskMarksArchived records a successful archive so that the
// same url is neither queued nor fetched again, even before the archive lookup
// would observe the new object.
func TestSaveURLContentTaskMarksArchived(t *testing.T) {
	var fetches, stores atomic.Int32
	const target = "https://public.example/archived-once"
	isUrlFinishedCache.Delete(target)
	t.Cleanup(func() { isUrlFinishedCache.Delete(target) })
	useArchiveSeams(t,
		func(ctx context.Context, raw string) (bool, error) {
			if finished, ok := isUrlFinishedCache.Load(raw); ok {
				return finished, nil
			}
			return false, nil
		},
		func(context.Context, string) ([]byte, error) {
			fetches.Add(1)
			return []byte("<html>archived</html>"), nil
		},
		func(_ context.Context, objkey string, content []byte) error {
			stores.Add(1)
			require.Equal(t, url2objkey(target), objkey)
			require.Equal(t, "<html>archived</html>", string(content))
			return nil
		})

	require.NoError(t, SaveUrlContent(context.Background(), taskItem{url: target}))
	saveURLContentTask(context.Background(), <-taskChan)
	require.NoError(t, SaveUrlContent(context.Background(), taskItem{url: target}))
	require.Zero(t, drainArchiveTasks(), "an archived url must not be queued again")
	require.EqualValues(t, 1, fetches.Load())
	require.EqualValues(t, 1, stores.Load())
}
