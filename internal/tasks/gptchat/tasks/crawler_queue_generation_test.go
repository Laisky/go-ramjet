package tasks

import (
	"context"
	"sync"
	"testing"
	"time"

	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const legacyCrawlerQueueFixture = "laisky/tasks/html_crawler/pending"

// crawlerQueueFixture implements the list commands used by producer and consumer without live Redis.
// Embedding Cmdable keeps the production interface exact; unused commands cannot silently succeed.
type crawlerQueueFixture struct {
	redis.Cmdable
	mu       sync.Mutex
	lists    map[string][]string
	values   map[string]string
	consumed []string
}

// Set records a serialized result snapshot for key and returns the Redis status result.
func (f *crawlerQueueFixture) Set(_ context.Context, key string, value any, _ time.Duration) *redis.StatusCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] = value.(string)
	return redis.NewStatusResult("OK", nil)
}

// RPush appends serialized values only to key and returns its resulting list length.
func (f *crawlerQueueFixture) RPush(_ context.Context, key string, values ...any) *redis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, value := range values {
		f.lists[key] = append(f.lists[key], value.(string))
	}
	return redis.NewIntResult(int64(len(f.lists[key])), nil)
}

// BLPop atomically removes the first value from the first populated supplied key.
// Empty fixture lists immediately return redis.Nil instead of spending timeout in a live connection.
func (f *crawlerQueueFixture) BLPop(_ context.Context, _ time.Duration, keys ...string) *redis.StringSliceCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consumed = append(f.consumed, keys...)
	for _, key := range keys {
		if len(f.lists[key]) == 0 {
			continue
		}
		value := f.lists[key][0]
		f.lists[key] = f.lists[key][1:]
		return redis.NewStringSliceResult([]string{key, value}, nil)
	}
	return redis.NewStringSliceResult(nil, redis.Nil)
}

// TestHTMLCrawlerVerifiedQueueMixedFleet reproduces legacy consumer theft of a modern serialized task.
// The secure producer and consumer must share a distinct queue while old pending entries remain untouched.
func TestHTMLCrawlerVerifiedQueueMixedFleet(t *testing.T) {
	ctx := context.Background()
	f := &crawlerQueueFixture{lists: map[string][]string{}, values: map[string]string{}}
	task := verifiedTestTask()
	require.NoError(t, enqueueHTMLCrawlerTaskUsingClient(ctx, task, f))

	// An older process already blocked on its compiled legacy key cannot receive this modern task.
	_, err := f.BLPop(ctx, time.Second, legacyCrawlerQueueFixture).Result()
	require.ErrorIs(t, err, redis.Nil, "legacy workers must never receive verified tasks")
	require.Equal(t, "laisky/tasks/html_crawler/verified/v1/pending", rlibs.KeyTaskHTMLCrawlerPending)
	require.NotEqual(t, legacyCrawlerQueueFixture, rlibs.KeyTaskHTMLCrawlerPending)

	legacyPayload := `{"task_id":"legacy-fixture","status":"pending"}`
	require.NoError(t, f.RPush(ctx, legacyCrawlerQueueFixture, legacyPayload).Err())
	received, err := popHTMLCrawlerTaskUsingClient(ctx, f)
	require.NoError(t, err)
	require.Equal(t, task.TaskID, received.TaskID)
	require.Equal(t, task.Url, received.Url)
	require.Equal(t, task.OutputMarkdown, received.OutputMarkdown)
	require.Equal(t, task.Egress, received.Egress)
	require.Equal(t, rlibs.TaskStatusRunning, received.Status)
	require.Equal(t, []string{legacyPayload}, f.lists[legacyCrawlerQueueFixture])
	require.Equal(t, []string{legacyCrawlerQueueFixture, rlibs.KeyTaskHTMLCrawlerPending}, f.consumed)

	// The initial result remains in the existing UUID result namespace and retains the policy.
	snapshot, err := rlibs.NewHTMLCrawlerTaskFromString(f.values[crawlerResultKey(task.TaskID)])
	require.NoError(t, err)
	require.Equal(t, task.Egress, snapshot.Egress)
	require.Equal(t, task.TaskID, snapshot.TaskID)
	_, err = popHTMLCrawlerTaskUsingClient(ctx, f)
	require.ErrorIs(t, err, redis.Nil, "secure consumer must not fall back to pending legacy work")
	require.Equal(t, []string{legacyPayload}, f.lists[legacyCrawlerQueueFixture])
}
