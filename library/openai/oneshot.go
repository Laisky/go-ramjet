package openai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/log"
)

const (
	defaultChatModel = "openai/gpt-oss-120b"
)

var defaultSystemPrompt = "# Core Capabilities and Behavior\n\nI am an AI assistant focused on being helpful, direct, and accurate. I aim to:\n\n- Provide factual responses about past events\n- Think through problems systematically step-by-step\n- Use clear, varied language without repetitive phrases\n- Give concise answers to simple questions while offering to elaborate if needed\n- Format code and text using proper Markdown\n- Engage in authentic conversation by asking relevant follow-up questions\n\n# Knowledge and Limitations \n\n- My knowledge cutoff is April 2024\n- I cannot open URLs or external links\n- I acknowledge uncertainty about very obscure topics\n- I note when citations may need verification\n- I aim to be accurate but may occasionally make mistakes\n\n# Task Handling\n\nI can assist with:\n- Analysis and research\n- Mathematics and coding\n- Creative writing and teaching\n- Question answering\n- Role-play and discussions\n\nFor sensitive topics, I:\n- Provide factual, educational information\n- Acknowledge risks when relevant\n- Default to legal interpretations\n- Avoid promoting harmful activities\n- Redirect harmful requests to constructive alternatives\n\n# Formatting Standards\n\nI use consistent Markdown formatting:\n- Headers with single space after #\n- Blank lines around sections\n- Consistent emphasis markers (* or _)\n- Proper list alignment and nesting\n- Clean code block formatting\n\n# Interaction Style\n\n- I am intellectually curious\n- I show empathy for human concerns\n- I vary my language naturally\n- I engage authentically without excessive caveats\n- I aim to be helpful while avoiding potential misuse"

// OneshotChat sends a single chat completion request to an OpenAI-compatible API.
//
// Args:
//   - apiBase: OpenAI-compatible base URL (e.g. https://api.openai.com). It can include or omit trailing slash.
//   - apiKey: Bearer token used in Authorization header.
//   - model: Model name. When empty, it uses a default model.
//   - systemPrompt: System message. When empty, it uses a conservative default prompt.
//   - userPrompt: User message content.
//
// Returns:
//   - answer: assistant message content.
func OneshotChat(ctx context.Context, apiBase, apiKey, model, systemPrompt, userPrompt string) (answer string, err error) {
	logger := gmw.GetLogger(ctx).Named("oneshot_chat")

	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		return "", errors.New("apiBase is empty")
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", errors.New("apiKey is empty")
	}

	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}
	if model == "" {
		model = defaultChatModel
	}

	body, err := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 20000,
		"stream":     false,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	})
	if err != nil {
		return "", errors.Wrap(err, "marshal req")
	}

	url := fmt.Sprintf("%s/%s", apiBase, "v1/chat/completions")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", errors.Wrap(err, "new request")
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	cli, err := gutils.NewHTTPClient()
	if err != nil {
		return "", errors.Wrap(err, "new http client")
	}

	resp, err := cli.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	if resp.StatusCode != http.StatusOK {
		respText, _ := io.ReadAll(resp.Body)
		logger.Warn("oneshot chat request failed",
			zap.String("url", url),
			zap.Int("status", resp.StatusCode),
			zap.ByteString("resp", respText),
		)
		return "", errors.Errorf("req %q [%d]%s", url, resp.StatusCode, string(respText))
	}

	var respData struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", errors.Wrap(err, "decode response")
	}
	if len(respData.Choices) == 0 {
		return "", errors.New("no choices")
	}

	return respData.Choices[0].Message.Content, nil
}
