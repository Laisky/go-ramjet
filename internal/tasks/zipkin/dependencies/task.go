package dependencies

import (
	"time"

	gconfig "github.com/Laisky/go-config"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

func runTask() {
	log.Logger.Info("run zipkin-dependencies...")
	for env := range gconfig.Shared.Get("tasks.zipkin.dependencies.configs").(map[string]interface{}) {
		environ := generateContainerEnv(
			gconfig.Shared.GetString("tasks.zipkin.dependencies.host"),
			gconfig.Shared.GetString("tasks.zipkin.dependencies.configs."+env+".index"),
			gconfig.Shared.GetString("tasks.zipkin.dependencies.username"),
			gconfig.Shared.GetString("tasks.zipkin.dependencies.passwd"),
		)

		stdout, stderr, err := runDockerContainer(
			gconfig.Shared.GetString("tasks.zipkin.dependencies.endpoint"),
			gconfig.Shared.GetString("tasks.zipkin.dependencies.image"),
			environ,
		)
		if err != nil || stderr != nil {
			log.Logger.Error("try to run zipkin-dep container got error",
				zap.Error(err),
				zap.ByteString("stderr", stderr))
		}

		log.Logger.Debug("run zipkin-dep container done", zap.ByteString("stdout", stdout))
	}

	log.Logger.Info("zipkin-dependencies done")
}

func BindTask() {
	log.Logger.Info("bind zipkin-dependencies task...")
	go store.TaskStore.TickerAfterRun(gconfig.Shared.GetDuration("tasks.zipkin.dependencies.interval")*time.Second, runTask)
}
