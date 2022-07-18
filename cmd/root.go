package cmd

import (
	"context"
	"fmt"

	gconfig "github.com/Laisky/go-config"
	gcmd "github.com/Laisky/go-utils/v2/cmd"
	glog "github.com/Laisky/go-utils/v2/log"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/alert"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
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
			zap.Bool("debug", gconfig.Shared.GetBool("debug")),
			zap.String("addr", gconfig.Shared.GetString("addr")),
			zap.String("config", gconfig.Shared.GetString("config")),
			zap.Strings("task", gconfig.Shared.GetStringSlice("task")),
			zap.Strings("exclude", gconfig.Shared.GetStringSlice("exclude")),
		)

		// Bind each task here
		store.TaskStore.Start(ctx)

		// Run HTTP Server
		web.RunServer(gconfig.Shared.GetString("addr"))
	},
}

func initialize(ctx context.Context, cmd *cobra.Command) {
	if err := gconfig.Shared.BindPFlags(cmd.Flags()); err != nil {
		log.Logger.Panic("bind pflags", zap.Error(err))
	}

	setupSettings(ctx)
	setupLogger(ctx)
}

func setupSettings(ctx context.Context) {
	var err error

	//配置加载
	cfgFile := gconfig.Shared.GetString("config")
	log.Logger.Info("load config", zap.String("file", cfgFile))
	if err = gconfig.Shared.LoadFromFile(cfgFile); err != nil {
		log.Logger.Panic("setup settings", zap.Error(err))
	}
}

func setupLogger(ctx context.Context) {
	opts := []zap.Option{}

	if gconfig.Shared.GetString("logger.push_api") != "" {
		alertPusher, err := glog.NewAlert(
			ctx,
			gconfig.Shared.GetString("logger.push_api"),
			glog.WithAlertType(gconfig.Shared.GetString("logger.alert_type")),
			glog.WithAlertToken(gconfig.Shared.GetString("logger.push_token")),
		)
		if err != nil {
			log.Logger.Panic("create AlertPusher", zap.Error(err))
		}

		opts = append(opts, zap.HooksWithFields(alertPusher.GetZapHook()))
	}

	if _, err := glog.NewConsoleWithName(
		"go-ramjet:"+gconfig.Shared.GetString("host"),
		glog.Level(gconfig.Shared.GetString("log-level")),
		opts...,
	); err != nil {
		log.Logger.Panic("setup logger", zap.Error(err))
	}

	//根据入参来区分日志输出级别
	if gconfig.Shared.GetBool("debug") { // debug mode
		fmt.Println("run in debug mode")
		_ = log.Logger.ChangeLevel("debug")
		gconfig.Shared.Set("log-level", "debug")
	} else { // prod mode
		fmt.Println("run in prod mode")
		_ = log.Logger.ChangeLevel("info")
		gconfig.Shared.Set("log-level", "info")
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
		glog.Shared.Panic("start", zap.Error(err))
	}
}
