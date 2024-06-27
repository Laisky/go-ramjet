package http

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
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

// TTSHanler text to speech by azure, will return audio stream
func TTSHanler(ctx *gin.Context) {
	if config.Config.Azure.TTSKey == "" || config.Config.Azure.TTSRegion == "" {
		web.AbortErr(ctx, fmt.Errorf("azure tts key or region is empty"))
		return
	}

	user, err := getUserByToken(ctx, ctx.Query("apikey"))
	if web.AbortErr(ctx, errors.Wrap(err, "get user by auth header")) {
		return
	}

	text, err := url.QueryUnescape(ctx.Query("text"))
	if web.AbortErr(ctx, errors.Wrap(err, "url.QueryUnescape")) {
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

	ssml, err := generateSSML(ctx.Request.Context(), user, text)
	if err != nil {
		logger.Warn("failed to generate ssml by llm", zap.Error(err))
		// use default ssml
		ssml = fmt.Sprintf(`<speak xmlns="http://www.w3.org/2001/10/synthesis" xmlns:mstts="http://www.w3.org/2001/mstts"
			xmlns:emo="http://www.w3.org/2009/10/emotionml" version="1.0" xml:lang="zh-CN">
			<voice name="zh-CN-XiaoxiaoNeural">
				<mstts:express-as style="gentle">%s</mstts:express-as>
			</voice>
		</speak>`, text)
	}

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

	// Create a temporary file with a .wav extension in the default temporary directory
	tempFp, err := os.CreateTemp("", "tts-*.wav")
	if web.AbortErr(ctx, errors.Wrap(err, "create temp file")) {
		return
	}
	defer os.Remove(tempFp.Name()) // Ensure the temporary file is removed after use
	defer tempFp.Close()

	// Use the temporary file's name (path) for saving
	if err = <-stream.SaveToWavFileAsync(tempFp.Name()); web.AbortErr(ctx, errors.Wrap(err, "save to wav file")) {
		return
	}

	ctx.Header("Content-Type", "audio/wav")
	nBytes, err := io.Copy(ctx.Writer, tempFp)
	if web.AbortErr(ctx, errors.Wrap(err, "copy stream")) {
		return
	}

	if nBytes < 1 {
		web.AbortErr(ctx, errors.New("failed to generate audio"))
		return
	}

	logger.Info("tts audio succeed",
		zap.Int("text_len", len(text)),
		zap.String("audio_size", gutils.HumanReadableByteCount(nBytes, true)))
}

var ssmlRegexp = regexp.MustCompile(`(?ims)(<speak.*</speak>)`)

func generateSSML(ctx context.Context, user *config.UserConfig, text string) (ssml string, err error) {
	prompt := `将下列文字生成为一段供语音输出的 SSML 格式，按照你的理解增加语音语调，仅输出可直接使用的 SSML 内容，不要输出任何其他字符。我会提供一段示例。` +
		"\n```\n" + `<speak xmlns="http://www.w3.org/2001/10/synthesis" xmlns:mstts="http://www.w3.org/2001/mstts"
			xmlns:emo="http://www.w3.org/2009/10/emotionml" version="1.0" xml:lang="zh-CN">
			<voice name="zh-CN-XiaoxiaoNeural">
				<mstts:express-as style="lyrical"><prosody rate="+3.00%">且说</prosody>宝玉<prosody
						rate="+3.00%">正和</prosody>宝钗玩笑，<prosody rate="+5.00%">忽见</prosody>人说-<prosody
						pitch="+5.00%">史</prosody><prosody rate="+4.00%">大</prosody>姑娘<prosody
						pitch="+5.00%">来</prosody>了。宝钗<prosody rate="+4.00%">笑道</prosody>：</mstts:express-as>
			</voice>
			<voice name="zh-CN-XiaohanNeural">
				<mstts:express-as style="cheerful">”等着，咱们两个一起走，<prosody rate="+5.00%" pitch="+6.00%">瞧瞧</prosody><prosody
						contour="(49%, -11%)">他</prosody><prosody rate="+6.00%">
						<phoneme alphabet="sapi" ph="qu 5">去</phoneme>
					</prosody>。”</mstts:express-as>
			</voice>
			<voice name="zh-CN-XiaoxiaoNeural">
				<mstts:express-as style="lyrical">说着，下了炕，和宝玉<mstts:ttsbreak strength="none" />来至贾母这边。<prosody
						rate="+5.00%">只</prosody><prosody rate="+7.00%">见</prosody>史湘云<prosody rate="+4.00%">
					大说大笑的</prosody>，见了他两个，忙站<prosody rate="+5.00%">起来</prosody>问好。<prosody rate="+5.00%">正值</prosody>
					黛玉在旁，因问宝玉：</mstts:express-as>
				<mstts:express-as style="gentle">“<prosody rate="+5.00%">打</prosody><prosody pitch="+3.00%">
					哪里</prosody>来？”</mstts:express-as>
				<mstts:express-as style="lyrical">宝玉<prosody rate="+4.00%">便</prosody><prosody
						pitch="+7.00%">说</prosody>：</mstts:express-as>
			</voice>
		</speak>` +
		"\n```\n" + `请转换下列内容
		>>
		` + text

	answer, err := OneshotChat(ctx, user, "", "", prompt)
	if err != nil {
		return "", errors.Wrap(err, "oneshot chat")
	}

	matched := ssmlRegexp.FindStringSubmatch(answer)
	if len(matched) < 2 {
		return "", errors.Errorf("failed to extract ssml %q", answer)
	}

	return matched[1], nil
}
