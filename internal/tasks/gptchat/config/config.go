package config

import (
	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
)

var (
	Config *OpenAI
)

func SetupConfig() (err error) {
	Config = new(OpenAI)
	err = gconfig.Shared.UnmarshalKey("openai", Config)
	return errors.Wrap(err, "unmarshal openai config")
}

type OpenAI struct {
	Token             string        `json:"-" mapstructure:"token"`
	Proxy             string        `json:"-" mapstructure:"proxy"`
	BypassProxyTokens []proxyTokens `json:"bypass_proxy_tokens" mapstructure:"bypass_proxy_tokens"`
	GoogleAnalytics   string        `json:"ga" mapstructure:"ga"`
}

type proxyTokens struct {
	Token         string   `json:"-" mapstructure:"token"`
	OpenaiToken   string   `json:"-" mapstructure:"openai_token"`
	AllowedModels []string `json:"allowed_models" mapstructure:"allowed_models"`
}
