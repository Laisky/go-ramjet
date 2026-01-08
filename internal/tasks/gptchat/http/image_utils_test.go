package http

import (
	"testing"

	"github.com/Laisky/testify/require"
)

func Test_isImageModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"dall-e-3", true},
		{"black-forest-labs/flux-dev", true},
		{"flux-schnell", true},
		{"gpt-4o", false},
		{"gpt-3.5-turbo", false},
		{"google/imagen-3", true},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			require.Equal(t, tt.want, isImageModel(tt.model))
		})
	}
}

func TestExtractPromptFromMessages(t *testing.T) {
	req := &FrontendReq{
		Messages: []FrontendReqMessage{
			{Role: OpenaiMessageRoleUser, Content: FrontendReqMessageContent{StringContent: "first"}},
			{Role: OpenaiMessageRoleAI, Content: FrontendReqMessageContent{StringContent: "first resp"}},
			{Role: OpenaiMessageRoleUser, Content: FrontendReqMessageContent{StringContent: "second"}},
		},
	}

	var prompt string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == OpenaiMessageRoleUser {
			prompt = req.Messages[i].Content.String()
			break
		}
	}

	require.Equal(t, "second", prompt)
}
