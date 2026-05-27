package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

func TestSendToUser_Schema(t *testing.T) {
	t.Parallel()
	tt := NewSendToUserTool()
	require.Equal(t, SendToUserName, tt.Name())
	require.NotEmpty(t, tt.Description())

	// Schema must be valid JSON and declare final_answer required.
	var schema struct {
		Type       string          `json:"type"`
		Properties json.RawMessage `json:"properties"`
		Required   []string        `json:"required"`
	}
	require.NoError(t, json.Unmarshal(tt.Schema(), &schema))
	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Required, "final_answer")
	require.Contains(t, string(schema.Properties), "citations")
}

func TestSendToUser_Happy(t *testing.T) {
	t.Parallel()
	tt := NewSendToUserTool()
	args := json.RawMessage(`{"final_answer":"hello world","citations":[{"url":"https://example.com","title":"ex"}]}`)
	res, err := tt.Execute(context.Background(), tool.Call{
		CallID: "c1",
		Name:   SendToUserName,
		Args:   args,
	}, nil)
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Equal(t, "hello world", res.Content)

	// Details round-trips through SendToUserArgs.
	var got SendToUserArgs
	require.NoError(t, json.Unmarshal(res.Details, &got))
	require.Equal(t, "hello world", got.FinalAnswer)
	require.Len(t, got.Citations, 1)
	require.Equal(t, "https://example.com", got.Citations[0].URL)
	require.Equal(t, "ex", got.Citations[0].Title)
}

// U9 — send_to_user schema validation. Malformed args (wrong type) must
// return Result{IsError: true} with a descriptive message; never a Go
// error (the loop treats Go errors differently from IsError).
func TestSendToUser_U9_WrongTypeIsErrorResult(t *testing.T) {
	t.Parallel()
	tt := NewSendToUserTool()
	res, err := tt.Execute(context.Background(), tool.Call{
		Args: json.RawMessage(`{"final_answer":42}`),
	}, nil)
	require.NoError(t, err, "Execute must surface schema errors via Result.IsError, not Go errors")
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "send_to_user:")
}

func TestSendToUser_U9_MissingFinalAnswerIsErrorResult(t *testing.T) {
	t.Parallel()
	tt := NewSendToUserTool()
	res, err := tt.Execute(context.Background(), tool.Call{
		Args: json.RawMessage(`{}`),
	}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "final_answer")
}

func TestSendToUser_U9_EmptyFinalAnswerIsErrorResult(t *testing.T) {
	t.Parallel()
	tt := NewSendToUserTool()
	res, err := tt.Execute(context.Background(), tool.Call{
		Args: json.RawMessage(`{"final_answer":""}`),
	}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "final_answer")
}

func TestSendToUser_U9_MalformedJSONIsErrorResult(t *testing.T) {
	t.Parallel()
	tt := NewSendToUserTool()
	res, err := tt.Execute(context.Background(), tool.Call{
		Args: json.RawMessage(`{not json`),
	}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "send_to_user:")
}

func TestSendToUser_U9_NilArgsIsErrorResult(t *testing.T) {
	t.Parallel()
	tt := NewSendToUserTool()
	res, err := tt.Execute(context.Background(), tool.Call{
		Args: nil,
	}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "missing")
}

func TestSendToUser_U9_CitationMissingURLIsErrorResult(t *testing.T) {
	t.Parallel()
	tt := NewSendToUserTool()
	res, err := tt.Execute(context.Background(), tool.Call{
		Args: json.RawMessage(`{"final_answer":"ok","citations":[{"title":"x"}]}`),
	}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "citations[0].url")
}
