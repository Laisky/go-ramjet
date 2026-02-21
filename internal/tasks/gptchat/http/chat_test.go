package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/testify/require"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	gptTasks "github.com/Laisky/go-ramjet/internal/tasks/gptchat/tasks"
	"github.com/Laisky/go-ramjet/library/log"
)

func TestAPIHandler(t *testing.T) {
	if os.Getenv("RUN_GPT_HTTP_IT") == "" {
		t.Skip("integration test disabled: set RUN_GPT_HTTP_IT to run")
	}
	req := &FrontendReq{
		Model:  "gpt-4o-mini",
		Stream: true,
		Messages: []FrontendReqMessage{
			{
				Role:    OpenaiMessageRoleUser,
				Content: FrontendReqMessageContent{StringContent: "write a SSE client in golang"},
			},
		},
	}
	reqbody, err := json.Marshal(req)
	require.NoError(t, err)

	httpreq, err := http.NewRequest(http.MethodPost, "http://0.0.0.0:24456/api", bytes.NewReader(reqbody))
	require.NoError(t, err)

	httpreq.Header.Set("Content-Type", "application/json")
	httpreq.Header.Set("Accept", "text/event-stream")

	cli, err := gutils.NewHTTPClient(
		gutils.WithHTTPClientTimeout(time.Minute),
	)
	resp, err := cli.Do(httpreq)
	require.NoError(t, err)
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	lines := bytes.Split(body, []byte("\n"))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		resp := new(OpenaiCompletionStreamResp)
		// t.Logf("line: %q", string(line))
		err = json.Unmarshal(line, resp)
		require.NoError(t, err)

		if len(resp.Choices) == 0 || resp.Choices[0].FinishReason != "" {
			break
		}

		t.Logf("resp: %q", resp.Choices[0].Delta.Content)
	}
}

var testHTMLContent = `<!DOCTYPE html>
	<html>
		<head>
			<meta charset="UTF-8">
			<title>My HTML5 Document</title>
		</head>
		<body>
			<h1>Hello, world!</h1>
			<p>This is an example of an HTML5 document.</p>
		</body>
	</html>`

func Test_extractHTMLBody(t *testing.T) {
	if os.Getenv("RUN_GPT_HTTP_IT") == "" {
		t.Skip("integration test disabled: set RUN_GPT_HTTP_IT to run")
	}
	got, _, err := gptTasks.ExtractHTMLBody(context.Background(), "", []byte(testHTMLContent), "", false)
	require.NoError(t, err)
	require.Equal(t, "<body>\n\t\t\t<h1>Hello, world!</h1>\n\t\t\t<p>This is an example of an HTML5 document.</p>\n\t\t\n\t</body>", string(got))
}

