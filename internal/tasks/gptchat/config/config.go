// Package config implements config.
package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/log"
)

const (
	// FreetierUserToken freetier user token
	FreetierUserToken = "DEFAULT_PROXY_TOKEN"
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

	if Config.Token == "" {
		return errors.New("openai.token is empty")
	}

	if Config.ExternalBillingAPI != "" && Config.ExternalBillingToken == "" {
		return errors.New("external_billing_token should not be empty if external_billing_api is set")
	}

	// fill default
	Config.RateLimitExpensiveModelsIntervalSeconds = gutils.OptionalVal(
		&Config.RateLimitExpensiveModelsIntervalSeconds, 60)
	Config.RateLimitImageModelsIntervalSeconds = gutils.OptionalVal(
		&Config.RateLimitImageModelsIntervalSeconds, 600)
	Config.DefaultImageToken = gutils.OptionalVal(
		&Config.DefaultImageToken, Config.Token)
	Config.DefaultImageTokenType = gutils.OptionalVal(
		&Config.DefaultImageTokenType, ImageTokenOpenai)
	Config.API = strings.TrimRight(gutils.OptionalVal(
		&Config.API, "https://api.openai.com"), "/")
	Config.ExternalBillingAPI = gutils.OptionalVal(
		&Config.ExternalBillingAPI, "https://oneapi.laisky.com")

	return nil
}

// OpenAI openai config
//
// nolint: lll
type OpenAI struct {
	// API (optional) openai api base url, default is https://api.openai.com
	API string `json:"api" mapstructure:"api"`
	// Token (required) openai api request token
	Token string `json:"-" mapstructure:"token"`
	// DefaultImageToken (optional) default image token, default equals to token
	DefaultImageToken string `json:"-" mapstructure:"default_image_token"`
	// DefaultImageTokenType (optional) default image token type, default is openai
	DefaultImageTokenType ImageTokenType `json:"-" mapstructure:"default_image_token_type"`
	// RateLimitExpensiveModelsIntervalSeconds (optional) rate limit interval seconds for expensive models, default is 60
	RateLimitExpensiveModelsIntervalSeconds int `json:"rate_limit_expensive_models_interval_secs" mapstructure:"rate_limit_expensive_models_interval_secs"`
	// RateLimitImageModelsIntervalSeconds (optional) rate limit interval seconds for image models, default is 600
	RateLimitImageModelsIntervalSeconds int `json:"rate_limit_image_models_interval_secs" mapstructure:"rate_limit_image_models_interval_secs"`
	// Proxy (optional) proxy url to send request
	Proxy string `json:"-" mapstructure:"proxy"`
	// UserTokens (optional) paid user's tenant tokens
	UserTokens []*UserConfig `json:"user_tokens" mapstructure:"user_tokens"`
	// GoogleAnalytics (optional) google analytics id
	GoogleAnalytics string `json:"ga" mapstructure:"ga"`
	// StaticLibs (optional) replace default static libs' url
	StaticLibs map[string]string `json:"static_libs" mapstructure:"static_libs"`
	// QAChatModels (optional) qa chat models
	QAChatModels []qaChatModel `json:"qa_chat_models" mapstructure:"qa_chat_models"`
	// ExternalBillingAPI (optional) default billing api, default is https://oneapi.laisky.com
	ExternalBillingAPI string `json:"external_billing_api" mapstructure:"external_billing_api"`
	// ExternalBillingToken (optional) default billing token
	ExternalBillingToken string `json:"external_billing_token" mapstructure:"external_billing_token"`
}

type qaChatModel struct {
	Name    string `json:"name" mapstructure:"name"`
	URL     string `json:"url" mapstructure:"url"`
	Project string `json:"project" mapstructure:"project"`
}

// ImageTokenType image token type
type ImageTokenType string

func (t ImageTokenType) String() string {
	return string(t)
}

const (
	// ImageTokenAzure azure image token
	ImageTokenAzure ImageTokenType = "azure"
	// ImageTokenOpenai openai image token
	ImageTokenOpenai ImageTokenType = "openai"
)

// UserConfig user config
type UserConfig struct {
	UserName string `json:"username" mapstructure:"username"`
	// Token (required) client's tenant token, not the openai token
	Token string `json:"-" mapstructure:"token"`
	// OpenaiToken (optional) openai token, default is global default token
	OpenaiToken string `json:"-" mapstructure:"openai_token"`
	// ImageToken (optional) token be used to generate image, default is global default image token
	ImageToken string `json:"-" mapstructure:"image_token"`
	// ImageTokenType (optional) token type, default is global default image token type
	ImageTokenType ImageTokenType `json:"-" mapstructure:"image_token_type"`
	// APIBase (optional) api base url, default is global default api base
	APIBase string `json:"api_base" mapstructure:"api_base"`
	// IsPaid whether is paid user
	IsPaid bool `json:"is_paid" mapstructure:"is_paid"`
	// AllowedModels (required) allowed models
	AllowedModels []string `json:"allowed_models" mapstructure:"allowed_models"`
	// NoLimitExpensiveModels (optional) skip rate limiter for expensive models
	NoLimitExpensiveModels bool `json:"no_limit_expensive_models" mapstructure:"no_limit_expensive_models"`
	// NoLimitAllModels (optional) skip rate limiter for all models
	NoLimitAllModels bool `json:"no_limit_all_models" mapstructure:"no_limit_all_models"`
	// NoLimitImageModels (optional) skip rate limiter for image models
	NoLimitImageModels bool `json:"no_limit_image_models" mapstructure:"no_limit_image_models"`
	// EnableExternalImageBilling (optional) enable external image billing
	EnableExternalImageBilling bool `json:"enable_external_image_billing" mapstructure:"enable_external_image_billing"`
	// ExternalImageBillingUID (optional) external image billing uid
	ExternalImageBillingUID string `json:"external_image_billing_uid" mapstructure:"external_image_billing_uid"`
}

// Valid valid and fill default values
func (c *UserConfig) Valid() error {
	if c.Token == "" {
		return errors.New("token is empty")
	}

	if c.UserName == "" {
		hashed := sha256.Sum256([]byte(c.Token))
		c.UserName = hex.EncodeToString(hashed[:])[:16]
	}

	if c.EnableExternalImageBilling {
		if c.ExternalImageBillingUID == "" {
			return errors.Errorf("%q's external_image_billing_uid should not be empty "+
				"if enable_external_image_billing is true", c.UserName)
		}
	}

	c.APIBase = gutils.OptionalVal(&c.APIBase, Config.API)
	c.OpenaiToken = gutils.OptionalVal(&c.OpenaiToken, Config.Token)
	c.ImageToken = gutils.OptionalVal(&c.ImageToken, Config.DefaultImageToken)
	c.ImageTokenType = gutils.OptionalVal(&c.ImageTokenType, Config.DefaultImageTokenType)

	return nil
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
			return errors.Errorf("too many requests for image model %q, "+
				"please try after %d seconds",
				model, (price - imageRateLimiter.Len()))
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
				return errors.Errorf("too many requests for expensive model %q, "+
					"please try after %d seconds",
					model, (price - expensiveModelRateLimiter.Len()))
			}
		}
	}

	return nil
}
