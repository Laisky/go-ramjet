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
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/audio"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/speech"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/library/openai"
	"github.com/Laisky/go-ramjet/library/web"
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

	if err = IsModelAllowed(ctx.Request.Context(),
		user, &FrontendReq{Model: "tts"}); web.AbortErr(ctx,
		errors.Wrap(err, "check model allowed")) {
		return
	}

	text, err := url.QueryUnescape(ctx.Query("text"))
	if web.AbortErr(ctx, errors.Wrap(err, "url.QueryUnescape")) {
		return
	}

	logger := gmw.GetLogger(ctx)
	speechSynthesizer, releaseSynthesizer, err := newTTSSynthesizer()
	if web.AbortErr(ctx, err) {
		return
	}
	// The native synthesizer is released when the handler returns, unless an
	// abandoned synthesis still uses it; the late outcome's cleanup releases it then.
	releaseOnReturn := true
	defer func() {
		if releaseOnReturn {
			releaseSynthesizer()
		}
	}()

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

	if err := checkUserExternalBilling(gmw.Ctx(ctx), user, db.PriceTTS, "tts"); web.AbortErr(ctx, errors.Wrap(err, "check user external billing")) {
		return
	}

	ssml, err := generateSSML(gmw.Ctx(ctx), user, text)
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
	stopped := make(chan error, 1)
	outcome, err := awaitAsyncOutcome(gmw.Ctx(ctx), task, ttsSynthesisTimeout, func(late speech.SpeechSynthesisOutcome) {
		// Once the abandoned outcome and the stop request have both completed,
		// nothing uses the synthesizer any more and every native handle can be freed.
		stopErr := <-stopped
		logger.Debug("released abandoned speech synthesis",
			zap.NamedError("outcome_error", late.Error), zap.NamedError("stop_error", stopErr))
		late.Close()
		releaseSynthesizer()
	})
	if err != nil {
		releaseOnReturn = false
		// Ask the SDK to stop the abandoned synthesis so a hung native call returns;
		// its result channel is unbuffered and must be received.
		go func() { stopped <- <-speechSynthesizer.StopSpeakingAsync() }()
		web.AbortErr(ctx, errors.Wrap(err, "speech synthesis"))
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

// ttsSynthesisTimeout bounds how long the handler waits for synthesis to start; it
// is a variable so tests can shorten it.
var ttsSynthesisTimeout = 60 * time.Second

// ttsSynthesizer is the part of *speech.SpeechSynthesizer that TTSHanler drives, so
// tests can substitute a fake for the native SDK.
type ttsSynthesizer interface {
	StartSpeakingSsmlAsync(ssml string) chan speech.SpeechSynthesisOutcome
	StopSpeakingAsync() chan error
	SynthesisStarted(handler speech.SpeechSynthesisEventHandler)
	Synthesizing(handler speech.SpeechSynthesisEventHandler)
	SynthesisCompleted(handler speech.SpeechSynthesisEventHandler)
	SynthesisCanceled(handler speech.SpeechSynthesisEventHandler)
}

// newTTSSynthesizer creates the synthesizer used by TTSHanler; it is a package
// variable so tests can substitute a fake.
var newTTSSynthesizer = newAzureTTSSynthesizer

// newAzureTTSSynthesizer creates the Azure speech config, the audio config and the
// speech synthesizer used by TTSHanler.
//
// It returns the synthesizer and a release func that frees all three native handles
// in reverse creation order. Call release exactly once, after every outcome the
// synthesizer produced has been closed. On error nothing is left to release.
func newAzureTTSSynthesizer() (ttsSynthesizer, func(), error) {
	speechCfg, err := speech.NewSpeechConfigFromSubscription(config.Config.Azure.TTSKey, config.Config.Azure.TTSRegion)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new speech config")
	}

	// A fixed voice can be pinned here with speechCfg.SetSpeechSynthesisVoiceName
	// (e.g. "en-US-SaraNeural"); the SSML currently selects the voice instead.

	audioCfg, err := audio.NewAudioConfigFromDefaultSpeakerOutput()
	if err != nil {
		speechCfg.Close()
		return nil, nil, errors.Wrap(err, "new audio config")
	}

	synthesizer, err := speech.NewSpeechSynthesizerFromConfig(speechCfg, audioCfg)
	if err != nil {
		audioCfg.Close()
		speechCfg.Close()
		return nil, nil, errors.Wrap(err, "new speech synthesizer")
	}

	release := func() {
		synthesizer.Close()
		audioCfg.Close()
		speechCfg.Close()
	}
	return synthesizer, release, nil
}

// awaitAsyncOutcome waits for the single value an Azure Speech SDK async call
// delivers on task. The SDK sends that value from its own goroutine on an
// unbuffered channel, so the channel must be received from even after the caller
// gives up; otherwise the SDK goroutine blocks forever and the value's native
// handle is never freed.
//
// Parameters:
//   - ctx: Request context; its cancellation stops the wait.
//   - task: Channel returned by the SDK async call.
//   - timeout: Maximum time to wait for the value.
//   - release: Frees a value that arrives after the wait was abandoned. It runs on
//     a background goroutine.
//
// It returns the value when it arrives in time; the caller then owns it. Otherwise
// it returns an error and hands the late value to release once the SDK delivers it.
func awaitAsyncOutcome[T any](ctx context.Context, task <-chan T, timeout time.Duration, release func(T)) (T, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var waitErr error
	select {
	case outcome := <-task:
		return outcome, nil
	case <-timer.C:
		waitErr = errors.Errorf("timeout after %s", timeout)
	case <-ctx.Done():
		waitErr = errors.Wrap(context.Cause(ctx), "request canceled")
	}

	go func() {
		release(<-task)
	}()

	var zero T
	return zero, waitErr
}

