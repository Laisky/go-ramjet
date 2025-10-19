package ratelimit

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/log"
)

const (
	// BackendRedis persists limiter state in Redis.
	BackendRedis = "redis"
	// BackendLegacy keeps limiter state in memory (legacy behaviour).
	BackendLegacy = "legacy"
	backendMemory = "memory"
)

func normaliseBackend(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return BackendRedis
	}

	switch value {
	case BackendRedis:
		return BackendRedis
	case BackendLegacy, backendMemory:
		return BackendLegacy
	default:
		log.Logger.Warn("unknown rate limiter backend, fallback to redis", zap.String("backend", value))
		return BackendRedis
	}
}

// New creates a limiter instance using the configured backend.
func New(ctx context.Context, name string, args Args) (Limiter, error) {
	backend := normaliseBackend(gconfig.Shared.GetString("openai.rate_limiter_backend"))
	switch backend {
	case BackendLegacy:
		return newMemoryLimiter(ctx, args)
	case BackendRedis:
		return NewRedisLimiter(ctx, name, args)
	default:
		return nil, errors.Errorf("unsupported rate limiter backend %q", backend)
	}
}

type memoryLimiter struct {
	inner *gutils.RateLimiter
}

func newMemoryLimiter(ctx context.Context, args Args) (Limiter, error) {
	if args.Max <= 0 {
		return nil, errors.New("ratelimit: max must be > 0")
	}
	if args.NPerSec <= 0 {
		return nil, errors.New("ratelimit: n_per_sec must be > 0")
	}
	if args.Max < args.NPerSec {
		return nil, errors.New("ratelimit: max must be >= n_per_sec")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	inner, err := gutils.NewRateLimiter(ctx, gutils.RateLimiterArgs{
		Max:     args.Max,
		NPerSec: args.NPerSec,
	})
	if err != nil {
		return nil, errors.Wrap(err, "create memory rate limiter")
	}

	return &memoryLimiter{inner: inner}, nil
}

func (m *memoryLimiter) Allow() bool {
	return m.inner.Allow()
}

func (m *memoryLimiter) AllowN(n int) bool {
	if n <= 0 {
		n = 1
	}

	return m.inner.AllowN(n)
}

func (m *memoryLimiter) Len() int {
	return m.inner.Len()
}
