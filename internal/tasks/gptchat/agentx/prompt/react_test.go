package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// TestRender_AllComponentsPresent verifies the four load-bearing
// components from proposal §4.4 appear in the rendered prompt.
func TestRender_AllComponentsPresent(t *testing.T) {
	r := NewReactRenderer(20)
	text := r.Render(0, 20)

	// 1. ReAct directive (think → call exactly one tool → observe).
	if !strings.Contains(text, "exactly one tool") {
		t.Errorf("missing ReAct directive: %q", text)
	}
	if !strings.Contains(text, "observe") {
		t.Errorf("missing observe step: %q", text)
	}

	// 2. Exit-tool contract: send_to_user with final_answer.
	if !strings.Contains(text, "send_to_user") {
		t.Errorf("missing send_to_user contract: %q", text)
	}
	if !strings.Contains(text, "final_answer") {
		t.Errorf("missing final_answer mention: %q", text)
	}

	// 3. Untrusted delimiter guard.
	if !strings.Contains(text, `<tool_result tool="..." trust="untrusted">`) {
		t.Errorf("missing untrusted-content guard: %q", text)
	}

	// 4. Budget hint: "N step(s) remaining".
	if !strings.Contains(text, "remaining") {
		t.Errorf("missing budget hint: %q", text)
	}

	// Version marker is always at the head so the hook can overwrite.
	if !strings.HasPrefix(text, ReactVersionMarker) {
		t.Errorf("rendered prompt must start with %q, got %q", ReactVersionMarker, text[:min(64, len(text))])
	}
}

// TestRender_BudgetHintReflectsRemaining verifies the rendered budget
// number changes across rounds.
func TestRender_BudgetHintReflectsRemaining(t *testing.T) {
	r := NewReactRenderer(20)
	r0 := r.Render(0, 20)
	r5 := r.Render(5, 15)
	if !strings.Contains(r0, "20 step(s) remaining") {
		t.Errorf("round 0 hint missing 20 step(s) remaining: %q", r0)
	}
	if !strings.Contains(r5, "15 step(s) remaining") {
		t.Errorf("round 5 hint missing 15 step(s) remaining: %q", r5)
	}
}

// TestAsContextHook_PrependsOnFirstRound verifies the hook prepends a
// system message ahead of any existing input.
func TestAsContextHook_PrependsOnFirstRound(t *testing.T) {
	r := NewReactRenderer(20)
	hookFn := r.AsContextHook()
	original := []model.InputItem{
		httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hello"},
	}
	out, err := hookFn(context.Background(), hook.ContextEvent{Input: original})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if len(out.Input) != 2 {
		t.Fatalf("want 2 items, got %d", len(out.Input))
	}
	sys, ok := out.Input[0].(httppkg.OpenAIResponsesInputMessage)
	if !ok {
		t.Fatalf("want OpenAIResponsesInputMessage at [0], got %T", out.Input[0])
	}
	if sys.Role != "system" {
		t.Errorf("want system role, got %q", sys.Role)
	}
	body, ok := sys.Content.(string)
	if !ok {
		t.Fatalf("want string content, got %T", sys.Content)
	}
	if !strings.HasPrefix(body, ReactVersionMarker) {
		t.Errorf("system prompt missing marker: %q", body)
	}
	// Existing user message unchanged at [1].
	usr, ok := out.Input[1].(httppkg.OpenAIResponsesInputMessage)
	if !ok || usr.Role != "user" {
		t.Errorf("existing user message disturbed: got %+v", out.Input[1])
	}
}

// TestAsContextHook_PreservesUnrelatedSystemMessages verifies that
// system messages *without* the marker (e.g. memory hooks) survive the
// injection unchanged. The rendered prompt is appended at the head, not
// merged into them.
func TestAsContextHook_PreservesUnrelatedSystemMessages(t *testing.T) {
	r := NewReactRenderer(20)
	hookFn := r.AsContextHook()
	originalSystem := httppkg.OpenAIResponsesInputMessage{Role: "system", Content: "memory: be helpful"}
	original := []model.InputItem{
		originalSystem,
		httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hello"},
	}
	out, err := hookFn(context.Background(), hook.ContextEvent{Input: original})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if len(out.Input) != 3 {
		t.Fatalf("want 3 items, got %d", len(out.Input))
	}
	// Existing system message is at index 1 (after the prepended ReAct
	// prompt), and is unchanged.
	if got := out.Input[1].(httppkg.OpenAIResponsesInputMessage); got != originalSystem {
		t.Errorf("memory system message mutated: want %+v, got %+v", originalSystem, got)
	}
}

// TestAsContextHook_IdempotentRefresh verifies a second invocation
// overwrites the prior ReAct-marker system message instead of stacking
// a new copy.
func TestAsContextHook_IdempotentRefresh(t *testing.T) {
	r := NewReactRenderer(20)
	hookFn := r.AsContextHook()
	original := []model.InputItem{
		httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hello"},
	}
	first, err := hookFn(context.Background(), hook.ContextEvent{Input: original})
	if err != nil {
		t.Fatalf("first hook err: %v", err)
	}
	if len(first.Input) != 2 {
		t.Fatalf("first round expected 2 items, got %d", len(first.Input))
	}

	// Round 2: the hook should overwrite, not append, and the budget
	// number should reflect the new round.
	second, err := hookFn(context.Background(), hook.ContextEvent{Input: first.Input})
	if err != nil {
		t.Fatalf("second hook err: %v", err)
	}
	if len(second.Input) != 2 {
		t.Errorf("second round expected 2 items (overwrite), got %d", len(second.Input))
	}

	sys, ok := second.Input[0].(httppkg.OpenAIResponsesInputMessage)
	if !ok || sys.Role != "system" {
		t.Fatalf("expected refreshed system message at [0], got %T", second.Input[0])
	}
	body := sys.Content.(string)
	if !strings.Contains(body, "round 2 of") {
		t.Errorf("budget hint not refreshed: %q", body)
	}
}

// TestAsContextHook_RecognisesMapShape verifies the hook treats the
// loop's map-shaped systemMessage helper output the same as a typed
// OpenAIResponsesInputMessage when detecting the marker.
func TestAsContextHook_RecognisesMapShape(t *testing.T) {
	r := NewReactRenderer(20)
	hookFn := r.AsContextHook()
	// Simulate a prior round having injected via the map shape (matches
	// the loop's systemMessage builder) carrying the marker.
	rendered := r.Render(0, 20)
	prior := map[string]any{
		"role":    "system",
		"content": rendered,
	}
	original := []model.InputItem{
		prior,
		httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hello"},
	}
	out, err := hookFn(context.Background(), hook.ContextEvent{Input: original})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if len(out.Input) != 2 {
		t.Fatalf("want 2 items after overwrite, got %d", len(out.Input))
	}
	// Overwrite swaps in a typed message at the same slot.
	sys, ok := out.Input[0].(httppkg.OpenAIResponsesInputMessage)
	if !ok {
		t.Fatalf("want overwrite to produce typed message, got %T", out.Input[0])
	}
	if sys.Role != "system" {
		t.Errorf("want system role after overwrite, got %q", sys.Role)
	}
}

