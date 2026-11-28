package http

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

const (
	testCallerOneAPIToken = "sk-SYNTHETIC-CALLER-ONEAPI-KEY"
	testSharedOneAPIToken = "sk-SYNTHETIC-SHARED-GPTCHAT-KEY"
)

// TestOneAPIProxyRequiresCallerToken checks local rejection at the public handler.
func TestOneAPIProxyRequiresCallerToken(t *testing.T) {
	for _, tc := range []struct {
		name          string
		authorization []string
		sameSession   bool
		altHeader     string
	}{
		{name: "missing"},
		{name: "empty", authorization: []string{""}},
		{name: "empty Bearer", authorization: []string{"Bearer "}},
		{name: "free tier", authorization: []string{"Bearer FREETIER-synthetic-token"}},
		{name: "legacy free token", authorization: []string{config.FreetierUserToken}},
		{name: "sync key", authorization: []string{"sync-synthetic-account-key"}},
		{name: "session like", authorization: []string{"Bearer eyJhbGciOiJIUzI1NiJ9.synthetic.signature"}},
		{name: "same separate session", authorization: []string{testCallerOneAPIToken}, sameSession: true},
		{name: "multiple headers", authorization: []string{testCallerOneAPIToken, testCallerOneAPIToken}},
		{name: "multiple values", authorization: []string{testCallerOneAPIToken + ", " + testCallerOneAPIToken}},
		{name: "alternate credential", authorization: []string{testCallerOneAPIToken}, altHeader: "X-Api-Key"},
		{name: "wrong scheme", authorization: []string{"Basic " + testCallerOneAPIToken}},
		{name: "malformed token", authorization: []string{testCallerOneAPIToken + " trailing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.UserTokens = []*config.UserConfig{{Token: config.FreetierUserToken, OpenaiToken: testSharedOneAPIToken}}
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				return byokAuditResponse(req, "{}"), nil
			})}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/gptchat/oneapi/v1/models", nil)
			for _, value := range tc.authorization {
				c.Request.Header.Add("Authorization", value)
			}
			if tc.altHeader != "" {
				c.Request.Header.Set(tc.altHeader, "synthetic-other-key")
			}
			if tc.sameSession {
				c.Request.Header.Set(ramjetSessionHeader, "Bearer "+testCallerOneAPIToken)
			}
			OneapiProxyHandler(c)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Zero(t, calls)
			require.NotContains(t, w.Body.String(), testSharedOneAPIToken)
		})
	}
}

