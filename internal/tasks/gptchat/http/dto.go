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
	defaultChatModel = "gpt-3.5-turbo-0125"
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

	// LaiskyExtra some special config for laisky
	LaiskyExtra struct {
		ChatSwitch struct {
			// DisableHttpsCrawler disable https crawler
			DisableHttpsCrawler bool `json:"disable_https_crawler"`
			// EnableGoogleSearch enable google search
			EnableGoogleSearch bool `json:"enable_google_search"`
		} `json:"chat_switch"`
	} `json:"laisky_extra"`
}

// FrontendReqMessage request message from frontend
type FrontendReqMessage struct {
	Role    OpenaiMessageRole `json:"role"`
	Content string            `json:"content"`
	// Files send files with message
	Files []frontendReqMessageFiles `json:"files"`
}

type frontendReqMessageFiles struct {
	Type    string `json:"type" binding:"required,oneof=image"`
	Name    string `json:"name"`
	Content []byte `json:"content"`
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
	TopP             float64               `json:"top_p"`
	N                int                   `json:"n"`
	Tools            []OpenaiChatReqTool   `json:"tools,omitempty"`
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
	Type       string                      `json:"type"`
	Function   OpenaiChatReqToolFunction   `json:"function"`
	Parameters OpenaiChatReqToolParameters `json:"parameters"`
}

type OpenaiChatReqToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type OpenaiChatReqToolParameters struct {
	Type       string                      `json:"type"`
	Properties OpenaiChatReqToolProperties `json:"properties"`
	Required   []string                    `json:"required"`
}

type OpenaiChatReqToolProperties struct {
	Location OpenaiChatReqToolLocation `json:"location"`
	Unit     OpenaiChatReqToolUnit     `json:"unit"`
}

type OpenaiChatReqToolLocation struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type OpenaiChatReqToolUnit struct {
	Type string   `json:"type"`
	Enum []string `json:"enum"`
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
			Role    string `json:"role"`
			Content string `json:"content"`
		}
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
	Role      OpenaiMessageRole                    `json:"role"`
	Content   string                               `json:"content"`
	ToolCalls []OpenaiCompletionStreamRespToolCall `json:"tool_calls,omitempty"`
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

// OpenaiCreateImageRequest request to openai image api
type OpenaiCreateImageRequest struct {
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Style          string `json:"style,omitempty"`
}

// NewOpenaiCreateImageRequest create new request
func NewOpenaiCreateImageRequest(prompt string) *OpenaiCreateImageRequest {
	return &OpenaiCreateImageRequest{
		Model:  "dall-e-3",
		Prompt: prompt,
		N:      1,
		Size:   "1024x1024",
		// Quality:        "hd",  // price double
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

// AzureCreateImageResponse return from azure image api
type AzureCreateImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		RevisedPrompt string `json:"revised_prompt"`
		Url           string `json:"url"`
	} `json:"data"`
}

// DrawImageByTextRequest draw image by text and prompt
type DrawImageByTextRequest struct {
	Prompt string `json:"prompt" binding:"required,min=1"`
	Model  string `json:"model" binding:"required,min=1"`
}

// DrawImageByImageRequest draw image by image and prompt
type DrawImageByImageRequest struct {
	Prompt      string `json:"prompt" binding:"required,min=1"`
	Model       string `json:"model" binding:"required,min=1"`
	ImageBase64 string `json:"image_base64" binding:"required,min=1"`
}

// DrawImageByLcmRequest draw image by image and prompt with lcm
type DrawImageByLcmRequest struct {
	// Data consist of 6 strings:
	//  1. prompt,
	//  2. base64 encoded image with fixed prefix "data:image/png;base64,"
	//  3. steps
	//  4. cfg
	//  5. sketch strength
	//  6. seed
	Data    [6]any `json:"data"`
	FnIndex int    `json:"fn_index"`
}

type DrawImageBySdxlturboRequest struct {
	Model string `json:"model" binding:"required,min=1"`
	// Text prompt
	Text           string `json:"text" binding:"required,min=1"`
	NegativePrompt string `json:"negative_prompt"`
	ImageB64       string `json:"image"`
	// N how many images to generate
	N int `json:"n"`
}

type DrawImageBySdxlturboResponse struct {
	B64Images []string `json:"images"`
}

// DrawImageByLcmResponse draw image by image and prompt with lcm
type DrawImageByLcmResponse struct {
	// Data base64 encoded image with fixed prefix "data:image/png;base64,"
	Data            []string `json:"data"`
	IsGenerating    bool     `json:"is_generating"`
	Duration        float64  `json:"duration"`
	AverageDuration float64  `json:"average_duration"`
}
