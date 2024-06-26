package http

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
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

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, errors.Wrap(err, "get user by auth header")) {
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

	// if err = azureTTSConfig.SetSpeechSynthesisVoiceName("en-US-SaraNeural"); web.AbortErr(ctx, errors.Wrap(err, "set voice name")) {
	// 	return
	// }

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

	if err := checkUserExternalBilling(ctx.Request.Context(), user, db.PriceTTS, "tts"); web.AbortErr(ctx, errors.Wrap(err, "check user external billing")) {
		return
	}

	// StartSpeakingTextAsync sends the result to channel when the synthesis starts.
	ssml := fmt.Sprintf(`<speak xmlns="http://www.w3.org/2001/10/synthesis" xmlns:mstts="http://www.w3.org/2001/mstts"
			xmlns:emo="http://www.w3.org/2009/10/emotionml" version="1.0" xml:lang="zh-CN">
			<voice name="zh-CN-XiaoxiaoNeural">
				<mstts:express-as style="gentle">%s</mstts:express-as>
			</voice>
		</speak>`, req.Text)
	task := speechSynthesizer.StartSpeakingSsmlAsync(ssml)
	// task := speechSynthesizer.StartSpeakingTextAsync(req.Text)
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

	if outcome.Failed() {
		web.AbortErr(ctx, errors.Errorf("speech synthesis failed: %s", outcome.Error))
		return
	}

	tmpdir, err := os.MkdirTemp("", "tts-*.wav")
	if web.AbortErr(ctx, errors.Wrap(err, "create temp file")) {
		return
	}
	defer os.RemoveAll(tmpdir)

	fpath := filepath.Join(tmpdir, "tts.wav")
	if err = <-stream.SaveToWavFileAsync(fpath); web.AbortErr(ctx, errors.Wrap(err, "save to wav file")) {
		return
	}

	fp, err := os.Open(fpath)
	if web.AbortErr(ctx, errors.Wrap(err, "open wav file")) {
		return
	}

	ctx.Header("Content-Type", "audio/wav")
	nBytes, err := io.Copy(ctx.Writer, fp)
	if web.AbortErr(ctx, errors.Wrap(err, "copy stream")) {
		return
	}

	if nBytes < 1 {
		web.AbortErr(ctx, errors.New("failed to generate audio"))
		return
	}

	logger.Info("tts audio succeed",
		zap.Int("text_len", len(req.Text)),
		zap.String("audio_size", gutils.HumanReadableByteCount(nBytes, true)))
}
