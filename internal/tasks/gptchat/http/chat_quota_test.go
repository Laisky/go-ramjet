package http

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

// freeUserQuotaHook supplies an empty quota window for handler unit tests.
type freeUserQuotaHook struct {
	reads atomic.Int32
}

// DialHook rejects unexpected connections so the test cannot use a Redis service.
func (h *freeUserQuotaHook) DialHook(redis.DialHook) redis.DialHook {
	return func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unexpected Redis dial in free-user unit test")
	}
}

// ProcessHook supplies only the quota commands exercised by the handler.
func (h *freeUserQuotaHook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, command redis.Cmder) error {
		switch cmd := command.(type) {
		case *redis.SliceCmd:
			if cmd.Name() != "hmget" {
				return errors.Errorf("unexpected quota command %s", cmd.Name())
			}
			h.reads.Add(1)
			cmd.SetVal(make([]any, len(cmd.Args())-2))
		case *redis.IntCmd:
			if cmd.Name() != "hincrby" {
				return errors.Errorf("unexpected quota command %s", cmd.Name())
			}
			cmd.SetVal(0)
		case *redis.BoolCmd:
			if cmd.Name() != "expire" {
				return errors.Errorf("unexpected quota command %s", cmd.Name())
			}
			cmd.SetVal(true)
		case *redis.Cmd:
			if cmd.Name() != "evalsha" {
				return errors.Errorf("unexpected quota command %s", cmd.Name())
			}
			cmd.SetVal(int64(0))
		default:
			return errors.Errorf("unexpected quota command %s", command.Name())
		}
		return nil
	}
}

// ProcessPipelineHook rejects pipelines because this fake does not implement them.
func (h *freeUserQuotaHook) ProcessPipelineHook(redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(context.Context, []redis.Cmder) error {
		return errors.New("unexpected Redis pipeline in free-user unit test")
	}
}

// setupFreeUserTestQuota replaces and restores quota state without copying sync.Once.
func setupFreeUserTestQuota(t *testing.T) *freeUserQuotaHook {
	t.Helper()
	t.Setenv("DISABLE_LLM_CONSERVATION_AUDIT", "true")
	original := tokenQuotaMgr
	hook := &freeUserQuotaHook{}
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	client.AddHook(hook)
	t.Cleanup(func() {
		tokenQuotaMgr = original
		tokenQuotaOnce = sync.Once{}
		if original != nil {
			tokenQuotaOnce.Do(func() {})
		}
		require.NoError(t, client.Close())
	})
	tokenQuotaOnce = sync.Once{}
	tokenQuotaOnce.Do(func() {})
	tokenQuotaMgr = &TokenQuotaManager{
		client: client, logger: log.Logger.Named("free_user_test_quota"),
		limit: tokenQuotaLimit, windowDuration: tokenQuotaWindow,
		windowMinutes: int64(tokenQuotaWindow.Minutes()), ttl: tokenQuotaWindow,
	}
	return hook
}
