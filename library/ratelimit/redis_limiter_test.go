package ratelimit

import (
	"context"
	"testing"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/redis/go-redis/v9"
)

const (
	testRedisAddr = "100.122.41.16:6379"
	testRedisDB   = 4
)

func setupTestRedis(t *testing.T) *redis.Client {
	addr := testRedisAddr
	db := testRedisDB

	gconfig.Shared.Set("redis.addr", addr)
	gconfig.Shared.Set("redis.db", db)
	gconfig.S.Set("redis.addr", addr)
	gconfig.S.Set("redis.db", db)

	cli := redis.NewClient(&redis.Options{Addr: addr, DB: db})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cli.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not available at %s db %d: %v", addr, db, err)
	}

	if err := cli.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush redis db: %v", err)
	}

	t.Cleanup(func() {
		_ = cli.Close()
	})

	return cli
}

func TestRedisLimiterAllowsAndRefills(t *testing.T) {
	setupTestRedis(t)

	limiter, err := NewRedisLimiter(context.Background(), t.Name(), Args{Max: 5, NPerSec: 1})
	if err != nil {
		t.Fatalf("create redis limiter: %v", err)
	}

	if !limiter.AllowN(5) {
		t.Fatalf("expected initial burst to allow all tokens")
	}
	if limiter.Allow() {
		t.Fatalf("expected limiter to block when bucket empty")
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		if limiter.Allow() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("limiter did not refill within expected time")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRedisLimiterPersistsAcrossInstances(t *testing.T) {
	setupTestRedis(t)

	name := t.Name()
	limiter1, err := NewRedisLimiter(context.Background(), name, Args{Max: 3, NPerSec: 1})
	if err != nil {
		t.Fatalf("create first limiter: %v", err)
	}

	if !limiter1.AllowN(3) {
		t.Fatalf("expected initial tokens to be available")
	}

	limiter2, err := NewRedisLimiter(context.Background(), name, Args{Max: 3, NPerSec: 1})
	if err != nil {
		t.Fatalf("create second limiter: %v", err)
	}

	if limiter2.Allow() {
		t.Fatalf("expected limiter to maintain state across instances")
	}
}

func TestNewLimiterBackendSelection(t *testing.T) {
	t.Run("redis-backend", func(t *testing.T) {
		setupTestRedis(t)
		previous := gconfig.Shared.GetString("openai.rate_limiter_backend")
		gconfig.Shared.Set("openai.rate_limiter_backend", BackendRedis)
		t.Cleanup(func() {
			gconfig.Shared.Set("openai.rate_limiter_backend", previous)
		})

		limiter, err := New(context.Background(), t.Name(), Args{Max: 2, NPerSec: 1})
		if err != nil {
			t.Fatalf("create redis-backed limiter: %v", err)
		}
		if _, ok := limiter.(*redisLimiter); !ok {
			t.Fatalf("expected redis limiter, got %T", limiter)
		}
	})

	t.Run("legacy-backend", func(t *testing.T) {
		previous := gconfig.Shared.GetString("openai.rate_limiter_backend")
		gconfig.Shared.Set("openai.rate_limiter_backend", BackendLegacy)
		t.Cleanup(func() {
			gconfig.Shared.Set("openai.rate_limiter_backend", previous)
		})

		limiter, err := New(context.Background(), t.Name(), Args{Max: 2, NPerSec: 1})
		if err != nil {
			t.Fatalf("create legacy limiter: %v", err)
		}
		if _, ok := limiter.(*memoryLimiter); !ok {
			t.Fatalf("expected memory limiter, got %T", limiter)
		}
	})
}