var ssmlRegexp = regexp.MustCompile(`(?ims)(<speak.*</speak>)`)
var zhRegexp = regexp.MustCompile(`[\p{Han}]{3,}`)

const (
	ttsPromptCN = `Please generate a segment of SSML formatted text for voice output, incorporating appropriate intonation based on your understanding.
		Only output the directly usable SSML content, excluding any other characters. I will provide an example.
		Please pay attention to retaining all "speak", "voice", and "mstts" metadata.
		Regardless of the language, the "voice name" should always be set to "zh-CN-XiaoxiaoNeural" and not be changed.
		Your response should start from "<speak xmlns".` +
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
		"\n```\n" + `Please convert the following content:
		>>
		`
	ttsPromptEN = `Please generate a segment of SSML formatted text for voice output, incorporating appropriate intonation based on your understanding.
		Only output the directly usable SSML content, excluding any other characters. I will provide an example.
		Please pay attention to retaining all "speak", "voice", and "mstts" metadata.
		Regardless of the language, the "voice name" should always be set to "en-US-JaneNeural" and not be changed. Your response should start from "<speak xmlns".` +
		"\n```\n" + `<speak xmlns="http://www.w3.org/2001/10/synthesis" xmlns:mstts="http://www.w3.org/2001/mstts"
			xmlns:emo="http://www.w3.org/2009/10/emotionml" version="1.0" xml:lang="en-US">
			<voice name="en-US-JaneNeural">
				<s />
				<mstts:express-as style="cheerful">Good morning, Contoso restaurant. I am your AI assistant,
					Jane. How can I help you?</mstts:express-as>
				<s />
			</voice>
			<voice name="en-US-TonyNeural">
				<s />
				<mstts:express-as style="friendly">Hi<break strength="weak" />, I would like to make a
					dinner reservation.</mstts:express-as>
				<s />
			</voice>
			<voice name="en-US-JaneNeural">
				<s />
				<mstts:express-as style="cheerful">Of course, what evening will you be joining us on?</mstts:express-as>
				<s />
			</voice>
			<voice name="en-US-TonyNeural">We will need the reservation for Thursday night.</voice>
			<voice name="en-US-JaneNeural">
				<s />
				<mstts:express-as style="cheerful">
					<prosody rate="+20.00%">And what time would you like the reservation for?</prosody>
				</mstts:express-as>
				<s />
			</voice>
			<voice name="en-US-TonyNeural">We would prefer 7:00 or 7:30.</voice>
			<voice name="en-US-JaneNeural">
				<s />
				<mstts:express-as style="cheerful">
					<prosody rate="+10.00%">Sounds good!</prosody>
					<prosody rate="-5.00%"> And for how many people?</prosody>
				</mstts:express-as>
				<s />
			</voice>
			<voice name="en-US-TonyNeural"><s />There will be 5 of us.<s /></voice>
			<voice name="en-US-JaneNeural">
				<s />
				<mstts:express-as style="cheerful">Fine, I can seat you at 7:00 on Thursday, if you would
					kindly give me your name.</mstts:express-as>
				<s />
			</voice>
			<voice name="en-US-TonyNeural">
				<mstts:express-as style="friendly">The last name is Wood. W-O-O-D, Wood.</mstts:express-as>
				<s />
			</voice>
			<voice name="en-US-JaneNeural">
				<s />
				<mstts:express-as style="excited">See you at 7:00 this Thursday, Mr. Wood.</mstts:express-as>
				<s />
			</voice>
			<voice name="en-US-TonyNeural">
				<s />
				<mstts:express-as style="Default">Thank you.</mstts:express-as>
				<s />
			</voice>
		</speak>` +
		"\n```\n" + `Please convert the following content:
		>>
		`
)

// containsChinese check if string contains chinese
func containsChinese(s string) bool {
	return zhRegexp.MatchString(s)
}

// generateSSML generate ssml by llm
func generateSSML(ctx context.Context, user *config.UserConfig, text string) (ssml string, err error) {
	logger := gmw.GetLogger(ctx)
	var prompt string
	if containsChinese(text) {
		prompt = ttsPromptCN + text
	} else {
		prompt = ttsPromptEN + text
	}

	answer, err := openai.OneshotChat(ctx, user.APIBase, user.OpenaiToken, "",
		"follow my order, return exactly what I asked",
		prompt)
	if err != nil {
		return "", errors.Wrap(err, "oneshot chat")
	}

	matched := ssmlRegexp.FindStringSubmatch(answer)
	if len(matched) < 2 {
		return "", errors.Errorf("failed to extract ssml %q", answer)
	}

	logger.Debug("extracted ssml", zap.String("ssml", matched[1]))
	return matched[1], nil
}
