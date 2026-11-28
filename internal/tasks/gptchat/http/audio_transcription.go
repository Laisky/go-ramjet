package http

import (
	"bytes"
	"crypto/subtle"
	"io"
	"mime"
	"mime/multipart"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/web"
)

const (
	gptchatTranscriptionModel = "whisper-1"
	transcriptionFileLimit    = 25 << 20
	transcriptionBodyLimit    = transcriptionFileLimit + (64 << 10)
)

// AudioTranscriptionHandler handles GPTChat free-tier transcription separately
// from the caller-token proxy. It also keeps custom-provider voice requests on
// the explicitly selected provider, using the existing caller egress safeguards.
func AudioTranscriptionHandler(ctx *gin.Context) {
	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}
	switch {
	case user.IsFree && user.OpenaiToken != "":
		// The dedicated free-tier capability always uses platform One API.
	case user.BYOK && user.APIBaseUserSelected:
		token, ok := callerOneAPIToken(ctx.Request.Header)
		if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(user.OpenaiToken)) != 1 {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	default:
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "A configured free-tier session or caller-owned provider is required"})
		return
	}

	base, err := mediaInferenceBase(user)
	if web.AbortErr(ctx, err) {
		return
	}

	body := http.MaxBytesReader(ctx.Writer, ctx.Request.Body, transcriptionBodyLimit)
	contentType := ctx.GetHeader("Content-Type")
	payload, err := io.ReadAll(body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Audio upload exceeds the size limit"})
			return
		}
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Unable to read audio upload"})
		return
	}
	if !validTranscriptionForm(contentType, payload) {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Expected one audio file and the whisper-1 model"})
		return
	}

	if err = IsModelAllowed(ctx, user, &FrontendReq{Model: gptchatTranscriptionModel}); web.AbortErr(ctx, err) {
		return
	}

	const path = "/v1/audio/transcriptions"
	req, err := http.NewRequestWithContext(gmw.Ctx(ctx), http.MethodPost, base+path, bytes.NewReader(payload))
	if web.AbortErr(ctx, err) {
		return
	}
	req.Header.Set("Content-Type", contentType)
	relayModelRequest(ctx, req, req.URL, mediaInferenceClient(user), user.OpenaiToken)
}

// validTranscriptionForm accepts exactly one nonempty bounded file and one
// literal model field; duplicate or alternate model selections are rejected.
func validTranscriptionForm(contentType string, payload []byte) bool {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return false
	}
	reader := multipart.NewReader(bytes.NewReader(payload), params["boundary"])
	var foundFile, foundModel bool
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return foundFile && foundModel
		}
		if err != nil {
			return false
		}
		switch part.FormName() {
		case "file":
			if foundFile || part.FileName() == "" {
				return false
			}
			size, err := io.Copy(io.Discard, part)
			if err != nil || size == 0 || size > transcriptionFileLimit {
				return false
			}
			foundFile = true
		case "model":
			if foundModel || part.FileName() != "" {
				return false
			}
			model, err := io.ReadAll(io.LimitReader(part, int64(len(gptchatTranscriptionModel)+1)))
			if err != nil || string(model) != gptchatTranscriptionModel {
				return false
			}
			foundModel = true
		default:
			return false
		}
	}
}
