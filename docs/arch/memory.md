# Technical Design: Transparent Memory Integration in GPTChat (Current)

## 1. Objective

Integrate `go-utils/agents/memory` into `go-ramjet` chat flow so memory works automatically for users:

1. Persist user facts and context after each turn.
2. Recall and inject relevant memory before each turn.
3. Keep user experience transparent (no manual memory setup required).

## 2. Final Architecture

Use in-process memory hooks in chat flow:

1. Before upstream model call: `memory.Engine.BeforeTurn`.
2. After final assistant output: `memory.Engine.AfterTurn`.

This is deterministic and independent of model tool-calling decisions.

## 3. Storage Strategy (MCP-only)

Current implementation supports **MCP storage only**.

1. Backend: `go-utils/agents/memory/storage/mcp`.
2. Endpoint: `openai.memory_storage_mcp_url`.
3. MCP auth for memory storage: always use current user OpenAI token (`user.OpenaiToken`).
4. No local storage fallback.

## 4. Runtime Identity Mapping

Runtime identifiers are derived as follows:

1. `project`: `openai.memory_project` (default `gptchat`).
2. `user_id`: `user.UserName`.
3. `session_id`: stable hash from user API key (`user.OpenaiToken`, fallback `user.Token`).
4. `turn_id`: server-generated UUID per request.

This means memory is isolated by API key, not by frontend chat session id.

## 5. End-to-End Lifecycle

### 5.1 Before upstream call

In `convert2UpstreamResponsesRequest`:

1. Parse frontend request and resolve authenticated user.
2. Convert current Responses input to `[]memory.ResponseItem`.
3. Build `BeforeTurn` `CurrentInput` from only the latest `user` message (exclude `system` and earlier turns).
4. Run `BeforeTurn`.
5. Replace upstream `responsesReq.Input` with converted `BeforeTurnOutput.InputItems`.

Effect: recalled memory (including SDK-generated `developer` blocks) is injected before inference.

### 5.2 After final output

In `sendChatWithResponsesToolLoop` after final text is produced:

1. Build `AfterTurnInput` with actual input items used in loop and assistant output text.
2. Run `AfterTurn`.
3. Do not fail user response if memory persistence fails.

## 6. Data Conversion Contract

### 6.1 Responses -> memory.ResponseItem

1. Message text content maps to `input_text` parts.
2. Image content maps to `input_image` parts.
3. Tool outputs map to `function_call_output` items.

### 6.2 memory.ResponseItem -> Responses

1. Message role/content are preserved.
2. `developer` role memory blocks are preserved in upstream input.

## 7. Config Surface (Current)

`internal/tasks/gptchat/config/config.go` currently exposes:

1. `EnableMemory`
2. `MemoryProject`
3. `MemoryStorageMCPURL`
4. `MemoryModel`
5. `MemoryLLMTimeoutSeconds`
6. `MemoryLLMMaxOutputTokens`

Validation rules:

1. `MemoryProject` cannot be empty.
2. If memory is enabled, `MemoryStorageMCPURL` is required.
3. `MemoryLLMTimeoutSeconds > 0`.
4. `MemoryLLMMaxOutputTokens > 0`.

## 8. Failure Handling Policy

1. `BeforeTurn` failure: log warning, continue without memory injection.
2. `AfterTurn` failure: log warning, do not fail chat response.
3. Memory errors are observable via structured logging fields, without leaking secrets.

## 8.1 Runtime Switch and Prompt Safety

1. Memory is globally gated by `openai.enable_memory` and can be toggled per request via `laisky_extra.chat_switch.enable_memory`.
2. Per-request memory switch defaults to enabled when omitted.
3. During `BeforeTurn`, original `system` messages are preserved and cannot be overwritten by recalled memory content.
4. Memory-recalled context can still be injected as additional non-system items (for example `developer` role blocks).

## 9. Metrics (Current)

Memory metrics are exported via `expvar`:

1. `memory_before_turn_total`
2. `memory_before_turn_fail_total`
3. `memory_after_turn_total`
4. `memory_after_turn_fail_total`
5. `memory_recall_fact_count`
6. `memory_before_latency_ms`
7. `memory_after_latency_ms`

## 10. Key Code Locations

1. `internal/tasks/gptchat/memoryx/engine.go`: engine cache and MCP storage construction.
2. `internal/tasks/gptchat/memoryx/keys.go`: runtime key derivation and API-key-based session id.
3. `internal/tasks/gptchat/memoryx/mapping.go`: responses/memory item conversions.
4. `internal/tasks/gptchat/memoryx/hooks.go`: before/after turn orchestration.
5. `internal/tasks/gptchat/http/responses_chat_handler.go`: chat pipeline integration points.
