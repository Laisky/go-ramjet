package http

import (
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
)

const (
	ctxKeyUser     string = "ctx_user"
	ctxKeyUserAuth string = "ctx_user_auth"
)

// getRawUserToken returns the original user token from Authorization header.
//
// It is cached into gin context by getUserByAuthHeader.
func getRawUserToken(gctx *gin.Context) string {
	if gctx == nil {
		return ""
	}
	if v, ok := gctx.Get(ctxKeyUserAuth); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return strings.TrimSpace(strings.TrimPrefix(gctx.Request.Header.Get("authorization"), "Bearer "))
}

func getUserByAuthHeader(gctx *gin.Context) (user *config.UserConfig, err error) {
	if useri, ok := gctx.Get(ctxKeyUser); ok {
		return useri.(*config.UserConfig), nil
	}

	userToken := strings.TrimPrefix(gctx.Request.Header.Get("authorization"), "Bearer ")
	if userToken == "" {
		log.Logger.Debug("user token not found in header, use freetier token instead")
		userToken = config.FreetierUserToken
	}
	gctx.Set(ctxKeyUserAuth, strings.TrimSpace(userToken))

	user, err = getUserByToken(gctx, userToken)
	if err != nil {
		return nil, errors.Wrap(err, "get user by token")
	}

	gctx.Set(ctxKeyUser, user)
	return user, nil
}

// type oneapiUserResponse struct {
// 	TokenID  int    `json:"token_id"`
// 	UID      int    `json:"uid"`
// 	Username string `json:"username"`
// }

// var (
// 	cacheGetOneapiUserIDByToken sync.Map
// )

// func getOneapiUserIDByToken(ctx context.Context, token string) (uid string, err error) {
// 	// load from cache
// 	if v, ok := cacheGetOneapiUserIDByToken.Load(token); ok {
// 		return v.(string), nil //nolint: forcetypeassert
// 	}

// 	url := config.Config.ExternalBillingAPI + "/api/user/get-by-token"
// 	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
// 	if err != nil {
// 		return "", errors.Wrap(err, "new request")
// 	}

// 	req.Header.Add("Authorization", token)
// 	resp, err := httpcli.Do(req) //nolint: bodyclose
// 	if err != nil {
// 		return "", errors.Wrap(err, "do request")
// 	}
// 	defer gutils.LogErr(resp.Body.Close, log.Logger)
// 	if resp.StatusCode != http.StatusOK {
// 		return "", errors.Errorf("bad status code %d", resp.StatusCode)
// 	}

// 	var respData oneapiUserResponse
// 	if err = json.NewDecoder(resp.Body).Decode(&respData); err != nil {
// 		return "", errors.Wrap(err, "decode response")
// 	}

// 	uid = strconv.Itoa(respData.UID)
// 	cacheGetOneapiUserIDByToken.Store(token, uid)
// 	return uid, nil
// }

// var OpenaiModelList = []string{
// 	"gpt-3.5-turbo", "gpt-3.5-turbo-0301", "gpt-3.5-turbo-0613", "gpt-3.5-turbo-1106", "gpt-3.5-turbo-0125",
// 	"gpt-3.5-turbo-16k", "gpt-3.5-turbo-16k-0613",
// 	"gpt-3.5-turbo-instruct",
// 	"gpt-4", "gpt-4-0314", "gpt-4-0613", "gpt-4-1106-preview", "gpt-4-0125-preview",
// 	"gpt-4-32k", "gpt-4-32k-0314", "gpt-4-32k-0613",
// 	"gpt-4-turbo-preview", "gpt-4-turbo", "gpt-4-turbo-2024-04-09",
// 	"gpt-4o", "gpt-4o-2024-05-13", "gpt-4o-2024-08-06", "gpt-4o-2024-11-20", "chatgpt-4o-latest",
// 	"gpt-4o-mini", "gpt-4o-mini-2024-07-18",
// 	"gpt-4o-search-preview",
// 	"gpt-4o-mini-search-preview",
// 	"gpt-4-vision-preview",
// 	"text-embedding-ada-002", "text-embedding-3-small", "text-embedding-3-large",
// 	"text-curie-001", "text-babbage-001", "text-ada-001", "text-davinci-002", "text-davinci-003",
// 	"text-moderation-latest", "text-moderation-stable",
// 	"text-davinci-edit-001",
// 	"davinci-002", "babbage-002",
// 	"dall-e-2", "dall-e-3",
// 	"whisper-1",
// 	"tts-1", "tts-1-1106", "tts-1-hd", "tts-1-hd-1106",
// 	"o1", "o1-2024-12-17",
// 	"o1-preview", "o1-preview-2024-09-12",
// 	"o1-mini", "o1-mini-2024-09-12",
// }

