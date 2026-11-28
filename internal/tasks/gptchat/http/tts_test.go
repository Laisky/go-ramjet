package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

const fixtureWAV = "RIFF\x04\x00\x00\x00WAVEfixture"

// TestSpeechDelegatesInference verifies that the legacy endpoint requests WAV
// audio without local billing and forwards either the operator or caller key.
func TestSpeechDelegatesInference(t *testing.T) {
	for _, tc := range []struct {
		name, key, wantAuth string
		free                bool
	}{
		{"managed", "FREETIER-fixture-tts", "Bearer sk-managed-speech-fixture", true},
		{"caller-owned", "sk-user-speech-fixture", "Bearer sk-user-speech-fixture", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			const input = "100% valid + unchanged"
			gctx, recorder := imageTestContextWithRecorder(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/v1/audio/speech" {
					t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if got := r.Header.Get("Authorization"); got != tc.wantAuth {
					t.Error("wrong upstream credential")
				}
				var body struct {
					Model          string `json:"model"`
					Input          string `json:"input"`
					Voice          string `json:"voice"`
					ResponseFormat string `json:"response_format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode speech request: %v", err)
				}
				if body.Model != "fixture-speech-model" || body.Input != input ||
					body.Voice != "fixture-voice" || body.ResponseFormat != "wav" {
					t.Errorf("unexpected speech parameters: %+v", body)
				}
				if _, err := w.Write([]byte(fixtureWAV)); err != nil {
					t.Errorf("write fixture WAV: %v", err)
				}
			})
			if tc.free {
				config.Config.UserTokens = []*config.UserConfig{{
					Token: config.FreetierUserToken, OpenaiToken: "sk-managed-speech-fixture",
					APIBase: config.Config.API, AllowedModels: []string{"tts"}, NoLimitExpensiveModels: true,
				}}
			} else {
				gctx.Set(ctxKeyUser, &config.UserConfig{
					BYOK: true, OpenaiToken: tc.key, APIBase: config.Config.API,
					Voice: config.VoiceConfig{SpeechModel: "fixture-speech-model", SpeechVoice: "fixture-voice"},
				})
			}
			config.Config.Voice.SpeechModel = "fixture-speech-model"
			config.Config.Voice.SpeechVoice = "fixture-voice"
			gctx.Request = httptest.NewRequest(http.MethodGet, "/audio/tts?"+
				(url.Values{"apikey": {tc.key}, "text": {input}}).Encode(), nil)
			TTSHanler(gctx)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Equal(t, "audio/wav", recorder.Header().Get("Content-Type"))
			require.Equal(t, fixtureWAV, recorder.Body.String())
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

// TestSpeechPreservesApplicationAdmission verifies disallowed free-tier speech
// is rejected before One API, and upstream rejection is returned without retry.
func TestSpeechPreservesApplicationAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		byok       bool
		wantStatus int
		wantCalls  int64
	}{
		{"free capability missing", "hello", false, http.StatusBadRequest, 0},
		{"empty text", "", true, http.StatusBadRequest, 0},
		{"overlong text", strings.Repeat("a", maxSpeechInputCharacters+1), true, http.StatusBadRequest, 0},
		{"upstream cooldown", "hello", true, http.StatusTooManyRequests, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			gctx, recorder := imageTestContextWithRecorder(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1/audio/speech" {
					t.Errorf("unexpected upstream route %s", r.URL.Path)
				}
				w.Header().Set("Retry-After", "10")
				http.Error(w, "synthetic upstream error", http.StatusTooManyRequests)
			})
			gctx.Set(ctxKeyUser, &config.UserConfig{
				UserName: "fixture", BYOK: tc.byok, IsFree: !tc.byok,
				AllowedModels: []string{"gpt-4o-mini"}, OpenaiToken: "sk-fixture", APIBase: config.Config.API,
			})
			gctx.Request = httptest.NewRequest(http.MethodGet, "/audio/tts?"+
				(url.Values{"text": {tc.text}}).Encode(), nil)
			TTSHanler(gctx)
			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			require.Equal(t, tc.wantCalls, calls.Load())
			if tc.wantStatus == http.StatusTooManyRequests {
				require.Equal(t, "10", recorder.Header().Get("Retry-After"))
			}
		})
	}
}
