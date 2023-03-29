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
	API             string            `json:"api" mapstructure:"api"`
	Token           string            `json:"-" mapstructure:"token"`
	Proxy           string            `json:"-" mapstructure:"proxy"`
	UserTokens      []proxyTokens     `json:"user_tokens" mapstructure:"user_tokens"`
	GoogleAnalytics string            `json:"ga" mapstructure:"ga"`
	StaticLibs      map[string]string `json:"static_libs" mapstructure:"static_libs"`
	QAChatModels    []qaChatModel     `json:"qa_chat_models" mapstructure:"qa_chat_models"`
}

type qaChatModel struct {
	Name    string `json:"name" mapstructure:"name"`
	URL     string `json:"url" mapstructure:"url"`
	Project string `json:"project" mapstructure:"project"`
}

type proxyTokens struct {
	// Token (required) client request token
	Token string `json:"-" mapstructure:"token"`
	// OpenaiToken (optional) openai token
	OpenaiToken   string   `json:"-" mapstructure:"openai_token"`
	AllowedModels []string `json:"allowed_models" mapstructure:"allowed_models"`
}
