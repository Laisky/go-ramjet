package db

import (
	"context"
	"sync"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/laisky-blog-graphql/library/db/mongo"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	mu       sync.RWMutex
	openaiDB mongo.DB
)

//nolint:contextcheck  // need background context
func setupDB() (err error) {
	mu.Lock()
	defer mu.Unlock()

	if openaiDB != nil {
		return nil
	}

	addr := gconfig.Shared.GetString("db.openai.addr")
	if openaiDB, err = mongo.NewDB(context.Background(), mongo.DialInfo{
		Addr:   addr,
		DBName: gconfig.Shared.GetString("db.openai.db"),
		User:   gconfig.Shared.GetString("db.openai.user"),
		Pwd:    gconfig.Shared.GetString("db.openai.passwd"),
	}); err != nil {
		return errors.Wrapf(err, "connect db to %q", addr)
	}
	log.Logger.Info("connect to openai db", zap.String("addr", addr))

	return nil
}

// GetOpenaiDB get openai db
func GetOpenaiDB() (db mongo.DB, err error) {
	mu.RLock()

	if openaiDB == nil {
		mu.RUnlock()

		if err = setupDB(); err != nil {
			return nil, errors.Wrap(err, "setup db")
		}

		mu.RLock()
	}

	db = openaiDB
	mu.RUnlock()

	return db, nil
}