func TestCountVisionImagePrice(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		resolution VisionImageResolution
		want       int
		wantErr    bool
	}{
		{
			name:       "100x100 low",
			width:      100,
			height:     100,
			resolution: VisionImageResolutionLow,
			want:       85,
			wantErr:    false,
		},
		{
			name:       "256x256 high",
			width:      256,
			height:     256,
			resolution: VisionImageResolutionHigh,
			want:       255 * VisionTokenPrice,
			wantErr:    false,
		},
		{
			name:       "1024x1024 high",
			width:      1024,
			height:     1024,
			resolution: VisionImageResolutionHigh,
			want:       765 * VisionTokenPrice,
			wantErr:    false,
		},
		{
			name:       "1024x2048 high",
			width:      1024,
			height:     2048,
			resolution: VisionImageResolutionHigh,
			want:       1105 * VisionTokenPrice,
			wantErr:    false,
		},
		{
			name:       "unsupported resolution",
			width:      1024,
			height:     1024,
			resolution: "unsupported",
			want:       0,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CountVisionImagePrice(tt.width, tt.height, tt.resolution)
			if (err != nil) != tt.wantErr {
				t.Errorf("%q error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("%q = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func Test_bodyChecker(t *testing.T) {
	if os.Getenv("RUN_GPT_HTTP_IT") == "" {
		t.Skip("integration test disabled: set RUN_GPT_HTTP_IT to run")
	}
	raw := `{"model":"gpt-3.5-turbo-1106","stream":true,"max_tokens":500,"temperature":1,"presence_penalty":0,"frequency_penalty":0,"messages":[{"role":"system","content":"The following is a conversation with Chat-GPT, an AI created by OpenAI. The AI is helpful, creative, clever, and very friendly, it's mainly focused on solving coding problems, so it likely provide code example whenever it can and every code block is rendered as markdown. However, it also has a sense of humor and can talk about anything. Please answer user's last question, and if possible, reference the context as much as you can."},{"role":"user","chatID":"chat-1705284240927-Hv8nTi","content":"1+2"},{"role":"user","content":"what is the temperature in shanghai,"}],"stop":["\n\n"],"laisky_extra":{"chat_switch":{"disable_https_crawler":false,"enable_google_search":true}}}`
	req := new(FrontendReq)
	err := json.Unmarshal([]byte(raw), req)
	require.NoError(t, err)
	require.Equal(t, "gpt-3.5-turbo-1106", req.Model)
	require.True(t, req.LaiskyExtra.ChatSwitch.EnableGoogleSearch)
}

func Test_functionCallsRegexp(t *testing.T) {
	text := "```python\nsearch_web(\"Ottawa ON Canada weather forecast this week\")\n```"
	matched := functionCallsRegexp.FindAllStringSubmatch(text, -1)

	require.Len(t, matched, 1)
	require.Len(t, matched[0], 2)
	require.Equal(t, matched[0][0], "search_web(\"Ottawa ON Canada weather forecast this week\")")
	require.Equal(t, matched[0][1], "Ottawa ON Canada weather forecast this week")
}

func TestSendAndParseChatGETHandlesUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{\"error\":\"boom\"}"))
	}))
	t.Cleanup(upstream.Close)

	originalCli := httpcli
	httpcli = upstream.Client()
	t.Cleanup(func() { httpcli = originalCli })

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		DefaultImageToken:                       "srv-image-token",
		DefaultImageUrl:                         upstream.URL + "/v1/images/generations",
		API:                                     strings.TrimRight(upstream.URL, "/"),
		RateLimitExpensiveModelsIntervalSeconds: 600,
		RamjetURL:                               "",
	}
	t.Cleanup(func() { config.Config = originalConfig })

	user := &config.UserConfig{
		Token:         "laisky-abcdefghijklmno",
		UserName:      "tester",
		APIBase:       strings.TrimRight(upstream.URL, "/"),
		OpenaiToken:   "sk-user",
		AllowedModels: []string{"*"},
	}
	require.NoError(t, user.Valid())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(ctxKeyUser, user)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(`{"model":"gpt-4.1","stream":false,"max_tokens":50,"messages":[{"role":"user","content":"hi"}]}`))
	ctx.Request.Header.Set("content-type", "application/json")
	ctx.Request.Header.Set("authorization", "Bearer "+user.Token)

	require.NotPanics(t, func() { _ = sendChatWithResponsesToolLoop(ctx) })
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "upstream")
}

func TestSaveLLMConservationNilReq(t *testing.T) {
	require.NotPanics(t, func() { saveLLMConservation(nil, "response", "") })
}

func TestConvert2UpstreamResponsesRequestGETReturnsPlaceholderFrontendReq(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		DefaultImageToken:                       "srv-image-token",
		DefaultImageUrl:                         "https://api.test/v1/images/generations",
		API:                                     "https://api.test",
		RateLimitExpensiveModelsIntervalSeconds: 600,
		RamjetURL:                               "",
	}
	t.Cleanup(func() { config.Config = originalConfig })

	user := &config.UserConfig{
		Token:         "laisky-abcdefghijklmno",
		UserName:      "tester",
		APIBase:       "https://api.test",
		OpenaiToken:   "sk-user",
		AllowedModels: []string{"*"},
	}
	require.NoError(t, user.Valid())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(ctxKeyUser, user)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/gptchat/api", nil)
	ctx.Request.Header.Set("authorization", "Bearer "+user.Token)

	frontendReq, gotUser, responsesReq, err := convert2UpstreamResponsesRequest(ctx)
	require.NoError(t, err)
	require.NotNil(t, frontendReq)
	require.Empty(t, frontendReq.Messages)
	require.NotNil(t, gotUser)
	require.NotNil(t, responsesReq)
	require.NotEmpty(t, responsesReq.Model)
}

