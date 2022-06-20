package cmd

import (
	"context"
	"fmt"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/go-utils/v2"
	"github.com/Laisky/zap"
	"github.com/spf13/pflag"

	_ "github.com/Laisky/go-ramjet/internal/tasks"
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/alert"
	"github.com/Laisky/go-ramjet/library/web"
)

// setupSettings setup arguments restored in viper
func setupSettings(flag *pflag.FlagSet) {
	var err error
	//参数加载
	if err = utils.Settings.BindPFlags(flag); err != nil {
		log.Logger.Panic("BindPFlags", zap.Error(err))
	}
	//配置加载
	cfgFile := utils.Settings.GetString("config")
	log.Logger.Info("load config", zap.String("file", cfgFile))
	if err = utils.Settings.LoadFromFile(cfgFile); err != nil {
		log.Logger.Panic("setup settings", zap.Error(err))
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

func setupLogger(ctx context.Context) {
	// log
	alertPusher, err := utils.NewAlertPusherWithAlertType(
		ctx,
		utils.Settings.GetString("logger.push_api"),
		utils.Settings.GetString("logger.alert_type"),
		utils.Settings.GetString("logger.push_token"),
	)
	if err != nil {
		log.Logger.Panic("create AlertPusher", zap.Error(err))
	}

	if _, err := utils.NewConsoleLoggerWithName(
		"go-ramjet:"+utils.Settings.GetString("host"),
		utils.Settings.GetString("log-level"),
		zap.HooksWithFields(alertPusher.GetZapHook())); err != nil {
		log.Logger.Panic("setup logger", zap.Error(err))
	}
}

func setupCMDArgs() *pflag.FlagSet {
	pflag.Bool("debug", false, "run in debug mode")
	pflag.Bool("dry", false, "run in dry mode")
	pflag.Bool("pprof", false, "run with pprof")
	pflag.String("addr", "127.0.0.1:24087", "like `127.0.0.1:24087`")
	pflag.StringP("config", "c", "/etc/go-ramjet/settings.yml", "config file path")
	pflag.String("host", "", "hostname")
	pflag.String("log-level", "", "logger level")
	pflag.StringSliceP("task", "t", []string{}, "which tasks want to runnning, like\n ./main -t t1,t2,heartbeat")
	pflag.StringSliceP("exclude", "e", []string{}, "which tasks do not want to runnning, like\n ./main -e t1,t2,heartbeat")
	pflag.Parse()
	return pflag.CommandLine
}

// Execute run ramjet
func Execute() {
	defer fmt.Println("All done")
	ctx := context.Background()

	//加载参数并启动邮箱
	flags := setupCMDArgs()
	setupSettings(flags)
	setupLogger(ctx)
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
}
