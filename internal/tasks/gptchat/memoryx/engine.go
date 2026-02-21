package memoryx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/agents/files"
	"github.com/Laisky/go-utils/v6/agents/memory"
	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	memorystoragemcp "github.com/Laisky/go-utils/v6/agents/memory/storage/mcp"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

var (
	engineCacheMu sync.RWMutex
	engineCache   = map[string]memory.Engine{}
)

// GetEngine returns a cached memory engine for current runtime context.
//
// Parameters:
//   - ctx: Request context.
//   - conf: Global gptchat openai config.
//   - user: Current authenticated user config.
//
// Returns:
//   - memory.Engine: A cached or newly created memory engine.
//   - error: Non-nil when storage backend or engine initialization fails.
func GetEngine(ctx context.Context, conf *config.OpenAI, user *config.UserConfig) (memory.Engine, error) {
	return getEngine(ctx, conf, user, nil)
}

func getEngine(ctx context.Context, conf *config.OpenAI, user *config.UserConfig, caller files.ToolCaller) (memory.Engine, error) {
	cacheKey := buildEngineCacheKey(conf, user)

	engineCacheMu.RLock()
	cached, ok := engineCache[cacheKey]
	engineCacheMu.RUnlock()
	if ok {
		return cached, nil
	}

	storageEngine, err := buildStorageEngine(ctx, conf, user, caller)
	if err != nil {
		return nil, errors.Wrap(err, "build memory storage engine")
	}

	memoryConf := memory.Config{
		RecentContextItems:     30,
		RecallFactsLimit:       20,
		SearchLimit:            5,
		CompactThreshold:       0.8,
		L1RetentionDays:        1,
		L2RetentionDays:        7,
		CompactionMinAge:       24 * time.Hour,
		SummaryRefreshInterval: time.Hour,
		MaxProcessedTurns:      1024,
	}

	memoryModel := strings.TrimSpace(conf.MemoryModel)
	if memoryModel != "" {
		memoryConf.LLMModel = memoryModel
		memoryConf.LLMAPIBase = strings.TrimSpace(user.APIBase)
		memoryConf.LLMAPIKey = strings.TrimSpace(user.OpenaiToken)
		memoryConf.LLMTimeout = time.Duration(conf.MemoryLLMTimeoutSeconds) * time.Second
		memoryConf.LLMMaxOutputTokens = conf.MemoryLLMMaxOutputTokens
	}

	engine, err := memory.NewEngine(storageEngine, memoryConf)
	if err != nil {
		return nil, errors.Wrap(err, "new memory engine")
	}

	engineCacheMu.Lock()
	engineCache[cacheKey] = engine
	engineCacheMu.Unlock()

	return engine, nil
}

func buildStorageEngine(ctx context.Context, conf *config.OpenAI, user *config.UserConfig, caller files.ToolCaller) (memorystorage.Engine, error) {
	storageEngine, err := memorystoragemcp.NewEngine(ctx, memorystoragemcp.Config{
		Caller:   caller,
		Endpoint: strings.TrimSpace(conf.MemoryStorageMCPURL),
		APIKey:   strings.TrimSpace(user.OpenaiToken),
	})
	if err != nil {
		return nil, errors.Wrap(err, "new mcp memory storage")
	}

	return storageEngine, nil
}

func buildEngineCacheKey(conf *config.OpenAI, user *config.UserConfig) string {
	model := strings.TrimSpace(conf.MemoryModel)
	llmFingerprint := ""
	if model != "" {
		sum := sha256.Sum256([]byte(strings.TrimSpace(user.APIBase) + "|" + strings.TrimSpace(user.OpenaiToken)))
		llmFingerprint = hex.EncodeToString(sum[:])
	}

	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(conf.MemoryStorageMCPURL),
		strings.TrimSpace(conf.MemoryProject),
		model,
		llmFingerprint,
	}, "|")))

	return hex.EncodeToString(sum[:])
}
