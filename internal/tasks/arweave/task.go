package arweave

import (
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

// bindTask bind heartbeat task
func bindTask() {
	log.Logger.Info("bind arweave task...")
	bindHTTP()
}

func init() {
	store.TaskStore.Store("arweave", bindTask)
}
