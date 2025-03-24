// Package config implements config.
package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
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

	// if Config.ExternalBillingAPI != "" && Config.ExternalBillingToken == "" {
	// 	return errors.New("external_billing_token should not be empty " +
	// 		"if external_billing_api is set")
	// }

	// fill default
	Config.Gateway = gutils.OptionalVal(&Config.Gateway, "https://chat.laisky.com")
	Config.RateLimitExpensiveModelsIntervalSeconds = gutils.OptionalVal(
		&Config.RateLimitExpensiveModelsIntervalSeconds, 600)
	Config.RateLimitFreeModelsIntervalSeconds = gutils.OptionalVal(
		&Config.RateLimitFreeModelsIntervalSeconds, 1)
	Config.DefaultImageToken = gutils.OptionalVal(
		&Config.DefaultImageToken, Config.Token)
	// Config.DefaultImageTokenType = gutils.OptionalVal(
	// 	&Config.DefaultImageTokenType, ImageTokenOpenai)
	Config.API = trimUrl(gutils.OptionalVal(
		&Config.API, "https://api.openai.com"))
	Config.DefaultImageUrl = trimUrl(gutils.OptionalVal(
		&Config.DefaultImageUrl, Config.API+"/v1/images/generations"))
	Config.ExternalBillingAPI = trimUrl(gutils.OptionalVal(
		&Config.ExternalBillingAPI, "https://oneapi.laisky.com"))
	Config.RamjetURL = trimUrl(gutils.OptionalVal(
		&Config.RamjetURL, "https://app.laisky.com"))
	// Config.DefaultOpenaiToken = gutils.OptionalVal(
	// 	&Config.DefaultOpenaiToken, Config.Token)
	Config.LimitUploadFileBytes = gutils.OptionalVal(
		&Config.LimitUploadFileBytes, 20*1024*1024)

	// format normalize
	Config.API = strings.TrimRight(Config.API, "/")
	Config.ExternalBillingAPI = strings.TrimRight(Config.ExternalBillingAPI, "/")
	Config.RamjetURL = strings.TrimRight(Config.RamjetURL, "/")

	return nil
}

func trimUrl(url string) string {
	return strings.TrimRight(strings.TrimSpace(url), "/")
}

// OpenAI openai config
//
// nolint: lll
type OpenAI struct {
	// Gateway (optional) gateway url, default to https://chat.laisky.com
	Gateway string `json:"gateway" mapstructure:"gateway"`
	// API (optional) openai api base url, default is https://api.openai.com
	API string `json:"api" mapstructure:"api"`
	// Token (required) openai api request token
	Token string `json:"-" mapstructure:"token"`
	// DefaultOpenaiToken (optional) default openai token, default equals to token
	//
	// Dangerous: will escape paying wall, use it carefully
	// DefaultOpenaiToken string `json:"-" mapstructure:"default_openai_token"`
	// DefaultImageToken (optional) default image token, default equals to token
	DefaultImageToken string `json:"-" mapstructure:"default_image_token"`

	// DefaultImageTokenType (optional) default image token type, default is openai
	// DefaultImageTokenType ImageTokenType `json:"-" mapstructure:"default_image_token_type"`

	// DefaultImageUrl (optional) default image url
	//
	// default to https://{{API}}/v1/images/generations
	DefaultImageUrl string `json:"-" mapstructure:"default_image_url"`
	// RateLimitExpensiveModelsIntervalSeconds (optional) rate limit interval seconds for expensive models, default is 600
	RateLimitExpensiveModelsIntervalSeconds int `json:"rate_limit_expensive_models_interval_secs" mapstructure:"rate_limit_expensive_models_interval_secs"`
	// RateLimitFreeModelsIntervalSeconds (optional) rate limit interval seconds for free models, default is 1
	RateLimitFreeModelsIntervalSeconds int `json:"rate_limit_image_models_interval_secs" mapstructure:"rate_limit_image_models_interval_secs"`
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
	// ExternalBillingToken string `json:"external_billing_token" mapstructure:"external_billing_token"`
	// RamjetURL (optional) ramjet url
	RamjetURL string `json:"ramjet_url" mapstructure:"ramjet_url"`
	// S3 (optional) s3 config
	S3           s3Config `json:"s3" mapstructure:"s3"`
	NvidiaApikey string   `json:"-" mapstructure:"nvidia_apikey"`
	// ReplicateApikey is the api key for flux pro
	//
	// https://replicate.com/account/api-tokens
	ReplicateApikey string `json:"-" mapstructure:"replicate_apikey"`
	// SegmindApikey is the api key for segmind
	//
	// https://cloud.segmind.com/console/api-keys
	SegmindApikey string `json:"-" mapstructure:"segmind_apikey"`

	// PaymentStripeKey (optional) stripe key
	PaymentStripeKey string `json:"payment_stripe_key" mapstructure:"payment_stripe_key"`

	// LcmBasicAuthUsername (optional) lcm basic auth username
	LcmBasicAuthUsername string `json:"lcm_basic_auth_username" mapstructure:"lcm_basic_auth_username"`
	// LcmBasicAuthPassword (optional) lcm basic auth password
	LcmBasicAuthPassword string `json:"lcm_basic_auth_password" mapstructure:"lcm_basic_auth_password"`
	// LimitUploadFileBytes (optional) limit upload file bytes, default is 20MB
	LimitUploadFileBytes int `json:"limit_upload_file_bytes" mapstructure:"limit_upload_file_bytes"`

	// Azure (optional) azure config
	Azure azureConfig `json:"azure" mapstructure:"azure"`
}

