package http

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/testify/require"
)

func TestTranscript(t *testing.T) {
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
		OpenaiToken: "laisky-k24tal3x3eN6SjKhD78e85Fc4dD648F1B0781aF435455642", // no balance
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
