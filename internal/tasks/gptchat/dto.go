package gptchat

type OpenaiReqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type FrontendReq struct {
	Model            string             `json:"model"`
	MaxTokens        uint               `json:"max_tokens"`
	Messages         []OpenaiReqMessage `json:"messages,omitempty"`
	PresencePenalty  float64            `json:"presence_penalty"`
	FrequencyPenalty float64            `json:"frequency_penalty"`
	Stream           bool               `json:"stream"`
	Temperature      float64            `json:"temperature"`
	TopP             float64            `json:"top_p"`
	N                int                `json:"n"`
	Prompt           string             `json:"prompt,omitempty"`
	StaticContext    string             `json:"static_context,omitempty"`
}

type OpenaiChatReq struct {
	Model            string             `json:"model"`
	MaxTokens        uint               `json:"max_tokens"`
	Messages         []OpenaiReqMessage `json:"messages,omitempty"`
	PresencePenalty  float64            `json:"presence_penalty"`
	FrequencyPenalty float64            `json:"frequency_penalty"`
	Stream           bool               `json:"stream"`
	Temperature      float64            `json:"temperature"`
	TopP             float64            `json:"top_p"`
	N                int                `json:"n"`
}

type OpenaiCompletionReq struct {
	Model            string  `json:"model"`
	MaxTokens        uint    `json:"max_tokens"`
	PresencePenalty  float64 `json:"presence_penalty"`
	FrequencyPenalty float64 `json:"frequency_penalty"`
	Stream           bool    `json:"stream"`
	Temperature      float64 `json:"temperature"`
	TopP             float64 `json:"top_p"`
	N                int     `json:"n"`
	Prompt           string  `json:"prompt,omitempty"`
}
