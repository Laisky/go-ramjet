package ratelimit

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/redis/go-redis/v9"

	glog "github.com/Laisky/go-utils/v5/log"

	"github.com/Laisky/go-ramjet/library/log"
	rutils "github.com/Laisky/go-ramjet/library/redis"
)

// Limiter defines the behaviour shared by all rate limiters in this project.
type Limiter interface {
	Allow() bool
	AllowN(n int) bool
	Len() int
}

// Args describes how a limiter should be initialised.
type Args struct {
	// Max is the bucket capacity.
	Max int
	// NPerSec defines the refill speed per second.
	NPerSec int
}

// redisLimiter is a Redis-backed token bucket rate limiter.
type redisLimiter struct {
	ctx        context.Context
	client     *redis.Client
	logger     glog.Logger
	key        string
	maxTokens  float64
	refillRate float64
	ttl        time.Duration
	mu         sync.Mutex
	lastTokens int
}

// tokenBucketScript implements the token bucket algorithm atomically in Redis.
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local max_tokens = tonumber(ARGV[2])
local refill_rate = tonumber(ARGV[3])
local cost = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

local state = redis.call("HMGET", key, "tokens", "timestamp")
local tokens = tonumber(state[1])
local last_ts = tonumber(state[2])

if tokens == nil or last_ts == nil then
	tokens = max_tokens
	last_ts = now
end

local elapsed = now - last_ts
if elapsed < 0 then
	elapsed = 0
end

local refill = elapsed * refill_rate / 1000
if refill > 0 then
	tokens = math.min(max_tokens, tokens + refill)
	last_ts = now
end

local allowed = 0
if cost <= tokens then
	tokens = tokens - cost
	allowed = 1
end

redis.call("HMSET", key, "tokens", tokens, "timestamp", last_ts)
if ttl_ms > 0 then
	redis.call("PEXPIRE", key, ttl_ms)
end

return {allowed, tokens}
`)

// NewRedisLimiter creates a Redis-backed limiter with the provided name and arguments.
func NewRedisLimiter(ctx context.Context, name string, args Args) (Limiter, error) {
	if args.Max <= 0 {
		return nil, errors.Errorf("ratelimit %q: max must be > 0", name)
	}
	if args.NPerSec <= 0 {
		return nil, errors.Errorf("ratelimit %q: n_per_sec must be > 0", name)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	cli := rutils.GetCli().GetDB()
	if cli == nil {
		return nil, errors.Errorf("ratelimit %q: redis client is nil", name)
	}

	refill := float64(args.NPerSec)
	maxTokens := float64(args.Max)
	windowSeconds := maxTokens / refill
	if windowSeconds <= 0 {
		windowSeconds = 1
	}
	ttlSeconds := math.Max(windowSeconds*3, 60)
	ttlMillis := int64(math.Ceil(ttlSeconds * 1000))

	l := &redisLimiter{
		ctx:        ctx,
		client:     cli.Client,
		logger:     log.Logger.Named("ratelimit").With(zap.String("name", name)),
		key:        fmt.Sprintf("ramjet:ratelimit:%s", name),
		maxTokens:  maxTokens,
		refillRate: refill,
		ttl:        time.Duration(ttlMillis) * time.Millisecond,
		lastTokens: -1,
	}

	return l, nil
}

func (l *redisLimiter) Allow() bool {
	return l.AllowN(1)
}

func (l *redisLimiter) AllowN(n int) bool {
	if n <= 0 {
		n = 1
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	allowed, tokens, err := l.eval(n)
	if err != nil {
		l.logger.Warn("redis limiter allow", zap.Error(err))
		return true
	}

	l.lastTokens = tokens
	return allowed
}

func (l *redisLimiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastTokens >= 0 {
		return l.lastTokens
	}

	_, tokens, err := l.eval(0)
	if err != nil {
		l.logger.Warn("redis limiter len", zap.Error(err))
		return 0
	}

	l.lastTokens = tokens
	return tokens
}

func (l *redisLimiter) eval(cost int) (bool, int, error) {
	res, err := tokenBucketScript.Run(l.ctx, l.client, []string{l.key},
		time.Now().UnixMilli(), l.maxTokens, l.refillRate, cost, int(l.ttl/time.Millisecond)).Result()
	if err != nil {
		return true, 0, err
	}

	values, ok := res.([]interface{})
	if !ok || len(values) != 2 {
		return true, 0, errors.Errorf("unexpected redis limiter response: %#v", res)
	}

	allowed := toInt(values[0]) == 1
	tokens := toInt(values[1])

	return allowed, tokens, nil
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(math.Floor(val + 0.5))
	case string:
		if val == "" {
			return 0
		}
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0
		}
		return int(math.Floor(f + 0.5))
	default:
		return 0
	}
}
