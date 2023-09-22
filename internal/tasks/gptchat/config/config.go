package config

import (
	"context"
	"strings"
	"sync"

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
	if err = gconfig.Shared.UnmarshalKey("openai", Config); err != nil {
		return errors.Wrap(err, "unmarshal openai config")
	}

	// fill default
	Config.RateLimitExpensiveModelsIntervalSeconds = gutils.OptionalVal(&Config.RateLimitExpensiveModelsIntervalSeconds, 60)
	Config.RateLimitImageModelsIntervalSeconds = gutils.OptionalVal(&Config.RateLimitImageModelsIntervalSeconds, 600)

	return nil
}

// OpenAI openai config
type OpenAI struct {
	API                                     string            `json:"api" mapstructure:"api"`
	Token                                   string            `json:"-" mapstructure:"token"`
	DefaultImageToken                       string            `json:"-" mapstructure:"default_image_token"`
	RateLimitExpensiveModelsIntervalSeconds int               `json:"rate_limit_expensive_models_interval_secs" mapstructure:"rate_limit_expensive_models_interval_secs"`
	RateLimitImageModelsIntervalSeconds     int               `json:"rate_limit_image_models_interval_secs" mapstructure:"rate_limit_image_models_interval_secs"`
	Proxy                                   string            `json:"-" mapstructure:"proxy"`
	UserTokens                              []UserConfig      `json:"user_tokens" mapstructure:"user_tokens"`
	GoogleAnalytics                         string            `json:"ga" mapstructure:"ga"`
	StaticLibs                              map[string]string `json:"static_libs" mapstructure:"static_libs"`
	QAChatModels                            []qaChatModel     `json:"qa_chat_models" mapstructure:"qa_chat_models"`
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
	OpenaiToken string `json:"-" mapstructure:"openai_token"`
	// ImageToken (optional) token be used to generate image
	ImageToken string `json:"-" mapstructure:"image_token"`
	// IsPaid whether is paid user
	IsPaid        bool     `json:"is_paid" mapstructure:"-"`
	AllowedModels []string `json:"allowed_models" mapstructure:"allowed_models"`
	// NoLimitExpensiveModels more strict rate limit for expensive models
	NoLimitExpensiveModels bool `json:"no_limit_expensive_models" mapstructure:"no_limit_expensive_models"`
	// NoLimitAllModels general rate limiter for all models
	NoLimitAllModels bool `json:"no_limit_all_models" mapstructure:"no_limit_all_models"`
	// NoLimitImageModels rate limiter for image models
	NoLimitImageModels bool `json:"no_limit_image_models" mapstructure:"no_limit_image_models"`
}

var (
	onceLimiter                                              sync.Once
	ratelimiter, expensiveModelRateLimiter, imageRateLimiter *gutils.RateLimiter
)

// setupRateLimiter setup ratelimiter depends on loaded config
func setupRateLimiter() {
	const burstRatio = 1.2
	var err error
	logger := log.Logger.Named("gptchat.ratelimiter")

	{
		if ratelimiter, err = gutils.NewRateLimiter(context.Background(),
			gutils.RateLimiterArgs{
				Max:     10,
				NPerSec: 1,
			}); err != nil {
			log.Logger.Panic("new ratelimiter", zap.Error(err))
		}
		logger.Info("set overall ratelimiter", zap.Int("burst", 10))
	}

	{
		burst := int(float64(Config.RateLimitExpensiveModelsIntervalSeconds) * burstRatio)
		if expensiveModelRateLimiter, err = gutils.NewRateLimiter(context.Background(),
			gutils.RateLimiterArgs{
				Max:     burst,
				NPerSec: 1,
			}); err != nil {
			log.Logger.Panic("new expensiveModelRateLimiter", zap.Error(err))
		}
		logger.Info("set ratelimiter for expensive models", zap.Int("burst", burst))
	}

	{
		burst := int(float64(Config.RateLimitImageModelsIntervalSeconds) * burstRatio)
		if imageRateLimiter, err = gutils.NewRateLimiter(context.Background(),
			gutils.RateLimiterArgs{
				Max:     burst,
				NPerSec: 1,
			}); err != nil {
			log.Logger.Panic("new imageRateLimiter", zap.Error(err))
		}
		logger.Info("set ratelimiter for image models", zap.Int("burst", burst))
	}
}

// IsModelAllowed check if model is allowed
func (c *UserConfig) IsModelAllowed(model string) error {
	onceLimiter.Do(setupRateLimiter)

	if len(c.AllowedModels) == 0 {
		return errors.Errorf("no allowed models for current user %q", c.UserName)
	}

	var allowed bool
	for _, m := range c.AllowedModels {
		if m == "*" {
			allowed = true
			break
		}

		if m == model {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.Errorf("model %q is not allowed for user %q", model, c.UserName)
	}

	if !c.NoLimitAllModels && !ratelimiter.Allow() { // check rate limit
		return errors.Errorf("too many requests, please try again later")
	}

	if strings.HasPrefix(model, "image-") { // check image models first
		if c.NoLimitImageModels {
			return nil
		}

		price := gconfig.Shared.GetInt("openai.rate_limit_image_models_interval_secs")
		if price == 0 {
			price = 600 // default
		}

		// if price less than 0, means no limit
		log.Logger.Debug("check image model rate limit",
			zap.String("model", model),
			zap.Int("price", price))
		if price >= 0 && !imageRateLimiter.AllowN(price) { // check rate limit
			return errors.Errorf("%q too many requests for image model %q, "+
				"try after %d seconds",
				c.UserName, model, (price - imageRateLimiter.Len()))
		}
	} else if !c.NoLimitExpensiveModels { // then check expensive models
		if model != "gpt-3.5-turbo" {
			// rate limit only support limit by second,
			// so we consume 60 tokens once to make it limit by minute
			price := gconfig.Shared.GetInt("openai.rate_limit_expensive_models_interval_secs")
			if price == 0 {
				price = 60 // default
			}

			// if price less than 0, means no limit
			log.Logger.Debug("check expensive model rate limit",
				zap.String("model", model),
				zap.Int("price", price))
			if price >= 0 && !expensiveModelRateLimiter.AllowN(price) { // check rate limit
				return errors.Errorf("%q too many requests for expensive model %q, "+
					"please after %d seconds",
					c.UserName, model, (price - expensiveModelRateLimiter.Len()))
			}
		}
	}

	return nil
}
