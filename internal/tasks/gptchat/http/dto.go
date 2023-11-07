package http

import (
	gconfig "github.com/Laisky/go-config/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
)

// OpenaiMessageRole message role
type OpenaiMessageRole string

// String return string
func (r OpenaiMessageRole) String() string {
	return string(r)
}

const (
	// OpenaiMessageRoleSystem system message
	OpenaiMessageRoleSystem = "system"
	// OpenaiMessageRoleUser user message
	OpenaiMessageRoleUser = "user"
	// OpenaiMessageRoleAI ai message
	OpenaiMessageRoleAI = "assistant"
)

const (
	// defaultMaxTokens   = 2000
	defaultMaxMessages = 7
	defaultChatModel   = "gpt-3.5-turbo"
)

// ChatModel return chat model
func ChatModel() string {
	v := gconfig.Shared.GetString("openai.default_model")
	if v != "" {
		return v
	}

	return defaultChatModel
}

// MaxTokens return max tokens
func MaxTokens() int {
	return gconfig.Shared.GetInt("openai.max_tokens")
}

// MaxMessages return max messages
func MaxMessages() int {
	v := gconfig.Shared.GetInt("openai.max_messages")
	if v != 0 {
		return v
	}

	return defaultMaxMessages
}

// OpenaiReqMessage request message to openai chat api
type OpenaiReqMessage struct {
	Role    OpenaiMessageRole `json:"role"`
	Content string            `json:"content"`
}

// FrontendReq request from frontend
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
	// StaticContext    string             `json:"static_context,omitempty"`
}

// OpenaiChatReq request to openai chat api
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

// OpenaiCompletionReq request to openai chat api
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

// nolint: lll
// OpenaiCompletionResp return from openai chat api
//
// https://platform.openai.com/docs/guides/chat/response-format
//
//	{
//		"id": "chatcmpl-6p9XYPYSTTRi0xEviKjjilqrWU2Ve",
//		"object": "chat.completion",
//		"created": 1677649420,
//		"model": "gpt-3.5-turbo",
//		"usage": {"prompt_tokens": 56, "completion_tokens": 31, "total_tokens": 87},
//		"choices": [
//		  {
//		   "message": {
//			 "role": "assistant",
//			 "content": "The 2020 World Series was played in Arlington, Texas at the Globe Life Field, which was the new home stadium for the Texas Rangers."},
//		   "finish_reason": "stop",
//		   "index": 0
//		  }
//		 ]
//	   }
type OpenaiCompletionResp struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Model  string `json:"model"`
	Usage  struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
	} `json:"choices"`
}

// OpenaiCOmpletionStreamResp stream chunk return from openai chat api
//
//	{
//	    "id":"chatcmpl-6tCPrEY0j5l157mOIddZi4I0tIFhv",
//	    "object":"chat.completion.chunk",
//	    "created":1678613787,
//	    "model":"gpt-3.5-turbo-0301",
//	    "choices":[{"delta":{"role":"assistant"}, "index":0, "finish_reason":null}]
//	}
type OpenaiCOmpletionStreamResp struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Role    OpenaiMessageRole `json:"role"`
			Content string            `json:"content"`
		} `json:"delta"`
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// ExternalBillingUserStatus user status
type ExternalBillingUserStatus int

const (
	// ExternalBillingUserStatusActive active
	ExternalBillingUserStatusActive ExternalBillingUserStatus = 1
)

// ExternalBillingUserResponse return from external billing api
type ExternalBillingUserResponse struct {
	Data struct {
		Status      ExternalBillingUserStatus `json:"status"`
		RemainQuota db.Price                  `json:"remain_quota"`
	} `json:"data"`
}

// OpenaiCreateImageRequest request to openai image api
type OpenaiCreateImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size"`
	Quality        string `json:"quality"`
	ResponseFormat string `json:"response_format"`
	Style          string `json:"style"`
}

// NewOpenaiCreateImageRequest create new request
func NewOpenaiCreateImageRequest(prompt string) *OpenaiCreateImageRequest {
	return &OpenaiCreateImageRequest{
		Model:          "dall-e-3",
		Prompt:         prompt,
		N:              1,
		Size:           "1024x1024",
		Quality:        "hd",
		ResponseFormat: "b64_json",
		Style:          "vivid",
	}
}

// OpenaiCreateImageResponse return from openai image api
type OpenaiCreateImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		Url     string `json:"url"`
		B64Json string `json:"b64_json"`
	} `json:"data"`
}

type ImageHandlerRequest struct {
	Prompt string `json:"prompt" binding:"required,min=1"`
	Model  string `json:"model" binding:"required,min=1"`
}
