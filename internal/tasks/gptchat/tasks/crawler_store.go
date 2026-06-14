package tasks

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/zap"
	"github.com/redis/go-redis/v9"

	rutils "github.com/Laisky/go-ramjet/library/redis"
)

const (
	crawlerURLLatestKeyPrefix = "ramjet:gptchat:crawler:html:latest:"
	crawlerRecordKeyPrefix    = "ramjet:gptchat:crawler:html:record:"
	crawlerDataTTL            = 3 * time.Hour
)

// redisSetter stores a value in Redis with an expiration.
// It takes a context, key, value, and retention duration, and returns the Redis status command.
type redisSetter interface {
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
}

// CrawlRecord is the persisted crawl result for a URL.
//
// It is stored in Redis for caching and later retrieval.
type CrawlRecord struct {
	TaskID       string    `json:"task_id"`
	CrawledAt    time.Time `json:"crawled_at"`
	APIKeyPrefix string    `json:"api_key_prefix"`
	URL          string    `json:"url"`
	RawBody      []byte    `json:"raw_body"`
	Markdown     *string   `json:"markdown,omitempty"`
}

// apiKeyPrefix returns the first 9 characters of the api key.
func apiKeyPrefix(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}

	if len(apiKey) <= 9 {
		return apiKey
	}

	return apiKey[:9]
}

// urlHash takes a URL string and returns a stable SHA-1 hash for Redis cache keys.
func urlHash(url string) string {
	h := sha1.Sum([]byte(url))
	return hex.EncodeToString(h[:])
}

// crawlerLatestKey takes a URL and returns the Redis key that stores its latest crawl record.
func crawlerLatestKey(url string) string {
	return crawlerURLLatestKeyPrefix + urlHash(url)
}

// crawlerRecordKey takes a task ID and returns the Redis key that stores its crawl record.
func crawlerRecordKey(taskID string) string {
	return crawlerRecordKeyPrefix + taskID
}

// LoadLatestCrawlRecord loads the newest cached crawl record for the URL.
func LoadLatestCrawlRecord(ctx context.Context, url string) (*CrawlRecord, bool, error) {
	client := rutils.GetCli().GetDB().Client
	key := crawlerLatestKey(url)

	payload, err := client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, false, errors.WithStack(err)
		}
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, errors.Wrap(err, "load latest crawl record")
	}

	var record CrawlRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return nil, false, errors.Wrap(err, "unmarshal crawl record")
	}

	return &record, true, nil
}

// SaveCrawlRecord persists the crawl record and updates the latest record for the URL.
func SaveCrawlRecord(ctx context.Context, record *CrawlRecord) error {
	if record == nil {
		return errors.New("record is nil")
	}
	if record.TaskID == "" {
		return errors.New("taskID is empty")
	}
	if record.URL == "" {
		return errors.New("url is empty")
	}

	logger := gmw.GetLogger(ctx).Named("crawler_store").With(
		zap.String("task_id", record.TaskID),
		zap.String("url", record.URL),
	)

	payload, err := json.Marshal(record)
	if err != nil {
		return errors.Wrap(err, "marshal crawl record")
	}

	client := rutils.GetCli().GetDB().Client
	if err := saveCrawlRecordPayload(ctx, client, record, payload); err != nil {
		return errors.Wrap(err, "save crawl record payload")
	}

	logger.Debug("saved crawl record", zap.Int("raw_len", len(record.RawBody)))
	return nil
}

// saveCrawlRecordPayload stores the task-specific and latest crawl records with bounded retention.
// It takes a Redis setter, crawl record, and serialized payload, and returns an error when either write fails.
func saveCrawlRecordPayload(ctx context.Context, client redisSetter, record *CrawlRecord, payload []byte) error {
	if err := client.Set(ctx, crawlerRecordKey(record.TaskID), payload, crawlerDataTTL).Err(); err != nil {
		return errors.Wrap(err, "save crawl record")
	}

	if err := client.Set(ctx, crawlerLatestKey(record.URL), payload, crawlerDataTTL).Err(); err != nil {
		return errors.Wrap(err, "save latest crawl record")
	}

	return nil
}
