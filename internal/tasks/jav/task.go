// Package jav is a package for jav tasks
package jav

import (
	"context"

	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/jav/model"
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

// bindTask bind heartbeat task
func bindTask() {
	log.Logger.Info("bind jav task...")

	if err := model.SetupDB(context.Background()); err != nil {
		log.Logger.Panic("setup db", zap.Error(err))
	}

	bindHTTP()
}

func init() {
	store.TaskStore.Store("jav", bindTask)
}
