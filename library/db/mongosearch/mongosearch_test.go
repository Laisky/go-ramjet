package mongosearch

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// TestValidateQuery verifies ValidateQuery rejects exactly the queries MongoDB
// or the driver refuse as a pattern, plus those over the caller's bound.
func TestValidateQuery(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, query, wantErr string
	}{
		{name: "empty", query: ""},
		{name: "plain", query: "Tokyo"},
		{name: "regex syntax is fine", query: `(a+)+$|.*`},
		{name: "multi-byte at the bound", query: strings.Repeat("東", 3)},
		{name: "over the bound", query: strings.Repeat("a", 10), wantErr: "at most 9 bytes"},
		{name: "invalid utf-8", query: "a\xffb", wantErr: "valid UTF-8"},
		{name: "nul byte", query: "a\x00b", wantErr: "NUL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateQuery(tc.query, 9)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// TestLiteralSubstringIsTypedLiteralRegex verifies the filter value is a typed
// BSON regular expression (never an operator document) whose pattern matches
// the query literally and case-insensitively.
func TestLiteralSubstringIsTypedLiteralRegex(t *testing.T) {
	t.Parallel()

	for _, query := range []string{`{"$ne":null}`, `$where`, ".*", "(a+)+$", `\Q.*\E`, "(?x)a #b", "Tokyo"} {
		raw, err := bson.Marshal(bson.D{{Key: "word", Value: LiteralSubstring(query)}})
		require.NoError(t, err)

		pattern, opts, ok := bson.Raw(raw).Lookup("word").RegexOK()
		require.True(t, ok, "%q must encode as a BSON regular expression", query)
		require.Equal(t, regexp.QuoteMeta(query), pattern)
		require.Equal(t, "i", opts)

		// RE2 as a stand-in for PCRE on these ASCII patterns: only the
		// literal text matches.
		re := regexp.MustCompile("(?" + opts + ")" + pattern)
		require.True(t, re.MatchString("x"+strings.ToUpper(query)+"y"), query)
		if query != "Tokyo" {
			require.False(t, re.MatchString("tokyo aaaa where ne null"), query)
		}
	}
}
