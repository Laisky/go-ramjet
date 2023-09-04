package config

import (
	"context"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/log"
)

const (
	// FREETIER_USER_TOKEN freetier user token
	FREETIER_USER_TOKEN = "DEFAULT_PROXY_TOKEN"
)

var (
	// Config global shared config instance
	Config *OpenAI
)

// SetupConfig setup config
func SetupConfig() (err error) {
	Config = new(OpenAI)
	err = gconfig.Shared.UnmarshalKey("openai", Config)
	return errors.Wrap(err, "unmarshal openai config")
}

// OpenAI openai config
type OpenAI struct {
	API             string            `json:"api" mapstructure:"api"`
	Token           string            `json:"-" mapstructure:"token"`
	Proxy           string            `json:"-" mapstructure:"proxy"`
	UserTokens      []UserConfig      `json:"user_tokens" mapstructure:"user_tokens"`
	GoogleAnalytics string            `json:"ga" mapstructure:"ga"`
	StaticLibs      map[string]string `json:"static_libs" mapstructure:"static_libs"`
	QAChatModels    []qaChatModel     `json:"qa_chat_models" mapstructure:"qa_chat_models"`
}

type qaChatModel struct {
	Name    string `json:"name" mapstructure:"name"`
	URL     string `json:"url" mapstructure:"url"`
	Project string `json:"project" mapstructure:"project"`
}

// UserConfig user config
type UserConfig struct {
	UserName string `json:"username" mapstructure:"username"`
	// Token (required) client request token
	Token string `json:"-" mapstructure:"token"`
	// OpenaiToken (optional) openai token
	OpenaiToken   string   `json:"-" mapstructure:"openai_token"`
	AllowedModels []string `json:"allowed_models" mapstructure:"allowed_models"`
	// LimitExpensiveModels more strict rate limit for expensive models
	LimitExpensiveModels bool `json:"limit_expensive_models" mapstructure:"limit_expensive_models"`
}

var (
	ratelimiter, expensiveModelRateLimiter *gutils.RateLimiter
)

func init() {
	var err error
	if ratelimiter, err = gutils.NewRateLimiter(context.Background(),
		gutils.RateLimiterArgs{
			Max:     10,
			NPerSec: 1,
		}); err != nil {
		log.Logger.Panic("new ratelimiter", zap.Error(err))
	}
	if expensiveModelRateLimiter, err = gutils.NewRateLimiter(context.Background(),
		gutils.RateLimiterArgs{
			Max:     65,
			NPerSec: 1,
		}); err != nil {
		log.Logger.Panic("new expensiveModelRateLimiter", zap.Error(err))
	}
}

// IsModelAllowed check if model is allowed
func (c *UserConfig) IsModelAllowed(model string) error {
	if len(c.AllowedModels) == 0 {
		return errors.Errorf("no allowed models for current user %q", c.UserName)
	}

	if !ratelimiter.Allow() { // check rate limit
		return errors.Errorf("too many requests, please try again later")
	}

	if c.LimitExpensiveModels && model != "gpt-3.5-turbo" {
		// rate limit only support limit by second,
		// so we consume 60 tokens once to make it limit by minute
		for i := 0; i < 60; i++ {
			if !expensiveModelRateLimiter.Allow() { // check rate limit
				return errors.Errorf("too many requests for expensive model %q, "+
					"please try again later or use 3.5-turbo instead", model)
			}
		}
	}

	for _, m := range c.AllowedModels {
		if m == "*" {
			return nil
		}

		if m == model {
			return nil
		}
	}

	return errors.Errorf("model %q is not allowed for user %q", model, c.UserName)
}
