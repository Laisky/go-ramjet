// Package cmd implements the root command of go-ramjet.
package cmd

import (
	"context"
	"fmt"
	"os"

	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
	gcmd "github.com/Laisky/go-utils/v4/cmd"
	glog "github.com/Laisky/go-utils/v4/log"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	"github.com/Laisky/go-ramjet/internal/pkg/singleton"
	_ "github.com/Laisky/go-ramjet/internal/tasks"
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

		if !initialize(ctx, cmd) {
			return
		}

		//加载参数并启动邮箱
		alert.Manager.Setup()

		// if err := alert.Telegram.SendAlert("start go-ramjet"); err != nil {
		// 	log.Logger.Error("send telegram msg", zap.Error(err))
		// }

		//获取参数
		log.Logger.Info("running...",
			zap.Bool("debug", gconfig.Shared.GetBool("debug")),
			zap.String("addr", gconfig.Shared.GetString("server.addr")),
			zap.String("config", gconfig.Shared.GetString("config")),
			zap.Strings("task", gconfig.Shared.GetStringSlice("task")),
			zap.Strings("exclude", gconfig.Shared.GetStringSlice("exclude")),
		)

		// Bind each task here
		store.TaskStore.Start(ctx)

		// Run HTTP Server
		web.RunServer(gconfig.Shared.GetString("server.addr"))
	},
}

func initialize(ctx context.Context, cmd *cobra.Command) bool {
	if err := gconfig.Shared.BindPFlags(cmd.Flags()); err != nil {
		log.Logger.Panic("bind pflags", zap.Error(err))
	}

	if !setupSettings(ctx) {
		return false
	}

	setupLogger(ctx)
	return true
}

func setupSettings(_ context.Context) bool {
	var err error

	if gconfig.Shared.GetBool("version") {
		fmt.Println(gutils.PrettyBuildInfo())
		return false
	}

	//配置加载
	cfgFile := gconfig.Shared.GetString("config")
	log.Logger.Info("load config", zap.String("file", cfgFile))
	if err = gconfig.Shared.LoadFromFile(cfgFile); err != nil {
		log.Logger.Panic("setup settings", zap.Error(err))
	}

	if err = singleton.Setup(); err != nil {
		log.Logger.Panic("setup singleton", zap.Error(err))
	}

	return true
}

func setupLogger(ctx context.Context) {
	opts := []zap.Option{}

	if gconfig.Shared.GetString("logger.push_api") != "" {
		alertPusher, err := glog.NewAlert(
			ctx,
			gconfig.Shared.GetString("logger.push_api"),
			glog.WithAlertType(gconfig.Shared.GetString("logger.alert_type")),
			glog.WithAlertToken(gconfig.Shared.GetString("logger.push_token")),
			glog.WithAlertHookLevel(zap.ErrorLevel),
		)
		if err != nil {
			log.Logger.Panic("create AlertPusher", zap.Error(err))
		}

		opts = append(opts, zap.HooksWithFields(alertPusher.GetZapHook()))
		log.Logger.Info("set alert",
			zap.String("alert_api", gconfig.Shared.GetString("logger.push_api")),
			zap.String("alert_type", gconfig.Shared.GetString("logger.alert_type")),
		)
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Logger.Panic("get hostname", zap.Error(err))
	}

	logger := log.Logger.WithOptions(opts...).With(
		zap.String("host", hostname),
	)
	log.Logger = logger

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
	// rootCMD.PersistentFlags().String("addr", "127.0.0.1:24087", "like `127.0.0.1:24087`")
	rootCMD.PersistentFlags().StringP("config", "c", "/etc/go-ramjet/settings.yml", "config file path")
	// rootCMD.PersistentFlags().String("host", "127.0.0.1", "hostname")
	rootCMD.PersistentFlags().BoolP("version", "v", false, "show version")
	rootCMD.PersistentFlags().String("log-level", "info", "logger level")
	rootCMD.PersistentFlags().StringSliceP("task", "t", []string{},
		"which tasks want to runnning, like\n ./main -t t1,t2,heartbeat")
	rootCMD.PersistentFlags().StringSliceP("exclude", "e", []string{},
		"which tasks do not want to runnning, like\n ./main -e t1,t2,heartbeat")
}

// Execute run root command
func Execute() {
	if err := rootCMD.Execute(); err != nil {
		glog.Shared.Panic("start", zap.Error(err))
	}
}
