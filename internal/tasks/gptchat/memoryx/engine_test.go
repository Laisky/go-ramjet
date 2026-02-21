package memoryx

import (
	"context"
	stdjson "encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-utils/v6/agents/files"
	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

type mockToolCaller struct {
	mu    sync.Mutex
	files map[string]string
}

func (m *mockToolCaller) CallTool(_ context.Context, toolName string, args any, out any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.files == nil {
		m.files = map[string]string{}
	}

	argm := map[string]any{}
	data, _ := stdjson.Marshal(args)
	_ = stdjson.Unmarshal(data, &argm)
	project, _ := argm["project"].(string)
	path, _ := argm["path"].(string)
	key := project + ":" + path

	switch toolName {
	case "file_write":
		content, _ := argm["content"].(string)
		mode, _ := argm["mode"].(string)
		switch mode {
		case "APPEND":
			m.files[key] += content
		default:
			m.files[key] = content
		}
		return nil
	case "file_read":
		if out == nil {
			return nil
		}
		resp := map[string]any{"content": m.files[key]}
		buf, _ := stdjson.Marshal(resp)
		return stdjson.Unmarshal(buf, out)
	case "file_stat":
		if out == nil {
			return nil
		}
		_, exists := m.files[key]
		resp := map[string]any{"exists": exists, "type": "file", "size": len(m.files[key]), "updated_at": ""}
		buf, _ := stdjson.Marshal(resp)
		return stdjson.Unmarshal(buf, out)
	case "file_list":
		if out == nil {
			return nil
		}
		resp := map[string]any{"entries": []any{}, "has_more": false}
		buf, _ := stdjson.Marshal(resp)
		return stdjson.Unmarshal(buf, out)
	case "file_search":
		if out == nil {
			return nil
		}
		resp := map[string]any{"chunks": []any{}}
		buf, _ := stdjson.Marshal(resp)
		return stdjson.Unmarshal(buf, out)
	case "file_delete":
		delete(m.files, key)
		return nil
	default:
		return &files.ToolError{Code: files.ErrorCodeNotFound, Message: "unsupported"}
	}
}

func TestBuildStorageEngineMCPWithMockCaller(t *testing.T) {
	conf := &config.OpenAI{EnableMemory: true, MemoryStorageMCPURL: "https://mcp.example.com", MemoryProject: "gptchat"}
	user := &config.UserConfig{OpenaiToken: "sk-user"}
	caller := &mockToolCaller{}
	storageEngine, err := buildStorageEngine(context.Background(), conf, user, caller)
	require.NoError(t, err)
	require.NotNil(t, storageEngine)

	err = storageEngine.Write(context.Background(), "gptchat", "/memory/s1/test.txt", "hello", memorystorage.WriteModeAppend, 0)
	require.NoError(t, err)
	content, err := storageEngine.Read(context.Background(), "gptchat", "/memory/s1/test.txt", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "hello", content)
}
