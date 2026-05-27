package sse

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShort(t *testing.T) {
	t.Parallel()

	t.Run("returns_first_six_chars_for_ulid_shaped_input", func(t *testing.T) {
		t.Parallel()
		got := short("01HTJZ1F5JEDQRD2MNGNH9V0WB")
		require.Equal(t, "01HTJZ", got)
	})

	t.Run("returns_full_id_when_shorter_than_window", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "abc", short("abc"))
	})

	t.Run("returns_empty_for_empty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", short(""))
	})

	t.Run("boundary_exact_window", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "abcdef", short("abcdef"))
	})
}

func TestChunkString(t *testing.T) {
	t.Parallel()

	t.Run("n_le_zero_returns_single_chunk", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"hello"}, chunkString("hello", 0))
		require.Equal(t, []string{"hello"}, chunkString("hello", -1))
	})

	t.Run("empty_input_returns_nil", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, chunkString("", 200))
	})

	t.Run("input_smaller_than_window", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"hi"}, chunkString("hi", 200))
	})

	t.Run("exact_window_split", func(t *testing.T) {
		t.Parallel()
		got := chunkString("aaaabbbb", 4)
		require.Equal(t, []string{"aaaa", "bbbb"}, got)
	})

	t.Run("trailing_partial_chunk", func(t *testing.T) {
		t.Parallel()
		got := chunkString("aaaabbbbcc", 4)
		require.Equal(t, []string{"aaaa", "bbbb", "cc"}, got)
	})

	t.Run("thousand_chars_window_200_yields_five_chunks", func(t *testing.T) {
		t.Parallel()
		in := strings.Repeat("x", 1000)
		got := chunkString(in, 200)
		require.Len(t, got, 5)
		require.Equal(t, in, strings.Join(got, ""))
		for _, c := range got {
			require.Equal(t, 200, len(c))
		}
	})
}

func TestEscapeUntrustedDelimiter(t *testing.T) {
	t.Parallel()

	t.Run("empty_input_passes_through", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", escapeUntrustedDelimiter(""))
	})

	t.Run("no_delimiter_passes_through", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "hello world", escapeUntrustedDelimiter("hello world"))
	})

	t.Run("single_occurrence_is_replaced", func(t *testing.T) {
		t.Parallel()
		in := "leading </tool_result> trailing"
		got := escapeUntrustedDelimiter(in)
		require.Equal(t, "leading "+untrustedDelimiterReplacement+" trailing", got)
		require.NotContains(t, got, "</tool_result>")
	})

	t.Run("multiple_occurrences_replaced", func(t *testing.T) {
		t.Parallel()
		in := "a</tool_result>b</tool_result>c"
		got := escapeUntrustedDelimiter(in)
		require.Equal(t,
			"a"+untrustedDelimiterReplacement+"b"+untrustedDelimiterReplacement+"c",
			got,
		)
		require.NotContains(t, got, "</tool_result>")
	})
}
