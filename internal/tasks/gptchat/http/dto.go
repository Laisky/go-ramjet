package http

import (
	"encoding/json"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/zap"
	"github.com/pkoukk/tiktoken-go"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/library/log"
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
	defaultChatModel = "openai/gpt-oss-120b"
)

// ChatModel return chat model
func ChatModel() string {
	v := gconfig.Shared.GetString("openai.default_model")
	if v != "" {
		return v
	}

	return defaultChatModel
}

type LLMConservationReq struct {
	Model     string               `json:"model" binding:"required,min=1"`
	MaxTokens uint                 `json:"max_tokens" binding:"required,min=1"`
	Messages  []FrontendReqMessage `json:"messages" binding:"required,min=1"`
	Response  string               `json:"response" binding:"required,min=1"`
	Reasoning string               `json:"reasoning,omitempty"`
}

// FrontendReq request from frontend
type FrontendReq struct {
	Model            string               `json:"model"`
	MaxTokens        uint                 `json:"max_tokens"`
	Messages         []FrontendReqMessage `json:"messages,omitempty"`
	PresencePenalty  float64              `json:"presence_penalty"`
	FrequencyPenalty float64              `json:"frequency_penalty"`
	Stream           bool                 `json:"stream"`
	Temperature      float64              `json:"temperature"`
	TopP             float64              `json:"top_p"`
	N                int                  `json:"n"`
	Tools            []OpenaiChatReqTool  `json:"tools,omitempty"`
	ToolChoice       any                  `json:"tool_choice,omitempty"`
	EnableMCP        *bool                `json:"enable_mcp,omitempty"`
	MCPServers       []MCPServerConfig    `json:"mcp_servers,omitempty"`
	// ReasoningEffort constrains effort on reasoning for reasoning models, reasoning models only.
	ReasoningEffort string `json:"reasoning_effort,omitempty" binding:"omitempty,oneof=low medium high"`

	// -------------------------------------
	// Anthropic
	// -------------------------------------
	Thinking *Thinking `json:"thinking,omitempty"`

	// LaiskyExtra some special config for laisky
	LaiskyExtra *struct {
		ChatSwitch struct {
			// DisableHttpsCrawler disable https crawler
			DisableHttpsCrawler bool `json:"disable_https_crawler"`
			// EnableGoogleSearch enable google search
			EnableGoogleSearch bool `json:"enable_google_search"`
		} `json:"chat_switch"`
	} `json:"laisky_extra,omitempty"`
}

// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#implementing-extended-thinking
type Thinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens" binding:"omitempty,min=1024"`
}

// FrontendReqMessage request message from frontend
type FrontendReqMessage struct {
	Role    OpenaiMessageRole         `json:"role"`
	Content FrontendReqMessageContent `json:"content"`
	// Files send files with message
	Files []frontendReqMessageFiles `json:"files"`
}

// FrontendReqMessageContent is a custom type that can unmarshal from either a string or an array of OpenaiVisionMessageContent.
type FrontendReqMessageContent struct {
	StringContent string
	ArrayContent  []OpenaiVisionMessageContent
}

// UnmarshalJSON unmarshal from either a string or an array of OpenaiVisionMessageContent.
func (c *FrontendReqMessageContent) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '[' {
		return json.Unmarshal(data, &c.ArrayContent)
	}
	return json.Unmarshal(data, &c.StringContent)
}

// MarshalJSON marshal to either a string or an array of OpenaiVisionMessageContent.
func (c FrontendReqMessageContent) MarshalJSON() ([]byte, error) {
	if len(c.ArrayContent) > 0 {
		return json.Marshal(c.ArrayContent)
	}
	return json.Marshal(c.StringContent)
}

// String return string content
func (c FrontendReqMessageContent) String() string {
	if len(c.ArrayContent) > 0 {
		var s string
		for _, part := range c.ArrayContent {
			if part.Type == OpenaiVisionMessageContentTypeText {
				s += part.Text
			}
		}
		return s
	}
	return c.StringContent
}

// Append append string to content
func (c *FrontendReqMessageContent) Append(s string) {
	if len(c.ArrayContent) > 0 {
		c.ArrayContent = append(c.ArrayContent, OpenaiVisionMessageContent{
			Type: OpenaiVisionMessageContentTypeText,
			Text: s,
		})
	} else {
		c.StringContent += s
	}
}

type frontendReqMessageFiles struct {
	Type    string `json:"type" binding:"required,oneof=image"`
	Name    string `json:"name"`
	Content []byte `json:"content"`
}

// Tiktoken return tiktoken, could be nil if not found
func Tiktoken() *tiktoken.Tiktoken {
	tik, err := tiktoken.EncodingForModel("gpt-3.5-turbo")
	if err != nil {
		log.Logger.Warn("get tiktoken failed", zap.Error(err))
	}

	return tik
}

// PromptTokens count prompt tokens
func (r *FrontendReq) PromptTokens() (n int) {
	for _, msg := range r.Messages {
		n += CountTextTokens(msg.Content.String())
	}

	return n
}

// CountTextTokens returns the approximate token count for a string using tiktoken when available.
func CountTextTokens(text string) int {
	tik := Tiktoken()
	if tik != nil {
		return len(tik.Encode(text, nil, nil))
	}

	return len(text)
}