type azureConfig struct {
	// TTSKey (optional) tts key
	TTSKey string `json:"tts_key" mapstructure:"tts_key"`
	// TTSRegion (optional) tts region
	TTSRegion string `json:"tts_region" mapstructure:"tts_region"`
}

type qaChatModel struct {
	Name    string `json:"name" mapstructure:"name"`
	URL     string `json:"url" mapstructure:"url"`
	Project string `json:"project" mapstructure:"project"`
}

type s3Config struct {
	Endpoint  string `json:"endpoint" mapstructure:"endpoint"`
	Bucket    string `json:"bucket" mapstructure:"bucket"`
	AccessID  string `json:"access_id" mapstructure:"access_id"`
	AccessKey string `json:"-" mapstructure:"access_key"`
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
	// ImageToken (optional) token be used to generate image,
	// default is global default image token
	ImageToken string `json:"-" mapstructure:"image_token"`
	// ImageUrl (optional) image url, default is global default image url
	ImageUrl string `json:"-" mapstructure:"image_url"`

	// ImageTokenType (optional) token type, default is global default image token type
	// ImageTokenType ImageTokenType `json:"-" mapstructure:"image_token_type"`

	// APIBase (optional) api base url, default is global default api base
	APIBase string `json:"api_base" mapstructure:"api_base"`
	// IsFree (optional) is free user, default is false
	IsFree bool `json:"is_free" mapstructure:"is_free"`
	// BYOK (optional) user's bring his own token, default is false
	BYOK bool `json:"byok" mapstructure:"byok"`
	// AllowedModels (required) allowed models
	AllowedModels []string `json:"allowed_models" mapstructure:"allowed_models"`
	// NoLimitExpensiveModels (optional) skip rate limiter for expensive models
	NoLimitExpensiveModels bool `json:"no_limit_expensive_models" mapstructure:"no_limit_expensive_models"`

	// NoLimitImageModels (optional) skip rate limiter for image models
	// NoLimitImageModels bool `json:"no_limit_image_models" mapstructure:"no_limit_image_models"`
	// NoLimitOpenaiModels (optional) skip rate limiter for models that only supported by openai
	// NoLimitOpenaiModels bool `json:"no_limit_openai_models" mapstructure:"no_limit_openai_models"`

	// LimitPromptTokenLength (optional) set limit for prompt token length, <=0 means no limit
	LimitPromptTokenLength int `json:"limit_prompt_token_length" mapstructure:"limit_prompt_token_length"`
	// EnableExternalImageBilling (optional) enable external image billing
	EnableExternalImageBilling bool `json:"enable_external_image_billing" mapstructure:"enable_external_image_billing"`
	// ExternalImageBillingUID (optional) external image billing uid
	// ExternalImageBillingUID string `json:"external_image_billing_uid" mapstructure:"external_image_billing_uid"`
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
		// if !strings.HasPrefix(c.OpenaiToken, "laisky-") {
		// 	return errors.Errorf("%q's openai_token should start with `laisky-` "+
		// 		"if enable_external_image_billing is true", c.UserName)

		// if c.ExternalImageBillingUID == "" {
		// 	return errors.Errorf("%q's external_image_billing_uid should not be empty "+
		// 		"if enable_external_image_billing is true", c.UserName)
		// }
	}

	// fill default
	c.APIBase = gutils.OptionalVal(&c.APIBase, Config.API)
	c.OpenaiToken = gutils.OptionalVal(&c.OpenaiToken, Config.Token)
	c.ImageToken = gutils.OptionalVal(&c.ImageToken, Config.DefaultImageToken)
	// c.ImageTokenType = gutils.OptionalVal(&c.ImageTokenType, Config.DefaultImageTokenType)
	c.ImageUrl = gutils.OptionalVal(&c.ImageUrl, Config.DefaultImageUrl)

	// format normalize
	c.APIBase = strings.TrimRight(c.APIBase, "/")

	return nil
}

var (
	onceLimiter                                     sync.Once
	freeModelRateLimiter, expensiveModelRateLimiter *gutils.RateLimiter
)

