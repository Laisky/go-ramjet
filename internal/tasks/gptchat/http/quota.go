package http

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
	rutils "github.com/Laisky/go-ramjet/library/redis"
)

const (
	tokenReservationContextKey = "gptchat_token_quota"
	tokenQuotaLimit            = 10000
	tokenQuotaWindow           = 10 * time.Minute
)

var (
	tokenQuotaOnce sync.Once
	tokenQuotaMgr  *TokenQuotaManager
)

// QuotaExceededError indicates the free-tier quota has been exhausted.
type QuotaExceededError struct {
	Limit      int
	Used       int
	Remaining  int
	RetryAfter time.Duration
}

func (e *QuotaExceededError) Error() string {
	return fmt.Sprintf("free tier quota exceeded: limit %d tokens per 10-minute window, %d tokens used", e.Limit, e.Used)
}

// TokenQuotaManager keeps track of per-user token usage within a rolling window.
type TokenQuotaManager struct {
	client         *redis.Client
	logger         glog.Logger
	limit          int
	windowDuration time.Duration
	windowMinutes  int64
	ttl            time.Duration
}

// TokenReservation represents a temporary reservation of tokens for a request.
type TokenReservation struct {
	manager        *TokenQuotaManager
	key            string
	field          string
	promptTokens   int
	reservedOutput int
	reservedTotal  int
	createdAt      time.Time
	once           sync.Once
}

var quotaAdjustScript = redis.NewScript(`
local key = KEYS[1]
local field = ARGV[1]
local delta = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

local current = tonumber(redis.call("HGET", key, field) or "0")
local new_value = current + delta
if new_value < 0 then
	new_value = 0
end

if new_value == 0 then
	redis.call("HDEL", key, field)
else
	redis.call("HSET", key, field, new_value)
end

if ttl > 0 then
	redis.call("EXPIRE", key, ttl)
end

return new_value
`)

func getTokenQuotaManager() *TokenQuotaManager {
	tokenQuotaOnce.Do(func() {
		client := rutils.GetCli().GetDB()
		tokenQuotaMgr = &TokenQuotaManager{
			client:         client.Client,
			logger:         log.Logger.Named("token_quota"),
			limit:          tokenQuotaLimit,
			windowDuration: tokenQuotaWindow,
			windowMinutes:  int64(tokenQuotaWindow / time.Minute),
			ttl:            tokenQuotaWindow + 2*time.Minute,
		}
	})

	return tokenQuotaMgr
}

// Reserve tokens for a free-tier user. Returns nil when no reservation is needed.
func ReserveTokens(ctx *gin.Context, user *config.UserConfig, req *FrontendReq) (*TokenReservation, error) {
	if ctx == nil || user == nil || req == nil || !user.IsFree {
		return nil, nil
	}

	manager := getTokenQuotaManager()
	if manager == nil {
		return nil, errors.New("token quota manager not available")
	}

	promptTokens := req.PromptTokens()
	estimatedOutput := int(req.MaxTokens)
	if estimatedOutput < 0 {
		estimatedOutput = 0
	}
	reservation, err := manager.reserve(gmw.Ctx(ctx), user.UserName, promptTokens, estimatedOutput)
	if err != nil {
		return nil, err
	}

	if reservation != nil {
		ctx.Set(tokenReservationContextKey, reservation)
	}

	return reservation, nil
}

func getTokenReservation(ctx *gin.Context) *TokenReservation {
	if ctx == nil {
		return nil
	}

	val, ok := ctx.Get(tokenReservationContextKey)
	if !ok {
		return nil
	}

	reservation, _ := val.(*TokenReservation)
	return reservation
}

func clearTokenReservation(ctx *gin.Context) {
	if ctx != nil {
		ctx.Set(tokenReservationContextKey, nil)
	}
}

func (m *TokenQuotaManager) key(user string) string {
	return fmt.Sprintf("ramjet:gptchat:quota:%s", user)
}

func (m *TokenQuotaManager) reserve(ctx context.Context, user string, promptTokens, estimatedOutput int) (*TokenReservation, error) {
	if user == "" {
		return nil, errors.New("empty user name for quota reservation")
	}

	total := promptTokens + estimatedOutput
	if total <= 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	bucket := now.Unix() / 60
	fields := make([]string, 0, m.windowMinutes)
	for i := int64(0); i < m.windowMinutes; i++ {
		fields = append(fields, strconv.FormatInt(bucket-i, 10))
	}

	key := m.key(user)
	vals, err := m.client.HMGet(ctx, key, fields...).Result()
	if err != nil {
		return nil, errors.Wrap(err, "load token quota usage")
	}

	used := 0
	earliestBucket := int64(-1)
	for idx, raw := range vals {
		if raw == nil {
			continue
		}

		tokens := parseRedisInt(raw)
		if tokens <= 0 {
			continue
		}

		used += tokens
		candidate := bucket - int64(idx)
		if earliestBucket == -1 || candidate < earliestBucket {
			earliestBucket = candidate
		}
	}

	if used+total > m.limit {
		retryAfter := m.windowDuration
		if earliestBucket >= 0 {
			earliestTime := time.Unix(earliestBucket*60, 0).UTC()
			windowEnd := earliestTime.Add(m.windowDuration)
			retryAfter = windowEnd.Sub(now)
			if retryAfter < time.Second {
				retryAfter = time.Second
			}
		}

		remaining := m.limit - used
		if remaining < 0 {
			remaining = 0
		}
		return nil, &QuotaExceededError{
			Limit:      m.limit,
			Used:       used,
			Remaining:  remaining,
			RetryAfter: retryAfter,
		}
	}

	if err := m.client.HIncrBy(ctx, key, fields[0], int64(total)).Err(); err != nil {
		return nil, errors.Wrap(err, "reserve token quota")
	}

	if err := m.client.Expire(ctx, key, m.ttl).Err(); err != nil {
		m.logger.Warn("set quota ttl", zap.Error(err), zap.String("key", key))
	}

	return &TokenReservation{
		manager:        m,
		key:            key,
		field:          fields[0],
		promptTokens:   promptTokens,
		reservedOutput: estimatedOutput,
		reservedTotal:  total,
		createdAt:      now,
	}, nil
}

func (m *TokenQuotaManager) adjust(ctx context.Context, key, field string, delta int64) error {
	if delta == 0 {
		return nil
	}

	_, err := quotaAdjustScript.Run(ctx, m.client, []string{key}, field, delta, int64(m.ttl/time.Second)).Result()
	if err != nil {
		return errors.Wrap(err, "adjust token quota")
	}

	return nil
}

// Finalize updates the reservation to match the actual token usage.
func (r *TokenReservation) Finalize(ctx context.Context, actualOutputTokens int) error {
	if r == nil {
		return nil
	}

	var result error
	r.once.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}

		if actualOutputTokens < 0 {
			actualOutputTokens = 0
		}

		actualTotal := r.promptTokens + actualOutputTokens
		delta := actualTotal - r.reservedTotal
		if delta == 0 {
			return
		}

		if err := r.manager.adjust(ctx, r.key, r.field, int64(delta)); err != nil {
			result = err
		}
	})

	return result
}

// EstimatedOutputTokens returns the initially reserved completion token budget.
func (r *TokenReservation) EstimatedOutputTokens() int {
	if r == nil {
		return 0
	}

	return r.reservedOutput
}

func parseRedisInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		if val == "" {
			return 0
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}
