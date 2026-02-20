package http

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyFreeUserQuotaLimitsCapMaxTokensAndN(t *testing.T) {
	req := &FrontendReq{
		MaxTokens: 6000,
		N:         4,
	}

	droppedMessages, finalPromptTokens, changed, err := applyFreeUserQuotaLimits(req, 0)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 0, droppedMessages)
	require.Equal(t, 0, finalPromptTokens)
	require.Equal(t, uint(freeUserMaxTokens), req.MaxTokens)
	require.Equal(t, freeUserMaxResponses, req.N)
}

func TestApplyFreeUserQuotaLimitsDropOldestMessages(t *testing.T) {
	msg1 := FrontendReqMessage{Role: OpenaiMessageRoleSystem, Content: FrontendReqMessageContent{StringContent: "system rules"}}
	msg2 := FrontendReqMessage{Role: OpenaiMessageRoleUser, Content: FrontendReqMessageContent{StringContent: "oldest context message"}}
	msg3 := FrontendReqMessage{Role: OpenaiMessageRoleAI, Content: FrontendReqMessageContent{StringContent: "middle context message"}}
	msg4 := FrontendReqMessage{Role: OpenaiMessageRoleUser, Content: FrontendReqMessageContent{StringContent: "latest user question"}}

	req := &FrontendReq{
		MaxTokens: 100,
		N:         1,
		Messages:  []FrontendReqMessage{msg1, msg2, msg3, msg4},
	}

	limit := req.PromptTokens() - CountTextTokens(msg2.Content.String()) - CountTextTokens(msg3.Content.String())
	droppedMessages, finalPromptTokens, changed, err := applyFreeUserQuotaLimits(req, limit)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 2, droppedMessages)
	require.Len(t, req.Messages, 2)
	require.Equal(t, string(OpenaiMessageRoleSystem), req.Messages[0].Role.String())
	require.Equal(t, string(OpenaiMessageRoleUser), req.Messages[1].Role.String())
	require.Equal(t, msg1.Content.String(), req.Messages[0].Content.String())
	require.Equal(t, msg4.Content.String(), req.Messages[1].Content.String())
	require.LessOrEqual(t, finalPromptTokens, limit)
}

func TestApplyFreeUserQuotaLimitsNoChange(t *testing.T) {
	req := &FrontendReq{
		MaxTokens: 1024,
		N:         1,
		Messages: []FrontendReqMessage{
			{Role: OpenaiMessageRoleUser, Content: FrontendReqMessageContent{StringContent: "hello"}},
		},
	}

	limit := req.PromptTokens() + 10
	droppedMessages, finalPromptTokens, changed, err := applyFreeUserQuotaLimits(req, limit)

	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, 0, droppedMessages)
	require.Equal(t, req.PromptTokens(), finalPromptTokens)
	require.Equal(t, uint(1024), req.MaxTokens)
	require.Equal(t, 1, req.N)
	require.Len(t, req.Messages, 1)
}

func TestApplyFreeUserQuotaLimitsReturnErrorWhenOnlyProtectedMessagesRemain(t *testing.T) {
	systemMsg := FrontendReqMessage{Role: OpenaiMessageRoleSystem, Content: FrontendReqMessageContent{StringContent: "very long system instruction that costs many tokens"}}
	lastUserMsg := FrontendReqMessage{Role: OpenaiMessageRoleUser, Content: FrontendReqMessageContent{StringContent: "very long latest user prompt that also costs many tokens"}}

	req := &FrontendReq{
		MaxTokens: 100,
		N:         1,
		Messages:  []FrontendReqMessage{systemMsg, lastUserMsg},
	}

	limit := req.PromptTokens() - 1
	droppedMessages, finalPromptTokens, changed, err := applyFreeUserQuotaLimits(req, limit)

	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds free-user limit")
	require.False(t, changed)
	require.Equal(t, 0, droppedMessages)
	require.Equal(t, req.PromptTokens(), finalPromptTokens)
	require.Len(t, req.Messages, 2)
	require.Equal(t, string(OpenaiMessageRoleSystem), req.Messages[0].Role.String())
	require.Equal(t, string(OpenaiMessageRoleUser), req.Messages[1].Role.String())
}
