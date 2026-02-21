package memoryx

import (
	"context"
	"net/http"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/agents/files"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-utils/v6/agents/memory"
)

type stubEngine struct {
	beforeOut memory.BeforeTurnOutput
	afterIn   memory.AfterTurnInput
}

type stubBeforeErrEngine struct {
	err error
}

func (e *stubEngine) BeforeTurn(_ context.Context, _ memory.BeforeTurnInput) (memory.BeforeTurnOutput, error) {
	return e.beforeOut, nil
}

func (e *stubEngine) AfterTurn(_ context.Context, in memory.AfterTurnInput) error {
	e.afterIn = in
	return nil
}

func (e *stubBeforeErrEngine) BeforeTurn(_ context.Context, _ memory.BeforeTurnInput) (memory.BeforeTurnOutput, error) {
	return memory.BeforeTurnOutput{}, e.err
}

func (e *stubBeforeErrEngine) AfterTurn(_ context.Context, _ memory.AfterTurnInput) error {
	return nil
}

func TestBeforeAfterTurnHookRoundTrip(t *testing.T) {
	engineCacheMu.Lock()
	engineCache = map[string]memory.Engine{}
	engineCacheMu.Unlock()

	conf := &config.OpenAI{EnableMemory: true, MemoryProject: "gptchat", MemoryStorageMCPURL: "https://mcp.example.com"}
	user := &config.UserConfig{UserName: "alice", APIBase: "https://oneapi.laisky.com", OpenaiToken: "sk-abc"}
	cacheKey := buildEngineCacheKey(conf, user)
	st := &stubEngine{
		beforeOut: memory.BeforeTurnOutput{InputItems: []memory.ResponseItem{{
			Type: "message",
			Role: "developer",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "memory block",
			}},
		}}},
	}

	engineCacheMu.Lock()
	engineCache[cacheKey] = st
	engineCacheMu.Unlock()

	turn1Input := []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "my name is alice"}}}}
	before1, err := BeforeTurnHook(context.Background(), conf, user, http.Header{}, turn1Input, 120000)
	require.NoError(t, err)
	require.True(t, before1.Enabled)
	require.NotEmpty(t, before1.Keys.TurnID)
	require.NoError(t, AfterTurnHook(context.Background(), conf, user, before1.Keys, before1.PreparedInput, "Nice to meet you"))
	require.NotEmpty(t, st.afterIn.OutputItems)
	require.Equal(t, "assistant", st.afterIn.OutputItems[0].Role)
}

func TestAfterTurnHookBadKeys(t *testing.T) {
	conf := &config.OpenAI{EnableMemory: true, MemoryStorageMCPURL: "https://mcp.example.com", MemoryProject: "gptchat"}
	user := &config.UserConfig{UserName: "alice", APIBase: "https://oneapi.laisky.com", OpenaiToken: "sk-abc"}
	err := AfterTurnHook(context.Background(), conf, user, RuntimeKeys{}, []any{}, "hi")
	require.Error(t, err)
}

func TestBeforeTurnHookInjectsDeveloperMessage(t *testing.T) {
	conf := &config.OpenAI{
		EnableMemory:        true,
		MemoryProject:       "gptchat",
		MemoryStorageMCPURL: "https://mcp.example.com",
	}
	user := &config.UserConfig{UserName: "alice", APIBase: "https://oneapi.laisky.com", OpenaiToken: "sk-abc"}
	cacheKey := buildEngineCacheKey(conf, user)

	engineCacheMu.Lock()
	engineCache = map[string]memory.Engine{
		cacheKey: &stubEngine{beforeOut: memory.BeforeTurnOutput{InputItems: []memory.ResponseItem{
			{
				Type: "message",
				Role: "developer",
				Content: []memory.ResponseContentPart{{
					Type: "input_text",
					Text: "memory recalled",
				}},
			},
		}}},
	}
	engineCacheMu.Unlock()

	before, err := BeforeTurnHook(context.Background(), conf, user, http.Header{}, []any{}, 120000)
	require.NoError(t, err)
	require.Len(t, before.PreparedInput, 1)
	msg := before.PreparedInput[0].(map[string]any)
	require.Equal(t, "developer", msg["role"])
}

func TestBeforeTurnHookColdStartFallbackOnNotFound(t *testing.T) {
	conf := &config.OpenAI{
		EnableMemory:        true,
		MemoryProject:       "gptchat",
		MemoryStorageMCPURL: "https://mcp.example.com",
	}
	user := &config.UserConfig{UserName: "alice", APIBase: "https://oneapi.laisky.com", OpenaiToken: "sk-abc"}
	cacheKey := buildEngineCacheKey(conf, user)

	errNotFound := errors.Wrap(
		&files.ToolError{Code: files.ErrorCodeNotFound, Message: "path not found"},
		"load tier files",
	)
	engineCacheMu.Lock()
	engineCache = map[string]memory.Engine{cacheKey: &stubBeforeErrEngine{err: errNotFound}}
	engineCacheMu.Unlock()

	input := []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hello"}}}}
	before, err := BeforeTurnHook(context.Background(), conf, user, http.Header{}, input, 120000)
	require.NoError(t, err)
	require.True(t, before.Enabled)
	require.True(t, before.ColdStartFallback)
	require.Len(t, before.PreparedInput, 1)
	msg := before.PreparedInput[0].(map[string]any)
	require.Equal(t, "user", msg["role"])
}

func TestBeforeTurnHookReturnsErrorOnNonNotFound(t *testing.T) {
	conf := &config.OpenAI{
		EnableMemory:        true,
		MemoryProject:       "gptchat",
		MemoryStorageMCPURL: "https://mcp.example.com",
	}
	user := &config.UserConfig{UserName: "alice", APIBase: "https://oneapi.laisky.com", OpenaiToken: "sk-abc"}
	cacheKey := buildEngineCacheKey(conf, user)

	engineCacheMu.Lock()
	engineCache = map[string]memory.Engine{cacheKey: &stubBeforeErrEngine{err: errors.New("boom")}}
	engineCacheMu.Unlock()

	input := []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hello"}}}}
	before, err := BeforeTurnHook(context.Background(), conf, user, http.Header{}, input, 120000)
	require.Error(t, err)
	require.False(t, before.ColdStartFallback)
}
