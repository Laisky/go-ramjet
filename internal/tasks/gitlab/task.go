package gitlab

import (
	gconfig "github.com/Laisky/go-config"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

func bindTask() {
	log.Logger.Info("bind gitlab api server")

	InitSvc(
		gconfig.Shared.GetString("tasks.gitlab.api"),
		gconfig.Shared.GetString("tasks.gitlab.token"),
	)

	registerWeb()

}

func init() {
	store.TaskStore.Store("gitlab", bindTask)
}
