package http

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/json"
	"github.com/Laisky/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

func TestAPIHandler(t *testing.T) {
	req := &FrontendReq{
		Model:  "gpt-3.5-turbo",
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

		resp := new(OpenaiCOmpletionStreamResp)
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
	got, err := extractHTMLBody([]byte(testHTMLContent))
	require.NoError(t, err)
	require.Equal(t, "<body>\n\t\t\t<h1>Hello, world!</h1>\n\t\t\t<p>This is an example of an HTML5 document.</p>\n\t\t\n\t</body>", string(got))
}
