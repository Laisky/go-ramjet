package http

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/testify/require"

	gptTasks "github.com/Laisky/go-ramjet/internal/tasks/gptchat/tasks"
	"github.com/Laisky/go-ramjet/library/log"
)

func TestAPIHandler(t *testing.T) {
	req := &FrontendReq{
		Model:  "gpt-4o-mini",
		Stream: true,
		Messages: []FrontendReqMessage{
			{
				Role:    OpenaiMessageRoleUser,
				Content: "write a SSE client in golang",
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
	got, err := gptTasks.ExtractHTMLBody([]byte(testHTMLContent))
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
