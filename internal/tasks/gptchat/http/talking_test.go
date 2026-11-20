package http

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/Laisky/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

func TestTranscript(t *testing.T) {
	if os.Getenv("RUN_GPT_HTTP_IT") == "" {
		t.Skip("integration test disabled: set RUN_GPT_HTTP_IT to run")
	}
	// The provider token comes from the environment; never commit credentials.
	token := os.Getenv("RAMJET_IT_ONEAPI_TOKEN")
	if token == "" {
		t.Skip("integration test needs RAMJET_IT_ONEAPI_TOKEN")
	}
	ctx := context.Background()

	err := SetupHTTPCli()
	require.NoError(t, err)

	fp, err := os.CreateTemp("", "test*.wav")
	require.NoError(t, err)
	defer os.Remove(fp.Name())
	defer fp.Close()

	// download test wav file
	{
		req, err := http.NewRequest(http.MethodGet,
			"https://s3.laisky.com/uploads/2024/06/tts_audio.wav", nil)
		require.NoError(t, err)
		resp, err := httpcli.Do(req)
		require.NoError(t, err)

		_, err = io.Copy(fp, resp.Body)
		require.NoError(t, err)

		_, err = fp.Seek(0, 0)
		require.NoError(t, err)
	}

	user := &config.UserConfig{
		APIBase:     "https://oneapi.laisky.com",
		OpenaiToken: token,
	}

	// Create a test request
	transcriptReq := &TranscriptRequest{
		File:  fp,
		Model: "whisper-large-v3",
	}

	resp, err := Transcript(ctx, user, transcriptReq)
	require.NoError(t, err)
	require.Contains(t, resp.Text, "Hello, beautiful world")
}