func TestSendChatWithResponsesToolLoopMemoryEnabledRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var (
		mu       sync.Mutex
		requests []OpenAIResponsesReq
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req OpenAIResponsesReq
		require.NoError(t, json.Unmarshal(body, &req))
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","output_text":"ok","output":[]}`))
	}))
	t.Cleanup(upstream.Close)

	originalCli := httpcli
	httpcli = upstream.Client()
	t.Cleanup(func() { httpcli = originalCli })

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		DefaultImageToken:                       "srv-image-token",
		DefaultImageUrl:                         upstream.URL + "/v1/images/generations",
		API:                                     strings.TrimRight(upstream.URL, "/"),
		RateLimitExpensiveModelsIntervalSeconds: 600,
		RamjetURL:                               "",
		EnableMemory:                            true,
		MemoryProject:                           "gptchat",
		MemoryStorageMCPURL:                     "",
	}
	t.Cleanup(func() { config.Config = originalConfig })

	user := &config.UserConfig{
		Token:         "laisky-abcdefghijklmno",
		UserName:      "tester",
		APIBase:       strings.TrimRight(upstream.URL, "/"),
		OpenaiToken:   "sk-user",
		AllowedModels: []string{"*"},
	}
	require.NoError(t, user.Valid())

	firstReq := `{"model":"gpt-4.1","stream":false,"max_tokens":50,"messages":[{"role":"user","content":"my name is alice"}]}`
	secondReq := `{"model":"gpt-4.1","stream":false,"max_tokens":50,"messages":[{"role":"user","content":"what is my name"}]}`

	for _, raw := range []string{firstReq, secondReq} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set(ctxKeyUser, user)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(raw))
		ctx.Request.Header.Set("content-type", "application/json")
		ctx.Request.Header.Set("authorization", "Bearer "+user.Token)

		err := sendChatWithResponsesToolLoop(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, recorder.Code)
	}

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(requests), 2)

	secondInput, ok := requests[1].Input.([]any)
	require.True(t, ok)

	foundDeveloper := false
	for _, item := range secondInput {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if msg["role"] == "developer" {
			foundDeveloper = true
			break
		}
	}
	require.False(t, foundDeveloper)
}

func TestSendChatWithResponsesToolLoopMemoryDisabledNoInjection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var (
		mu       sync.Mutex
		requests []OpenAIResponsesReq
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req OpenAIResponsesReq
		require.NoError(t, json.Unmarshal(body, &req))
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","output_text":"ok","output":[]}`))
	}))
	t.Cleanup(upstream.Close)

	originalCli := httpcli
	httpcli = upstream.Client()
	t.Cleanup(func() { httpcli = originalCli })

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		DefaultImageToken:                       "srv-image-token",
		DefaultImageUrl:                         upstream.URL + "/v1/images/generations",
		API:                                     strings.TrimRight(upstream.URL, "/"),
		RateLimitExpensiveModelsIntervalSeconds: 600,
		RamjetURL:                               "",
		EnableMemory:                            false,
	}
	t.Cleanup(func() { config.Config = originalConfig })

	user := &config.UserConfig{
		Token:         "laisky-abcdefghijklmno",
		UserName:      "tester",
		APIBase:       strings.TrimRight(upstream.URL, "/"),
		OpenaiToken:   "sk-user",
		AllowedModels: []string{"*"},
	}
	require.NoError(t, user.Valid())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(ctxKeyUser, user)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(`{"model":"gpt-4.1","stream":false,"max_tokens":50,"messages":[{"role":"user","content":"hello"}]}`))
	ctx.Request.Header.Set("content-type", "application/json")
	ctx.Request.Header.Set("authorization", "Bearer "+user.Token)

	err := sendChatWithResponsesToolLoop(ctx)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 1)
	input, ok := requests[0].Input.([]any)
	require.True(t, ok)

	for _, item := range input {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		require.NotEqual(t, "developer", msg["role"])
	}
}

func TestSendChatWithResponsesToolLoopMemoryFailureNonFatal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","output_text":"ok","output":[]}`))
	}))
	t.Cleanup(upstream.Close)

	originalCli := httpcli
	httpcli = upstream.Client()
	t.Cleanup(func() { httpcli = originalCli })

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		DefaultImageToken:                       "srv-image-token",
		DefaultImageUrl:                         upstream.URL + "/v1/images/generations",
		API:                                     strings.TrimRight(upstream.URL, "/"),
		RateLimitExpensiveModelsIntervalSeconds: 600,
		RamjetURL:                               "",
		EnableMemory:                            true,
		MemoryProject:                           "gptchat",
		MemoryStorageMCPURL:                     "",
	}
	t.Cleanup(func() { config.Config = originalConfig })

	user := &config.UserConfig{
		Token:         "laisky-abcdefghijklmno",
		UserName:      "tester",
		APIBase:       strings.TrimRight(upstream.URL, "/"),
		OpenaiToken:   "sk-user",
		AllowedModels: []string{"*"},
	}
	require.NoError(t, user.Valid())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(ctxKeyUser, user)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(`{"model":"gpt-4.1","stream":false,"max_tokens":50,"messages":[{"role":"user","content":"hello"}]}`))
	ctx.Request.Header.Set("content-type", "application/json")
	ctx.Request.Header.Set("authorization", "Bearer "+user.Token)

	err := sendChatWithResponsesToolLoop(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "ok")
}
