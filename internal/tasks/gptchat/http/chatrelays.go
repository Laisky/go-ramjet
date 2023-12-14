package http

import "github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"

func convert2GoogleGeminiChat(user *config.UserConfig, frontendReq *FrontendReq) (url string, newReq any) {

	var (
		systemPrompt string
		userMessages []GeminiChatRequestInstanceMessage
	)

	for _, msg := range frontendReq.Messages {
		if msg.Role == OpenaiMessageRoleSystem {
			systemPrompt += msg.Content
			continue
		}

		userMessages = append(userMessages, GeminiChatRequestInstanceMessage{
			Content: msg.Content,
			Author:  GeminiAuthorUser,
		})
	}

	req := &GeminiChatRequest{
		Parameters: GeminiChatRequestParameters{
			Temperature:     frontendReq.Temperature,
			MaxOutputTokens: int(frontendReq.MaxTokens),
			TopP:            0.8,
			TopK:            40,
		},
		Instances: []GeminiChatRequestInstance{
			{
				Context:  systemPrompt,
				Messages: userMessages,
			},
		},
	}

	url = "POST https://us-central1-aiplatform.googleapis.com/v1/projects/chat-408106/locations/northamerica-northeast1/publishers/google/models/gemini-pro:streamGenerateContent"
}
