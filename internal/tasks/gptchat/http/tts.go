package http

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/web"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/audio"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/speech"
	"github.com/gin-gonic/gin"
)

// TTSRequest is the request struct for text to speech
type TTSRequest struct {
	Text string `json:"text" binding:"required,min=1"`
}

// TTSStreamHanler text to speech by azure, will return audio stream
func TTSStreamHanler(ctx *gin.Context) {
	if config.Config.Azure.TTSKey == "" || config.Config.Azure.TTSRegion == "" {
		web.AbortErr(ctx, fmt.Errorf("azure tts key or region is empty"))
		return
	}

	req := new(TTSRequest)
	if err := ctx.BindJSON(req); web.AbortErr(ctx, errors.Wrap(err, "bind json")) {
		return
	}

	logger := gmw.GetLogger(ctx)
	azureTTSConfig, err := speech.NewSpeechConfigFromSubscription(config.Config.Azure.TTSKey, config.Config.Azure.TTSRegion)
	if web.AbortErr(ctx, errors.Wrap(err, "new speech config")) {
		return
	}
	defer azureTTSConfig.Close()

	if err = azureTTSConfig.SetSpeechSynthesisVoiceName("en-US-AvaMultilingualNeural"); web.AbortErr(ctx, errors.Wrap(err, "set voice name")) {
		return
	}
	if err = azureTTSConfig.SetSpeechSynthesisLanguage("English (Canada)"); web.AbortErr(ctx, errors.Wrap(err, "set language")) {
		return
	}

	audioCfg, err := audio.NewAudioConfigFromDefaultSpeakerOutput()
	if web.AbortErr(ctx, errors.Wrap(err, "new audio config")) {
		return
	}

	speechSynthesizer, err := speech.NewSpeechSynthesizerFromConfig(azureTTSConfig, audioCfg)
	if web.AbortErr(ctx, errors.Wrap(err, "new speech synthesizer")) {
		return
	}
	defer speechSynthesizer.Close()

	speechSynthesizer.SynthesisStarted(func(event speech.SpeechSynthesisEventArgs) {
		defer event.Close()
		logger.Debug("synthesis started")
	})
	speechSynthesizer.Synthesizing(func(event speech.SpeechSynthesisEventArgs) {
		defer event.Close()
	})
	speechSynthesizer.SynthesisCompleted(func(event speech.SpeechSynthesisEventArgs) {
		defer event.Close()
		logger.Debug("synthesis completed")
	})
	speechSynthesizer.SynthesisCanceled(func(event speech.SpeechSynthesisEventArgs) {
		defer event.Close()
		logger.Debug("synthesis canceled")
	})

	// StartSpeakingTextAsync sends the result to channel when the synthesis starts.
	task := speechSynthesizer.StartSpeakingTextAsync(req.Text)
	var outcome speech.SpeechSynthesisOutcome
	select {
	case outcome = <-task:
	case <-time.After(60 * time.Second):
		web.AbortErr(ctx, errors.New("timeout for speech synthesis"))
		return
	}
	defer outcome.Close()
	if web.AbortErr(ctx, errors.Wrap(outcome.Error, "outcome")) {
		return
	}

	// in most case we want to streaming receive the audio to lower the latency,
	// we can use AudioDataStream to do so.)
	stream, err := speech.NewAudioDataStreamFromSpeechSynthesisResult(outcome.Result)
	if web.AbortErr(ctx, errors.Wrap(err, "new audio data stream")) {
		return
	}
	defer stream.Close()

	fp, err := os.CreateTemp("", "tts-*.wav")
	if web.AbortErr(ctx, errors.Wrap(err, "create temp file")) {
		return
	}
	defer os.Remove(fp.Name())
	defer fp.Close()

	if err = <-stream.SaveToWavFileAsync(fp.Name()); web.AbortErr(ctx, errors.Wrap(err, "save to wav file")) {
		return
	}

	ctx.Header("Content-Type", "audio/wav")
	nBytes, err := io.Copy(ctx.Writer, fp)
	if web.AbortErr(ctx, errors.Wrap(err, "copy stream")) {
		return
	}

	logger.Info("tts audio succeed",
		zap.Int("text_len", len(req.Text)),
		zap.String("audio_size", gutils.HumanReadableByteCount(nBytes, true)))
}
