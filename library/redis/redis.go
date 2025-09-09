package redis

import (
	"sync"

	gconfig "github.com/Laisky/go-config/v2"
	rdb "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"github.com/redis/go-redis/v9"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	mu  sync.RWMutex
	cli *rdb.DB
)

// GetCli returns the redis client
func GetCli() *rdb.DB {
	mu.RLock()
	if cli != nil {
		mu.RUnlock()
		return cli
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	if cli != nil {
		return cli
	}

	cli = rdb.NewDB(&redis.Options{
		Addr: gconfig.S.GetString("redis.addr"),
		DB:   gconfig.S.GetInt("redis.db"),
	})

	log.Logger.Info("new redis client",
		zap.String("addr", gconfig.S.GetString("redis.addr")))
	return cli
}