// setupRateLimiter setup ratelimiter depends on loaded config
func setupRateLimiter() {
	const burstRatio = 1.2
	var err error
	logger := log.Logger.Named("gptchat.ratelimiter")

	// {
	// 	if globalRatelimiter, err = gutils.NewRateLimiter(context.Background(),
	// 		gutils.RateLimiterArgs{
	// 			Max:     10,
	// 			NPerSec: 1,
	// 		}); err != nil {
	// 		log.Logger.Panic("new ratelimiter", zap.Error(err))
	// 	}
	// 	logger.Info("set overall ratelimiter", zap.Int("burst", 10))
	// }

	burst := int(float64(Config.RateLimitExpensiveModelsIntervalSeconds) * burstRatio)
	if expensiveModelRateLimiter, err = gutils.NewRateLimiter(context.Background(),
		gutils.RateLimiterArgs{
			Max:     burst,
			NPerSec: 1,
		}); err != nil {
		log.Logger.Panic("new expensiveModelRateLimiter", zap.Error(err))
	}
	logger.Info("set ratelimiter for expensive models", zap.Int("burst", burst))

	if freeModelRateLimiter, err = gutils.NewRateLimiter(context.Background(),
		gutils.RateLimiterArgs{
			Max:     3,
			NPerSec: 1,
		}); err != nil {
		log.Logger.Panic("new freeModelRateLimiter", zap.Error(err))
	}
	logger.Info("set ratelimiter for free models", zap.Int("burst", burst))
}

// IsModelAllowed check if model is allowed
//
// # Args
//   - model: model name
//   - nPromptTokens: the length of prompt tokens, 0 means no limit
func (c *UserConfig) IsModelAllowed(ctx context.Context, model string, nPromptTokens, maxTokens int) error {
	onceLimiter.Do(setupRateLimiter)

	logger := gmw.GetLogger(ctx)

	switch {
	case c.BYOK: // bypass if user bring their own token
		logger.Debug("bypass rate limit for BYOK user")
		return nil
	case c.NoLimitExpensiveModels:
		logger.Debug("bypass rate limit for no_limit_expensive_models user")
		return nil
	default:
	}

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

	// if !globalRatelimiter.Allow() { // check rate limit
	// 	return errors.Errorf("too many requests, please try again later")
	// }

	// rate limit only support limit by second,
	// so we consume 60 tokens once to make it limit by minute
	var (
		ratelimitCost int
		ratelimiter   = expensiveModelRateLimiter
	)
	switch model {
	case "gpt-3.5-turbo", // free models
		// "gpt-3.5-turbo-1106",
		// "gpt-3.5-turbo-0125",
		"gpt-4o-mini",
		// "llama2-70b-4096",
		"deepseek-chat",
		// "deepseek-coder",
		"gemma2-9b-it",
		"llama3-8b-8192",
		"llama3-70b-8192",
		"llama-3.1-8b-instant",
		"llama-3.1-405b-instruct",
		"llama-3.3-70b-versatile",
		"qwen-qwq-32b",
		// "mixtral-8x7b-32768",
		// "img-to-img",
		// "sdxl-turbo",
		"tts",
		"gemini-2.0-flash":
		ratelimiter = freeModelRateLimiter
		ratelimitCost = 1
	default: // expensive model
		if c.NoLimitExpensiveModels {
			return nil
		}

		ratelimitCost = gconfig.Shared.GetInt("openai.rate_limit_expensive_models_interval_secs")
	}

	if !c.NoLimitExpensiveModels {
		if c.LimitPromptTokenLength > 0 && nPromptTokens > c.LimitPromptTokenLength {
			return errors.Errorf(
				"The length of the prompt you submitted is %d, exceeds the limit for free users %d, "+
					"you need upgrade to a paid membership to use longer prompt tokens, "+
					"more info at https://wiki.laisky.com/projects/gpt/pay/cn/",
				nPromptTokens, c.LimitPromptTokenLength)
		}

		if maxTokens > 500 {
			return errors.New("max_tokens is limited to 500 for free users, " +
				"you need upgrade to a paid membership to use larger max_tokens, " +
				"more info at https://wiki.laisky.com/projects/gpt/pay/cn/")
		}
	}

	// if price less than 0, means no limit
	logger.Debug("check rate limit",
		zap.String("model", model), zap.Int("price", ratelimitCost))
	if ratelimitCost > 0 && !ratelimiter.AllowN(ratelimitCost) { // check rate limit
		return errors.Errorf("This model(%s) restricts usage for free users. "+
			"Please hold on for %d seconds before trying again, "+
			"alternatively, you may opt to switch to the free gpt-4o-mini, "+
			"or upgrade to a paid membership by https://wiki.laisky.com/projects/gpt/pay/cn/",
			model, (ratelimitCost - ratelimiter.Len()))
	}

	return nil
}
