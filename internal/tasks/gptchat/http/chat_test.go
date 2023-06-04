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
)

func TestAPIHandler(t *testing.T) {
	req := &FrontendReq{
		Model:  "gpt-3.5-turbo",
		Stream: true,
		Messages: []OpenaiReqMessage{
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
	defer resp.Body.Close()

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