// TestOneAPIProxyPreservesUpstreamContract checks credential selection and
// unchanged status, model lists, body and retry metadata for independent users.
func TestOneAPIProxyPreservesUpstreamContract(t *testing.T) {
	for _, tc := range []struct {
		method, path, query, auth, request, response string
		status                                       int
	}{
		{http.MethodGet, "/v1/models", "?scope=one%2Ftwo", testCallerOneAPIToken, "", `{"data":[{"id":"not-in-gptchat"},{"id":"gpt-4o-mini"}]}`, 200},
		{http.MethodPost, "/v1/chat/completions", "", "bEaReR " + testCallerOneAPIToken, `{"model":"not-in-gptchat"}`, `{"id":"one"}`, 200},
		{http.MethodPost, "/v1/chat/completions", "", "Bearer " + testCallerOneAPIToken, "{}", `{"error":"invalid key"}`, 401},
		{http.MethodPost, "/v1/chat/completions", "", "Bearer " + testCallerOneAPIToken, "{}", `{"error":"model disabled"}`, 403},
		{http.MethodPost, "/v1/chat/completions", "", "Bearer " + testCallerOneAPIToken, "{}", `{"error":"insufficient quota"}`, 402},
		{http.MethodPost, "/v1/chat/completions", "", "Bearer " + testCallerOneAPIToken, "{}", `{"error":"rate limited"}`, 429},
		{http.MethodPost, "/v1/chat/completions", "", "Bearer " + testCallerOneAPIToken, "{}", "", 307},
	} {
		t.Run(fmt.Sprint(tc.method, tc.path, "/", tc.status), func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.UserTokens = []*config.UserConfig{{Token: config.FreetierUserToken, OpenaiToken: testSharedOneAPIToken, AllowedModels: []string{"gpt-4o-mini"}}}
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "https://oneapi.laisky.com"+tc.path+tc.query, req.URL.String())
				require.Equal(t, "Bearer "+testCallerOneAPIToken, req.Header.Get("Authorization"))
				require.Empty(t, req.Header.Get(ramjetSessionHeader))
				require.Empty(t, req.Header.Get("Cookie"))
				require.Empty(t, req.Header.Get("X-Laisky-Api-Base"))
				if tc.request != "" {
					payload, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.Equal(t, tc.request, string(payload))
				}
				resp := byokAuditResponse(req, tc.response)
				resp.StatusCode = tc.status
				resp.Header.Set("X-Oneapi-Request-Id", "upstream-id")
				resp.Header.Set("Retry-After", "42")
				if tc.status == http.StatusTemporaryRedirect {
					resp.Header.Set("Location", "https://fixture.invalid/other")
				}
				return resp, nil
			})}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(tc.method, "/gptchat/oneapi"+tc.path+tc.query, strings.NewReader(tc.request))
			c.Request.Header.Set("Authorization", tc.auth)
			c.Request.Header.Set("Cookie", "session=synthetic")
			c.Request.Header.Set(ramjetSessionHeader, "Bearer synthetic-separate")
			c.Request.Header.Set("X-Laisky-Api-Base", "https://fixture.invalid")
			OneapiProxyHandler(c)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.response, w.Body.String())
			require.Equal(t, "42", w.Header().Get("Retry-After"))
			require.Equal(t, "upstream-id", w.Header().Get("X-Oneapi-Request-Id"))
			require.Equal(t, 1, calls)
		})
	}
}

// TestGPTChatAudioTranscriptionAdmission checks that the dedicated route
// uses shared credentials only for a configured, permitted transcription.
func TestGPTChatAudioTranscriptionAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, token, model, allowed string
		duplicate, oversized        bool
		status, calls               int
	}{
		{"allowed free tier", "FREETIER-synthetic-browser", "whisper-1", "whisper-1", false, false, 200, 1},
		{"not in allowlist", "FREETIER-synthetic-browser", "whisper-1", "gpt-4o-mini", false, false, 400, 0},
		{"unsupported model", "FREETIER-synthetic-browser", "other", "*", false, false, 400, 0},
		{"duplicate field", "FREETIER-synthetic-browser", "whisper-1", "*", true, false, 400, 0},
		{"oversized", "FREETIER-synthetic-browser", "whisper-1", "*", false, true, 413, 0},
		{"caller token uses proxy instead", testCallerOneAPIToken, "whisper-1", "*", false, false, 401, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.RateLimitExpensiveModelsIntervalSeconds = 600
			config.Config.UserTokens = []*config.UserConfig{{
				Token: config.FreetierUserToken, OpenaiToken: testSharedOneAPIToken,
				AllowedModels: []string{tc.allowed},
			}}
			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)
			file, err := writer.CreateFormFile("file", "sample.webm")
			require.NoError(t, err)
			_, err = io.WriteString(file, "synthetic audio data")
			require.NoError(t, err)
			require.NoError(t, writer.WriteField("model", tc.model))
			if tc.duplicate {
				require.NoError(t, writer.WriteField("model", tc.model))
			}
			require.NoError(t, writer.Close())
			if tc.oversized {
				buf.Write(make([]byte, transcriptionBodyLimit+1))
			}
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/audio/transcriptions", &buf)
			ctx.Request.Header.Set("Authorization", "Bearer "+tc.token)
			ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "https://oneapi.laisky.com/v1/audio/transcriptions", req.URL.String())
				require.Equal(t, "Bearer "+testSharedOneAPIToken, req.Header.Get("Authorization"))
				return byokAuditResponse(req, `{"text":"synthetic"}`), nil
			})}
			AudioTranscriptionHandler(ctx)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Equal(t, tc.calls, calls)
		})
	}
}
