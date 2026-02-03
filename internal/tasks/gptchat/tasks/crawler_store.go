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
)

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

func urlHash(url string) string {
	h := sha1.Sum([]byte(url))
	return hex.EncodeToString(h[:])
}

func crawlerLatestKey(url string) string {
	return crawlerURLLatestKeyPrefix + urlHash(url)
}

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
	if err := client.Set(ctx, crawlerRecordKey(record.TaskID), payload, 0).Err(); err != nil {
		return errors.Wrap(err, "save crawl record")
	}

	// Keep latest cache for 7 days to limit unbounded growth.
	if err := client.Set(ctx, crawlerLatestKey(record.URL), payload, 7*24*time.Hour).Err(); err != nil {
		return errors.Wrap(err, "save latest crawl record")
	}

	logger.Debug("saved crawl record", zap.Int("raw_len", len(record.RawBody)))
	return nil
}
