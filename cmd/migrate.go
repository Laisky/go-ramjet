package cmd

// import (
// 	"context"

// 	"github.com/Laisky/go-ramjet/internal/tasks/crawler"
// 	"github.com/Laisky/go-ramjet/library/log"

// 	gconfig "github.com/Laisky/go-config/v2"
// 	gcmd "github.com/Laisky/go-utils/v5/cmd"
// 	"github.com/Laisky/zap"
// 	"github.com/spf13/cobra"
// )

// var migrateCMD = &cobra.Command{
// 	Use:   "migrate",
// 	Short: "migrate",
// 	Long:  `migrate db`,
// 	Args:  gcmd.NoExtraArgs,
// 	PreRun: func(cmd *cobra.Command, args []string) {
// 		ctx := context.Background()
// 		initialize(ctx, cmd)
// 	},
// 	Run: func(cmd *cobra.Command, args []string) {
// 		d, err := crawler.NewDao(gconfig.Shared.GetString("db.crawler.dsn"))
// 		if err != nil {
// 			log.Logger.Panic("new dao", zap.Error(err))
// 		}

// 		if err := d.DB.
// 			Set("gorm:table_options",
// 				"ENGINE=MergeTree() PARTITION BY url ORDER BY (url) SETTINGS index_granularity=8192").
// 			AutoMigrate(
// 				crawler.SearchText{},
// 			); err != nil {
// 			log.Logger.Panic("migrate", zap.Error(err))
// 		}

// 		log.Logger.Info("migrate done")
// 	},
// }

// func init() {
// 	rootCMD.AddCommand(migrateCMD)
// }
