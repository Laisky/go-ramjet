package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

func TestSubAgent_NameAndSchema(t *testing.T) {
	t.Parallel()
	tt := NewSubAgentTool(0)
	require.Equal(t, SubAgentToolName, tt.Name())
	require.NotEmpty(t, tt.Description())

	var schema struct {
		Type       string          `json:"type"`
		Required   []string        `json:"required"`
		Properties json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(tt.Schema(), &schema))
	require.Equal(t, "object", schema.Type)
	require.ElementsMatch(t, []string{"profile", "task"}, schema.Required)
	props := string(schema.Properties)
	require.Contains(t, props, "allow_tools")
	require.Contains(t, props, "output_mode")
}

func TestSubAgent_NewDefaultsMaxDepthToTwo(t *testing.T) {
	t.Parallel()
	got := NewSubAgentTool(0).(*SubAgentTool)
	require.Equal(t, 2, got.MaxDepth)

	got = NewSubAgentTool(-1).(*SubAgentTool)
	require.Equal(t, 2, got.MaxDepth)

	got = NewSubAgentTool(5).(*SubAgentTool)
	require.Equal(t, 5, got.MaxDepth)
}

// U20 — spawn_agent reservation. The Phase 1 stub always returns an
// IsError result so the loop's error budget increments by one, signalling
// to the model that this capability is not yet wired up.
func TestSubAgent_U20_ExecuteReturnsPhase1Error(t *testing.T) {
	t.Parallel()
	tt := NewSubAgentTool(0)
	res, err := tt.Execute(context.Background(), tool.Call{
		CallID: "call_1",
		Name:   SubAgentToolName,
		Args:   json.RawMessage(`{"profile":"researcher","task":"summarize"}`),
	}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Equal(t, SubAgentToolPhase1Error, res.Content)
}

// SubAgentArgs is the locked-in argument shape and must remain JSON-stable
// across phases. Smoke-test the JSON tags so a future refactor cannot
// silently break Phase 2 callers that already know how to compose args.
func TestSubAgent_ArgsJSONTagsAreStable(t *testing.T) {
	t.Parallel()
	args := SubAgentArgs{
		Profile:    "researcher",
		Task:       "find latest blog",
		AllowTools: []string{"web_search", "web_fetch"},
		OutputMode: "inline",
	}
	data, err := json.Marshal(args)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"profile":"researcher","task":"find latest blog","allow_tools":["web_search","web_fetch"],"output_mode":"inline"}`,
		string(data),
	)

	var rt SubAgentArgs
	require.NoError(t, json.Unmarshal(data, &rt))
	require.Equal(t, args, rt)
}
