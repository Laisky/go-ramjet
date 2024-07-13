package config

import (
	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Instance global config instance
var Instance *Config

// Config config
type Config struct {
	S3    s3Config      `json:"s3" mapstructure:"s3"`
	S3Cli *minio.Client `json:"-" mapsructure:"-"`
}

type s3Config struct {
	Endpoint  string `json:"endpoint" mapstructure:"endpoint"`
	Bucket    string `json:"bucket" mapstructure:"bucket"`
	AccessID  string `json:"access_id" mapstructure:"access_id"`
	AccessKey string `json:"-" mapstructure:"access_key"`
}

// SetupConfig setup config
func SetupConfig() error {
	Instance = new(Config)
	if err := gconfig.Shared.UnmarshalKey("openai", Instance); err != nil {
		return errors.Wrap(err, "unmarshal openai config")
	}

	if err := setupS3Cli(); err != nil {
		return errors.Wrap(err, "setup s3 client")
	}

	return nil
}

func setupS3Cli() error {
	cli, err := minio.New(
		config.Config.S3.Endpoint,
		&minio.Options{
			Creds: credentials.NewStaticV4(
				config.Config.S3.AccessID, config.Config.S3.AccessKey, ""),
			Secure: true,
		},
	)
	if err != nil {
		return errors.Wrap(err, "new s3 client")
	}

	Instance.S3Cli = cli
	return nil
}
