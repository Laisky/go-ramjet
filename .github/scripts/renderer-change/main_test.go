package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMeaningfulGoTokens separates comments and formatting from executable or directive changes.
func TestMeaningfulGoTokens(t *testing.T) {
	baseline := "package prompt\nfunc Render() string { return \"stable\" }\n"
	before, err := meaningfulGoTokens([]byte(baseline))
	require.NoError(t, err)
	cases := []struct {
		name, source string
		changed      bool
	}{
		{"documentation", "// Correct spelling.\n" + baseline, false},
		{"lint directive", "package prompt\nfunc Render() string { return \"stable\" } //nolint:misspell // Preserve prompt text.\n", false},
		{"formatting", "package prompt\n\nfunc Render() string {\n\treturn \"stable\"\n}\n", false},
		{"literal", "package prompt\nfunc Render() string { return \"changed\" }\n", true},
		{"logic", "package prompt\nfunc Render() string { return \"stable\" + \"suffix\" }\n", true},
		{"line directive", "//line renderer.go:100\n" + baseline, true},
		{"block line directive", "/*line renderer.go:100*/\n" + baseline, true},
		{"compiler directive", "package prompt\n//go:noinline\nfunc Render() string { return \"stable\" }\n", true},
		{"build constraint", "//go:build linux\n\n" + baseline, true},
		{"legacy build constraint", "// +build linux\n\n" + baseline, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			after, err := meaningfulGoTokens([]byte(test.source))
			require.NoError(t, err)
			require.Equal(t, test.changed, before != after)
		})
	}
}

// TestMeaningfulGoTokensRejectsInvalidSource ensures lexer failures cannot silently skip benchmarking.
func TestMeaningfulGoTokensRejectsInvalidSource(t *testing.T) {
	_, err := meaningfulGoTokens([]byte("package prompt\nvar value = \"unterminated"))
	require.Error(t, err)
}

// TestMeaningfulGoTokensPreservesStatementSeparators keeps return newlines and for separators meaningful.
func TestMeaningfulGoTokensPreservesStatementSeparators(t *testing.T) {
	cases := []struct {
		name, before, after string
	}{
		{
			"return newline",
			"package prompt\nfunc Render() string { return\nliteral() }\n",
			"package prompt\nfunc Render() string { return literal() }\n",
		},
		{
			"for separators",
			"package prompt\nfunc Render() { for ; ; {} }\n",
			"package prompt\nfunc Render() { for {} }\n",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before, err := meaningfulGoTokens([]byte(test.before))
			require.NoError(t, err)
			after, err := meaningfulGoTokens([]byte(test.after))
			require.NoError(t, err)
			require.NotEqual(t, before, after)
		})
	}
}
