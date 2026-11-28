package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/web"
)

const (
	maxSpeechInputCharacters = 4096
	maxSpeechAudioBytes      = 32 << 20
	speechRequestTimeout     = 60 * time.Second
)

// TTSHanler serves the legacy GPTChat GET contract and returns a WAV response
// synthesized by the configured OpenAI-compatible upstream.
func TTSHanler(gctx *gin.Context) {
	setNoStoreHeaders(gctx)
	user, err := getUserByToken(gctx, gctx.Query("apikey"))
	if web.AbortErr(gctx, errors.Wrap(err, "authenticate speech request")) {
		return
	}

	// Gin already unescapes the query value. Do not decode it a second time:
	// a literal % or + is valid text and must reach the speech provider intact.
	text := gctx.Query("text")
	if strings.TrimSpace(text) == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxSpeechInputCharacters {
		gctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Speech text must contain 1 to 4096 characters"})
		return
	}
	// Keep the historical application capability and limiter independent of the
	// actual speech model name the selected upstream bills.
	if err := IsModelAllowed(gctx, user, &FrontendReq{Model: "tts"}); web.AbortErr(gctx, err) {
		return
	}

	ctx, cancel := context.WithTimeout(gmw.Ctx(gctx), speechRequestTimeout)
	defer cancel()

	resp, err := requestSpeech(ctx, user, text)
	if web.AbortErr(gctx, err) {
		return
	}
	defer gutils.LogErr(resp.Body.Close, gmw.GetLogger(gctx))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		status := resp.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			gctx.Header("Retry-After", retryAfter)
		}
		gctx.AbortWithStatusJSON(status, gin.H{"error": "Speech upstream rejected the request"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSpeechAudioBytes+1))
	if web.AbortErr(gctx, errors.Wrap(err, "read speech audio")) {
		return
	}
	if len(body) < 12 || len(body) > maxSpeechAudioBytes ||
		string(body[:4]) != "RIFF" || string(body[8:12]) != "WAVE" {
		gctx.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "Speech upstream did not return bounded WAV audio"})
		return
	}
	gctx.Data(http.StatusOK, "audio/wav", body)
}

// requestSpeech submits text to the selected upstream using only the resolved
// user's selected credential, and returns the response for the caller to close.
func requestSpeech(ctx context.Context, user *config.UserConfig, text string) (*http.Response, error) {
	base, err := mediaInferenceBase(user)
	if err != nil {
		return nil, errors.Wrap(err, "select speech upstream")
	}
	payload, err := json.Marshal(struct {
		Model          string `json:"model"`
		Input          string `json:"input"`
		Voice          string `json:"voice"`
		ResponseFormat string `json:"response_format"`
	}{
		Model:          gutils.OptionalVal(&user.Voice.SpeechModel, config.DefaultVoiceSpeechModel),
		Input:          text,
		Voice:          gutils.OptionalVal(&user.Voice.SpeechVoice, config.DefaultVoiceSpeechVoice),
		ResponseFormat: "wav",
	})
	if err != nil {
		return nil, errors.Wrap(err, "encode speech request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/v1/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, errors.Wrap(err, "build speech request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+user.OpenaiToken)

	resp, err := mediaInferenceClient(user).Do(req) //nolint:bodyclose // The caller owns the returned response.
	if err != nil {
		return nil, errors.Wrap(err, "request speech")
	}
	return resp, nil
}