// OpenaiChatReq request to openai chat api
type OpenaiChatReq[T string | []OpenaiVisionMessageContent] struct {
	Model            string                `json:"model"`
	MaxTokens        uint                  `json:"max_tokens"`
	Messages         []OpenaiReqMessage[T] `json:"messages,omitempty"`
	PresencePenalty  float64               `json:"presence_penalty"`
	FrequencyPenalty float64               `json:"frequency_penalty"`
	Stream           bool                  `json:"stream"`
	Temperature      float64               `json:"temperature"`
	TopP             float64               `json:"top_p,omitempty"`
	N                *int                  `json:"n,omitempty"`
	// ReasoningEffort constrains effort on reasoning for reasoning models, reasoning models only.
	ReasoningEffort string              `json:"reasoning_effort,omitempty" binding:"omitempty,oneof=low medium high"`
	Tools           []OpenaiChatReqTool `json:"tools,omitempty"`
	ToolChoice      any                 `json:"tool_choice,omitempty"`

	// -------------------------------------
	// Anthropic
	// -------------------------------------
	Thinking *Thinking `json:"thinking,omitempty"`
}

// OpenaiChatReqTool define tools
//
//	{
//		"type": "function",
//		"function": {
//		  "name": "get_current_weather",
//		  "description": "Get the current weather in a given location",
//		  "parameters": {
//			"type": "object",
//			"properties": {
//			  "location": {
//				"type": "string",
//				"description": "The city and state, e.g. San Francisco, CA"
//			  },
//			  "unit": {
//				"type": "string",
//				"enum": [
//				  "celsius",
//				  "fahrenheit"
//				]
//			  }
//			},
//			"required": [
//			  "location"
//			]
//		  }
//		}
//	}
type OpenaiChatReqTool struct {
	Type     string              `json:"type"`
	Function OpenaiChatReqToolFn `json:"function,omitempty"`
	Strict   *bool               `json:"strict,omitempty"`
}

// OpenaiChatReqToolFn matches OpenAI chat-completions tool schema.
type OpenaiChatReqToolFn struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// OpenaiReqMessage request message to openai chat api
//
// chat completion message and vision message have different content
type OpenaiReqMessage[T string | []OpenaiVisionMessageContent] struct {
	Role    OpenaiMessageRole `json:"role"`
	Content T                 `json:"content"`
}

// OpenaiVisionMessageContentType vision message content type
type OpenaiVisionMessageContentType string

const (
	// OpenaiVisionMessageContentTypeText text
	OpenaiVisionMessageContentTypeText OpenaiVisionMessageContentType = "text"
	// OpenaiVisionMessageContentTypeImageUrl image url
	OpenaiVisionMessageContentTypeImageUrl OpenaiVisionMessageContentType = "image_url"
)

// VisionImageResolution image resolution
type VisionImageResolution string

const (
	// VisionImageResolutionLow low resolution
	VisionImageResolutionLow VisionImageResolution = "low"
	// VisionImageResolutionHigh high resolution
	VisionImageResolutionHigh VisionImageResolution = "high"
)

// OpenaiVisionMessageContentImageUrl image url
type OpenaiVisionMessageContentImageUrl struct {
	URL    string                `json:"url"`
	Detail VisionImageResolution `json:"detail,omitempty"`
}

// OpenaiVisionMessageContent vision message content
type OpenaiVisionMessageContent struct {
	Type     OpenaiVisionMessageContentType      `json:"type"`
	Text     string                              `json:"text,omitempty"`
	ImageUrl *OpenaiVisionMessageContentImageUrl `json:"image_url,omitempty"`
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
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
	} `json:"choices"`
}

// OpenaiCompletionStreamResp stream chunk return from openai chat api
//
//	{
//	    "id":"chatcmpl-6tCPrEY0j5l157mOIddZi4I0tIFhv",
//	    "object":"chat.completion.chunk",
//	    "created":1678613787,
//	    "model":"gpt-3.5-turbo-0301",
//	    "choices":[{"delta":{"role":"assistant"}, "index":0, "finish_reason":null}]
//	}
type OpenaiCompletionStreamResp struct {
	ID      string                             `json:"id"`
	Object  string                             `json:"object"`
	Created int64                              `json:"created"`
	Model   string                             `json:"model"`
	Choices []OpenaiCompletionStreamRespChoice `json:"choices"`
}

type OpenaiCompletionStreamRespChoice struct {
	Delta        OpenaiCompletionStreamRespDelta `json:"delta"`
	Index        int                             `json:"index"`
	FinishReason string                          `json:"finish_reason"`
}

type OpenaiCompletionStreamRespDelta struct {
	Role OpenaiMessageRole `json:"role"`
	// Content may be string or []StreamRespContent
	Content          any                                  `json:"content"`
	ReasoningContent string                               `json:"reasoning_content,omitempty"`
	Reasoning        string                               `json:"reasoning,omitempty"`
	ToolCalls        []OpenaiCompletionStreamRespToolCall `json:"tool_calls,omitempty"`
}

type StreamRespContent struct {
	Type     string   `json:"type"`
	ImageUrl ImageUrl `json:"image_url"`
}

type ImageUrl struct {
	Url string `json:"url"`
}

// OpenaiCompletionStreamRespToolCall tool call
//
//	{
//		"id": "call_abc123",
//		"type": "function",
//		"function": {
//		  "name": "get_current_weather",
//		  "arguments": "{\n\"location\": \"Boston, MA\"\n}"
//		}
//	}
type OpenaiCompletionStreamRespToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
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

// PromptTokens count prompt tokens
// func (r *DrawImageByTextRequest) PromptTokens() int {
// 	tik := Tiktoken()
// 	if tik != nil {
// 		return len(tik.Encode(r.Prompt, nil, nil))
// 	}

// 	return len(r.Prompt)
// }

// OneShotChatRequest request to one-shot chat api
type OneShotChatRequest struct {
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt" binding:"required,min=1"`
}
