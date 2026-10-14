package memoryx

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/agents/files"
	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// contractCaller records real adapter calls and returns explicit synthetic MCP replies.
type contractCaller struct {
	response string
	err      error
	name     string
	args     map[string]any
}

// CallTool records the tool and arguments, then decodes the configured response or returns its error.
func (c *contractCaller) CallTool(_ context.Context, name string, args any, out any) error {
	c.name = name
	data, err := json.Marshal(args)
	if err != nil {
		return errors.Wrap(err, "encode fixture arguments")
	}
	c.args = map[string]any{}
	if err := json.Unmarshal(data, &c.args); err != nil {
		return errors.Wrap(err, "decode fixture arguments")
	}
	if c.err != nil {
		return errors.WithStack(c.err)
	}
	if out != nil {
		if err := json.Unmarshal([]byte(c.response), out); err != nil {
			return errors.Wrap(err, "decode fixture response")
		}
	}
	return nil
}

// TestMemoryStorageSearchContract checks empty, populated, and typed-error responses
// through the same buildStorageEngine adapter used by request memory.
func TestMemoryStorageSearchContract(t *testing.T) {
	ctx := context.Background()
	conf := &config.OpenAI{}
	user := &config.UserConfig{}
	caller := &contractCaller{response: `{"chunks":[]}`}
	engine, err := buildStorageEngine(ctx, conf, user, caller)
	require.NoError(t, err)
	chunks, err := engine.Search(ctx, "gptchat", "synthetic query", "/memory/session", 5)
	require.NoError(t, err)
	require.NotNil(t, chunks)
	require.Empty(t, chunks)
	require.Equal(t, "file_search", caller.name)
	require.Equal(t, map[string]any{"project": "gptchat", "query": "synthetic query", "path_prefix": "/memory/session", "limit": float64(5)}, caller.args)

	caller.response = `{"chunks":[{"file_path":"/memory/session/a.txt","file_seek_start_bytes":3,"file_seek_end_bytes":7,"chunk_content":"fact","score":0.9}]}`
	chunks, err = engine.Search(ctx, "gptchat", "synthetic query", "/memory/session", 5)
	require.NoError(t, err)
	require.Equal(t, []memorystorage.FileChunk{{FilePath: "/memory/session/a.txt", StartBytes: 3, EndBytes: 7, Content: "fact", Score: 0.9}}, chunks)

	caller.err = &files.ToolError{Code: files.ErrorCodeNotFound, Message: "synthetic missing path"}
	chunks, err = engine.Search(ctx, "gptchat", "synthetic query", "/memory/session", 5)
	require.Error(t, err)
	require.Nil(t, chunks)
	require.True(t, isMemoryColdStartNotFound(err))
	caller.err = &files.ToolError{Code: files.ErrorCodePayloadTooLarge, Message: "synthetic size limit"}
	_, err = engine.Search(ctx, "gptchat", "synthetic query", "/memory/session", 5)
	require.Error(t, err)
	require.False(t, isMemoryColdStartNotFound(err))
	require.True(t, isMemoryPayloadTooLarge(err))
}

// TestMemoryStorageFileContract checks exact append/range arguments, empty files,
// missing metadata, and validation failure without a backend call.
func TestMemoryStorageFileContract(t *testing.T) {
	ctx := context.Background()
	caller := &contractCaller{response: `{"content":""}`}
	engine, err := buildStorageEngine(ctx, &config.OpenAI{}, &config.UserConfig{}, caller)
	require.NoError(t, err)
	require.NoError(t, engine.Write(ctx, "gptchat", "/memory/session/a.txt", "fact", memorystorage.WriteModeAppend, 0))
	require.Equal(t, "file_write", caller.name)
	require.Equal(t, "APPEND", caller.args["mode"])
	require.Equal(t, "utf-8", caller.args["content_encoding"])
	require.Equal(t, map[string]any{"project": "gptchat", "path": "/memory/session/a.txt", "content": "fact", "content_encoding": "utf-8", "mode": "APPEND", "offset": float64(0)}, caller.args)
	content, err := engine.Read(ctx, "gptchat", "/memory/session/a.txt", 3, 7)
	require.NoError(t, err)
	require.Empty(t, content)
	require.Equal(t, float64(3), caller.args["offset"])
	require.Equal(t, float64(7), caller.args["length"])
	require.Equal(t, map[string]any{"project": "gptchat", "path": "/memory/session/a.txt", "offset": float64(3), "length": float64(7)}, caller.args)
	caller.response = `{"content":"populated synthetic content"}`
	content, err = engine.Read(ctx, "gptchat", "/memory/session/a.txt", 3, 7)
	require.NoError(t, err)
	require.Equal(t, "populated synthetic content", content)
	caller.response = `{"exists":false}`
	info, err := engine.Stat(ctx, "gptchat", "/memory/session/missing.txt")
	require.NoError(t, err)
	require.False(t, info.Exists)
	caller.name = ""
	_, err = engine.Read(ctx, "gptchat", "/memory/../escape", 0, -1)
	require.Error(t, err)
	require.Empty(t, caller.name)
}
