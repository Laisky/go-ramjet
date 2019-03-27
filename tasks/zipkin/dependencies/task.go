package dependencies

import (
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func runTask() {
	utils.Logger.Info("run zipkin-dependencies...")
	for env := range utils.Settings.Get("tasks.zipkin.dependencies.configs").(map[string]interface{}) {
		environ := generateContainerEnv(
			utils.Settings.GetString("tasks.zipkin.dependencies.host"),
			utils.Settings.GetString("tasks.zipkin.dependencies.configs."+env+".index"),
			utils.Settings.GetString("tasks.zipkin.dependencies.username"),
			utils.Settings.GetString("tasks.zipkin.dependencies.passwd"),
		)

		stdout, stderr, err := runDockerContainer(
			utils.Settings.GetString("tasks.zipkin.dependencies.endpoint"),
			utils.Settings.GetString("tasks.zipkin.dependencies.image"),
			environ,
		)
		if err != nil || stderr != nil {
			utils.Logger.Error("try to run zipkin-dep container got error",
				zap.Error(err),
				zap.ByteString("stderr", stderr))
		}

		utils.Logger.Debug("run zipkin-dep container done", zap.ByteString("stdout", stdout))
	}

	utils.Logger.Info("zipkin-dependencies done")
}

func BindTask() {
	utils.Logger.Info("bind zipkin-dependencies task...")
	go store.TickerAfterRun(utils.Settings.GetDuration("tasks.zipkin.dependencies.interval")*time.Second, runTask)
}
