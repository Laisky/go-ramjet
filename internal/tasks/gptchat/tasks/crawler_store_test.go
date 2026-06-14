package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type redisSetCall struct {
	key        string
	value      any
	expiration time.Duration
}

type recordingRedisSetter struct {
	calls []redisSetCall
}

// Set records a Redis Set call with its key, value, and expiration, then returns a successful status command.
func (s *recordingRedisSetter) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	s.calls = append(s.calls, redisSetCall{
		key:        key,
		value:      value,
		expiration: expiration,
	})

	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("OK")
	return cmd
}

// TestSaveCrawlRecordPayloadSetsBoundedTTLs verifies crawler records are written with bounded Redis TTLs.
func TestSaveCrawlRecordPayloadSetsBoundedTTLs(t *testing.T) {
	client := &recordingRedisSetter{}
	record := &CrawlRecord{
		TaskID:  "task-1",
		URL:     "https://example.com/page",
		RawBody: []byte("<html></html>"),
	}

	err := saveCrawlRecordPayload(context.Background(), client, record, []byte(`{"ok":true}`))
	require.NoError(t, err)
	require.Len(t, client.calls, 2)

	require.Equal(t, crawlerRecordKey(record.TaskID), client.calls[0].key)
	require.Equal(t, crawlerDataTTL, client.calls[0].expiration)
	require.Equal(t, crawlerLatestKey(record.URL), client.calls[1].key)
	require.Equal(t, crawlerDataTTL, client.calls[1].expiration)
}