func getUserByToken(gctx *gin.Context, userToken string) (user *config.UserConfig, err error) {
	if useri, ok := gctx.Get(ctxKeyUser); ok {
		return useri.(*config.UserConfig), nil
	}

	logger := gmw.GetLogger(gctx).Named("get_user_by_token")
	// ctx, cancel := context.WithTimeout(gmw.Ctx(gctx), time.Second*10)
	// defer cancel()

	userToken = strings.TrimSpace(strings.TrimPrefix(userToken, "Bearer "))
	if userToken == "" {
		return nil, errors.New("empty token")
	}

SWITCH_FOR_USER:
	switch {
	case strings.HasPrefix(userToken, "FREETIER-"),
		userToken == config.FreetierUserToken: // free userch
		if len(userToken) < 15 {
			return nil, errors.Errorf("invalid freetier token %q", userToken)
		}

		username := userToken[:15]
		logger.Debug("use server's freetier openai token",
			zap.String("token", userToken),
			zap.String("user", username))

		for _, commFreeUser := range config.Config.UserTokens {
			if commFreeUser.Token == config.FreetierUserToken {
				user = &config.UserConfig{}
				if err = copier.Copy(user, commFreeUser); err != nil {
					return nil, errors.Wrap(err, "copy free user")
				}

				user.IsFree = true
				user.UserName = username
				break SWITCH_FOR_USER
			}
		}

		return nil, errors.Errorf("can not find freetier user %q in settings",
			config.FreetierUserToken)
	case strings.HasPrefix(userToken, "laisky-"),
		strings.HasPrefix(userToken, "sk-"):
		if len(userToken) < 15 {
			return nil, errors.Errorf("invalid laisky's oneapi token %q", userToken)
		}

		username := userToken[:15]
		logger.Debug("use laisky's oneapi token", zap.String("user", username))
		user = &config.UserConfig{ // default to openai user
			UserName:    username,
			Token:       userToken,
			OpenaiToken: userToken,
			// Use user's own token for image generation & billing to avoid falling back to server's default.
			// This fixes a bug where image requests were billed to the server account because ImageToken was empty
			// and later filled by user.Valid() with the global default.
			ImageToken: userToken,
			BYOK:       true, // mark as bring-your-own-key for rate limiting & auditing logic
			// ImageTokenType:         config.Config.DefaultImageTokenType,
			// ImageUrl:               config.Config.DefaultImageUrl,
			AllowedModels:          []string{"*"},
			NoLimitExpensiveModels: true,
			APIBase:                "https://oneapi.laisky.com",
			// APIBase:  "http://100.97.108.34:3000",
			ImageUrl: "https://oneapi.laisky.com/v1/images/generations",
		}

		if strings.Contains(user.ImageUrl, "https://oneapi.laisky.com") {
			// billing by oneapi, no need to enable external billing
			user.EnableExternalImageBilling = false
			// user.NoLimitImageModels = true
		} else {
			// if oneapiUid, err := getOneapiUserIDByToken(ctx, userToken); err != nil {
			// 	logger.Error("get oneapi uid", zap.Error(err))
			// 	user.EnableExternalImageBilling = false
			// 	user.NoLimitImageModels = false
			// } else {
			// 	logger.Debug("get oneapi uid", zap.String("uid", oneapiUid))
			// 	user.EnableExternalImageBilling = true
			// 	// user.ExternalImageBillingUID = oneapiUid
			// 	user.NoLimitImageModels = true
			// }
		}
	default: // use server's token in settings
		return nil, errors.New("invalid token")
		// for _, u := range config.Config.UserTokens {
		// 	if u.Token == userToken {
		// 		logger.Debug("paid user", zap.String("user", u.UserName))
		// 		if err = u.Valid(); err != nil {
		// 			return nil, errors.Wrap(err, "valid paid user")
		// 		}

		// 		user = u
		// 		break SWITCH_FOR_USER
		// 	}
		// }

		// // use user's own openai/azure or whatever token
		// hashed := sha256.Sum256([]byte(userToken))
		// username := hex.EncodeToString(hashed[:])[:16]
		// logger.Debug("use user's own token", zap.String("user", username))
		// user = &config.UserConfig{ // default to openai user
		// 	UserName:               username,
		// 	Token:                  userToken,
		// 	OpenaiToken:            userToken,
		// 	ImageToken:             userToken,
		// 	ImageUrl:               "https://api.openai.com/v1/images/generations",
		// 	AllowedModels:          OpenaiModelList, // only allow openai models
		// 	NoLimitExpensiveModels: true,
		// 	// NoLimitOpenaiModels:    true,
		// 	// NoLimitImageModels:     true,
		// 	BYOK:    true,
		// 	APIBase: config.Config.API,
		// }

		// // only BYOK user can set api base
		// userApiBase := strings.TrimRight(gctx.Request.Header.Get("X-Laisky-Api-Base"), "/")
		// if userApiBase != "" {
		// 	user.APIBase = userApiBase

		// 	// set image url
		// 	switch {
		// 	case strings.Contains(userApiBase, "openai.azure.com"):
		// 		user.ImageUrl = userApiBase
		// 	case strings.Contains(userApiBase, "api.openai.com"):
		// 		user.ImageUrl = "https://api.openai.com/v1/images/generations"
		// 	default:
		// 		user.ImageUrl = fmt.Sprintf("%s/v1/images/generations", userApiBase)
		// 	}

		// 	logger.Debug("use user's own api base", zap.String("api_base", user.APIBase))
		// }
	}

	if err = user.Valid(); err != nil {
		return nil, errors.Wrap(err, "valid user")
	}

	return user, nil
}
