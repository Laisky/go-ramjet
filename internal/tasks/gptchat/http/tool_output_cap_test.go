package http

import (
	"context"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
)

func TestCapToolOutput_Summarize(t *testing.T) {
	old := oneshotChatForToolOutput
	defer func() { oneshotChatForToolOutput = old }()

	oneshootCalled := false
	oneshotChatForToolOutput = func(ctx context.Context, user *config.UserConfig, model, systemPrompt, userPrompt string) (string, error) {
		oneshootCalled = true
		return "summary", nil
	}

	ctx := gmw.SetLogger(context.Background(), log.Logger)
	user := &config.UserConfig{UserName: "u"}
	frontendReq := &FrontendReq{Messages: []FrontendReqMessage{{Role: OpenaiMessageRoleUser, Content: FrontendReqMessageContent{StringContent: "hello"}}}}
	big := strings.Repeat("a", maxToolOutputBytes+123)

	out, changed, err := capToolOutput(ctx, user, frontendReq, "mcp.tool", "{}", big)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, oneshootCalled)
	require.Equal(t, "summary", out)
}

func TestCapToolOutput_FallbackTruncate(t *testing.T) {
	old := oneshotChatForToolOutput
	defer func() { oneshotChatForToolOutput = old }()

	oneshotChatForToolOutput = func(ctx context.Context, user *config.UserConfig, model, systemPrompt, userPrompt string) (string, error) {
		return "", errors.New("boom")
	}

	ctx := gmw.SetLogger(context.Background(), log.Logger)
	user := &config.UserConfig{UserName: "u"}
	frontendReq := &FrontendReq{Messages: []FrontendReqMessage{{Role: OpenaiMessageRoleUser, Content: FrontendReqMessageContent{StringContent: "hello"}}}}
	big := strings.Repeat("b", maxToolOutputBytes+999)

	out, changed, err := capToolOutput(ctx, user, frontendReq, "mcp.tool", "{}", big)
	require.NoError(t, err)
	require.True(t, changed)
	require.LessOrEqual(t, len(out), maxToolOutputBytes)
	require.Contains(t, out, "[tool output truncated]")
}
