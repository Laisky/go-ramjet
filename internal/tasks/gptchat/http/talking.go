package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	gutils "github.com/Laisky/go-utils/v4"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TranscriptRequest is the request struct for speech to text
type TranscriptRequest struct {
	File  multipart.File `form:"file" binding:"required"`
	Model string         `form:"model" binding:"required"`
}

type transcriptionSegment struct {
	ID               int     `json:"id"`
	Seek             float64 `json:"seek"`
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Text             string  `json:"text"`
	Tokens           []int   `json:"tokens"`
	Temperature      float64 `json:"temperature"`
	AvgLogprob       float64 `json:"avg_logprob"`
	CompressionRatio float64 `json:"compression_ratio"`
	NoSpeechProb     float64 `json:"no_speech_prob"`
}

type xGroq struct {
	ID string `json:"id"`
}

// TranscriptionResponse is the request struct for speech to text
type TranscriptionResponse struct {
	Task     string                 `json:"task"`
	Language string                 `json:"language"`
	Duration float64                `json:"duration"`
	Text     string                 `json:"text"`
	Segments []transcriptionSegment `json:"segments"`
	XGroq    xGroq                  `json:"x_groq"`
}

// Transcript transcribe audio to text
func Transcript(ctx context.Context, user *config.UserConfig, req *TranscriptRequest) (respData *TranscriptionResponse, err error) {
	logger := gmw.GetLogger(ctx)
	// if err := checkUserExternalBilling(ctx, user, db.PriceTTS, "tts"); err != nil {
	// 	return nil, errors.Wrap(err, "check user external billing")
	// }

	upstreamUrl := fmt.Sprintf("%s/v1/audio/transcriptions", user.APIBase)
	var requestBody bytes.Buffer
	multiPartWriter := multipart.NewWriter(&requestBody)

	fileWriter, err := multiPartWriter.CreateFormFile("file", "file.wav")
	if err != nil {
		return nil, errors.Wrap(err, "create form field file")
	}

	if _, err = io.Copy(fileWriter, req.File); err != nil {
		return nil, errors.Wrap(err, "copy file")
	}

	if err = multiPartWriter.WriteField("model", req.Model); err != nil {
		return nil, errors.Wrap(err, "write field model")
	}
	if err = multiPartWriter.WriteField("response_format", "verbose_json"); err != nil {
		return nil, errors.Wrap(err, "write field response_format")
	}
	if err = multiPartWriter.Close(); err != nil {
		return nil, errors.Wrap(err, "close multipart writer")
	}

	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamUrl, &requestBody)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}
	upstreamReq.Header.Set("Content-Type", multiPartWriter.FormDataContentType())
	upstreamReq.Header.Set("Authorization", "Bearer "+user.OpenaiToken)

	upstreamResp, err := httpcli.Do(upstreamReq)
	if err != nil {
		return nil, errors.Wrap(err, "do request")
	}
	defer gutils.CloseWithLog(upstreamResp.Body, logger)

	if upstreamResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(upstreamResp.Body)
		return nil, errors.Errorf("[%d]%s", upstreamResp.StatusCode, string(respBody))
	}

	respData = new(TranscriptionResponse)
	if err = json.NewDecoder(upstreamResp.Body).Decode(respData); err != nil {
		return nil, errors.Wrap(err, "decode response")
	}

	logger.Info("transcript success")
	return respData, nil
}
