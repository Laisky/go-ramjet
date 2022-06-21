package cmd

import (
	"context"
	"fmt"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/alert"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
	gcmd "github.com/Laisky/go-utils/cmd"
	"github.com/Laisky/go-utils/v2"
	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var rootCMD = &cobra.Command{
	Use:   "go-ramjet",
	Short: "go-ramjet",
	Long:  `go-ramjet`,
	Args:  gcmd.NoExtraArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		initialize(ctx, cmd)

		//加载参数并启动邮箱
		alert.Manager.Setup()

		if err := alert.Telegram.SendAlert("start go-ramjet"); err != nil {
			log.Logger.Error("send telegram msg", zap.Error(err))
		}

		//获取参数
		log.Logger.Info("running...",
			zap.Bool("debug", utils.Settings.GetBool("debug")),
			zap.String("addr", utils.Settings.GetString("addr")),
			zap.String("config", utils.Settings.GetString("config")),
			zap.Strings("task", utils.Settings.GetStringSlice("task")),
			zap.Strings("exclude", utils.Settings.GetStringSlice("exclude")),
		)

		// Bind each task here
		store.TaskStore.Start(ctx)

		// Run HTTP Server
		web.RunServer(utils.Settings.GetString("addr"))
	},
}

func initialize(ctx context.Context, cmd *cobra.Command) error {
	if err := gutils.Settings.BindPFlags(cmd.Flags()); err != nil {
		return errors.Wrap(err, "bind pflags")
	}

	setupSettings(ctx)
	setupLogger(ctx)

	return nil
}

func setupSettings(ctx context.Context) {
	var err error

	//配置加载
	cfgFile := utils.Settings.GetString("config")
	log.Logger.Info("load config", zap.String("file", cfgFile))
	if err = utils.Settings.LoadFromFile(cfgFile); err != nil {
		log.Logger.Panic("setup settings", zap.Error(err))
	}
}

func setupLogger(ctx context.Context) {
	opts := []zap.Option{}

	if utils.Settings.GetString("logger.push_api") != "" {
		alertPusher, err := utils.NewAlertPusherWithAlertType(
			ctx,
			utils.Settings.GetString("logger.push_api"),
			utils.Settings.GetString("logger.alert_type"),
			utils.Settings.GetString("logger.push_token"),
		)
		if err != nil {
			log.Logger.Panic("create AlertPusher", zap.Error(err))
		}

		opts = append(opts, zap.HooksWithFields(alertPusher.GetZapHook()))
	}

	if _, err := utils.NewConsoleLoggerWithName(
		"go-ramjet:"+utils.Settings.GetString("host"),
		utils.Settings.GetString("log-level"),
		opts...,
	); err != nil {
		log.Logger.Panic("setup logger", zap.Error(err))
	}

	//根据入参来区分日志输出级别
	if utils.Settings.GetBool("debug") { // debug mode
		fmt.Println("run in debug mode")
		_ = log.Logger.ChangeLevel("debug")
		utils.Settings.Set("log-level", "debug")
	} else { // prod mode
		fmt.Println("run in prod mode")
		_ = log.Logger.ChangeLevel("info")
		utils.Settings.Set("log-level", "info")
	}

}

func init() {
	rootCMD.PersistentFlags().Bool("debug", false, "run in debug mode")
	rootCMD.PersistentFlags().Bool("dry", false, "run in dry mode")
	rootCMD.PersistentFlags().Bool("pprof", false, "run with pprof")
	rootCMD.PersistentFlags().String("addr", "127.0.0.1:24087", "like `127.0.0.1:24087`")
	rootCMD.PersistentFlags().StringP("config", "c", "/etc/go-ramjet/settings.yml", "config file path")
	rootCMD.PersistentFlags().String("host", "127.0.0.1", "hostname")
	rootCMD.PersistentFlags().String("log-level", "info", "logger level")
	rootCMD.PersistentFlags().StringSliceP("task", "t", []string{},
		"which tasks want to runnning, like\n ./main -t t1,t2,heartbeat")
	rootCMD.PersistentFlags().StringSliceP("exclude", "e", []string{},
		"which tasks do not want to runnning, like\n ./main -e t1,t2,heartbeat")
}

func Execute() {
	if err := rootCMD.Execute(); err != nil {
		gutils.Logger.Panic("start", zap.Error(err))
	}
}
