package http

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFrontendReqMessageContent_UnmarshalJSON(t *testing.T) {
	t.Run("string content", func(t *testing.T) {
		var c FrontendReqMessageContent
		err := json.Unmarshal([]byte(`"hello world"`), &c)
		require.NoError(t, err)
		require.Equal(t, "hello world", c.StringContent)
		require.Empty(t, c.ArrayContent)
		require.Equal(t, "hello world", c.String())
	})

	t.Run("array content", func(t *testing.T) {
		var c FrontendReqMessageContent
		err := json.Unmarshal([]byte(`[
			{"type": "text", "text": "hello"},
			{"type": "image_url", "image_url": {"url": "data:image/png;base64,xxx"}}
		]`), &c)
		require.NoError(t, err)
		require.Empty(t, c.StringContent)
		require.Len(t, c.ArrayContent, 2)
		require.Equal(t, OpenaiVisionMessageContentTypeText, c.ArrayContent[0].Type)
		require.Equal(t, "hello", c.ArrayContent[0].Text)
		require.Equal(t, OpenaiVisionMessageContentTypeImageUrl, c.ArrayContent[1].Type)
		require.Equal(t, "data:image/png;base64,xxx", c.ArrayContent[1].ImageUrl.URL)
		require.Equal(t, "hello", c.String())
	})
}

func TestFrontendReqMessageContent_MarshalJSON(t *testing.T) {
	t.Run("string content", func(t *testing.T) {
		c := FrontendReqMessageContent{StringContent: "hello"}
		data, err := json.Marshal(c)
		require.NoError(t, err)
		require.Equal(t, `"hello"`, string(data))
	})

	t.Run("array content", func(t *testing.T) {
		c := FrontendReqMessageContent{
			ArrayContent: []OpenaiVisionMessageContent{
				{Type: OpenaiVisionMessageContentTypeText, Text: "hello"},
			},
		}
		data, err := json.Marshal(c)
		require.NoError(t, err)
		require.Equal(t, `[{"type":"text","text":"hello"}]`, string(data))
	})
}

func TestFrontendReqMessageContent_Append(t *testing.T) {
	t.Run("string content", func(t *testing.T) {
		c := FrontendReqMessageContent{StringContent: "hello"}
		c.Append(" world")
		require.Equal(t, "hello world", c.StringContent)
	})

	t.Run("array content", func(t *testing.T) {
		c := FrontendReqMessageContent{
			ArrayContent: []OpenaiVisionMessageContent{
				{Type: OpenaiVisionMessageContentTypeText, Text: "hello"},
			},
		}
		c.Append(" world")
		require.Len(t, c.ArrayContent, 2)
		require.Equal(t, " world", c.ArrayContent[1].Text)
		require.Equal(t, "hello world", c.String())
	})
}
