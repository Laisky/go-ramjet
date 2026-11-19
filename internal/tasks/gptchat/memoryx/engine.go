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
	"github.com/Laisky/go-ramjet/library/egress"
)

var (
	engineCacheMu sync.RWMutex
	engineCache   = map[string]memory.Engine{}
	// memoryLLMResolver resolves hostnames when admitting a caller-selected
	// memory model base. Nil uses the system resolver; tests replace it.
	memoryLLMResolver egress.Resolver
)

// admitMemoryLLMBase applies the shared public-destination policy to user's
// model base when the caller selected it through X-Laisky-Api-Base and memory
// extraction will call it. The memory library builds its own client, which
// cannot be pinned, so the base is admitted on every engine lookup instead;
// the library's https-only rule and certificate verification stop a later DNS
// answer from reaching an internal service under the admitted name.
//
// Parameters:
//   - ctx: Request context bounding the DNS lookup.
//   - conf: Global gptchat config; extraction runs only when MemoryModel is set.
//   - user: Current user.
//
// Returns a wrapped *egress.AdmissionError when the base is refused.
func admitMemoryLLMBase(ctx context.Context, conf *config.OpenAI, user *config.UserConfig) error {
	if user == nil || !user.APIBaseUserSelected || strings.TrimSpace(conf.MemoryModel) == "" {
		return nil
	}
	if _, err := egress.Admit(ctx, strings.TrimSpace(user.APIBase), memoryLLMResolver); err != nil {
		return errors.Wrap(err, "admit memory model base")
	}
	return nil
}

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
	if err := admitMemoryLLMBase(ctx, conf, user); err != nil {
		return nil, err
	}
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
