package memoryx

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/agents/files"
	"github.com/Laisky/go-utils/v6/agents/memory"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// BeforeTurnResult stores prepared data from memory BeforeTurn hook.
type BeforeTurnResult struct {
	Enabled           bool
	ColdStartFallback bool
	Keys              RuntimeKeys
	InputItems        []memory.ResponseItem
	PreparedInput     []any
	RecallFactIDs     []string
	ContextTokenCount int
}

// BeforeTurnHook runs memory injection before upstream chat call.
//
// Parameters:
//   - ctx: Request context.
//   - conf: Global gptchat openai config.
//   - user: Current authenticated user.
//   - reqHeader: Request headers.
//   - responsesInput: Flattened Responses API input items.
//   - maxInputTok: Maximum input token budget for memory preparation.
//
// Returns:
//   - BeforeTurnResult: Prepared memory context and converted input payload.
//   - error: Non-nil when memory hook fails.
func BeforeTurnHook(
	ctx context.Context,
	conf *config.OpenAI,
	user *config.UserConfig,
	reqHeader http.Header,
	responsesInput []any,
	maxInputTok int,
) (BeforeTurnResult, error) {
	startedAt := time.Now().UTC()
	result := BeforeTurnResult{Enabled: isMemoryEnabled(conf)}
	if !result.Enabled {
		return result, nil
	}
	memoryBeforeTurnTotal.Add(1)

	result.Keys = BuildRuntimeKeys(conf, user, reqHeader)

	inputItems, err := ResponsesInputToMemoryItems(responsesInput)
	if err != nil {
		memoryBeforeTurnFail.Add(1)
		observeLatencyHistogram(memoryBeforeLatencyMs, time.Since(startedAt).Milliseconds())
		memoryBeforeLatencyCount.Add(1)
		return result, errors.Wrap(err, "responses input to memory items")
	}
	result.InputItems = inputItems

	engine, err := GetEngine(ctx, conf, user)
	if err != nil {
		memoryBeforeTurnFail.Add(1)
		observeLatencyHistogram(memoryBeforeLatencyMs, time.Since(startedAt).Milliseconds())
		memoryBeforeLatencyCount.Add(1)
		return result, errors.Wrap(err, "get memory engine")
	}

	prepared, err := engine.BeforeTurn(ctx, memory.BeforeTurnInput{
		Project:      result.Keys.Project,
		SessionID:    result.Keys.SessionID,
		UserID:       result.Keys.UserID,
		TurnID:       result.Keys.TurnID,
		CurrentInput: inputItems,
		MaxInputTok:  maxInputTok,
	})
	if err != nil {
		if isMemoryColdStartNotFound(err) {
			result.ColdStartFallback = true
			result.PreparedInput = MemoryItemsToResponsesInput(result.InputItems)
			observeLatencyHistogram(memoryBeforeLatencyMs, time.Since(startedAt).Milliseconds())
			memoryBeforeLatencyCount.Add(1)
			return result, nil
		}

		memoryBeforeTurnFail.Add(1)
		observeLatencyHistogram(memoryBeforeLatencyMs, time.Since(startedAt).Milliseconds())
		memoryBeforeLatencyCount.Add(1)
		return result, errors.Wrap(err, "memory before turn")
	}

	result.InputItems = prepared.InputItems
	result.PreparedInput = MemoryItemsToResponsesInput(prepared.InputItems)
	result.RecallFactIDs = append(result.RecallFactIDs, prepared.RecallFactIDs...)
	result.ContextTokenCount = prepared.ContextTokenCount
	memoryRecallFactCount.Add(int64(len(prepared.RecallFactIDs)))
	observeLatencyHistogram(memoryBeforeLatencyMs, time.Since(startedAt).Milliseconds())
	memoryBeforeLatencyCount.Add(1)

	return result, nil
}

// isMemoryColdStartNotFound returns true when memory before-turn failure comes from missing storage paths.
//
// Parameters:
//   - err: The error returned by memory engine before-turn flow.
//
// Returns:
//   - bool: True when the error unwraps to MCP FileIO NOT_FOUND.
func isMemoryColdStartNotFound(err error) bool {
	var toolErr *files.ToolError
	if !errors.As(err, &toolErr) {
		return false
	}

	return toolErr.Code == files.ErrorCodeNotFound
}

// AfterTurnHook persists memory after final assistant output is generated.
//
// Parameters:
//   - ctx: Request context.
//   - conf: Global gptchat openai config.
//   - user: Current authenticated user.
//   - keys: Runtime identifiers from BeforeTurn.
//   - inputItems: Final input items used in tool-loop turns.
//   - finalText: Final assistant output text.
//
// Returns:
//   - error: Non-nil when memory persistence fails.
func AfterTurnHook(
	ctx context.Context,
	conf *config.OpenAI,
	user *config.UserConfig,
	keys RuntimeKeys,
	inputItems []any,
	finalText string,
) error {
	startedAt := time.Now().UTC()
	if !isMemoryEnabled(conf) {
		return nil
	}
	memoryAfterTurnTotal.Add(1)

	if strings.TrimSpace(keys.Project) == "" || strings.TrimSpace(keys.SessionID) == "" ||
		strings.TrimSpace(keys.UserID) == "" || strings.TrimSpace(keys.TurnID) == "" {
		memoryAfterTurnFail.Add(1)
		observeLatencyHistogram(memoryAfterLatencyMs, time.Since(startedAt).Milliseconds())
		memoryAfterLatencyCount.Add(1)
		return errors.New("memory runtime keys are incomplete")
	}

	engine, err := GetEngine(ctx, conf, user)
	if err != nil {
		memoryAfterTurnFail.Add(1)
		observeLatencyHistogram(memoryAfterLatencyMs, time.Since(startedAt).Milliseconds())
		memoryAfterLatencyCount.Add(1)
		return errors.Wrap(err, "get memory engine")
	}

	memoryInputs, err := ResponsesInputToMemoryItems(inputItems)
	if err != nil {
		memoryAfterTurnFail.Add(1)
		observeLatencyHistogram(memoryAfterLatencyMs, time.Since(startedAt).Milliseconds())
		memoryAfterLatencyCount.Add(1)
		return errors.Wrap(err, "responses input to memory items")
	}

	if err = engine.AfterTurn(ctx, memory.AfterTurnInput{
		Project:     keys.Project,
		SessionID:   keys.SessionID,
		UserID:      keys.UserID,
		TurnID:      keys.TurnID,
		InputItems:  memoryInputs,
		OutputItems: BuildAssistantOutputItems(finalText),
	}); err != nil {
		memoryAfterTurnFail.Add(1)
		observeLatencyHistogram(memoryAfterLatencyMs, time.Since(startedAt).Milliseconds())
		memoryAfterLatencyCount.Add(1)
		return errors.Wrap(err, "memory after turn")
	}

	observeLatencyHistogram(memoryAfterLatencyMs, time.Since(startedAt).Milliseconds())
	memoryAfterLatencyCount.Add(1)

	return nil
}

func isMemoryEnabled(conf *config.OpenAI) bool {
	return conf != nil && conf.EnableMemory
}
