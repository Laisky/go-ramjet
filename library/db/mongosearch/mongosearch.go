// Package mongosearch builds MongoDB filters that search for user-supplied
// text as a literal, case-insensitive substring.
//
// Search handlers pass untrusted query text to MongoDB's $regex matching. Two
// things keep that safe: the text is escaped with regexp.QuoteMeta, so pattern
// syntax (wildcards, anchors, alternation, catastrophic backtracking) only
// matches itself, and the value is a typed BSON regular expression, so it can
// never be read as a query operator document. ValidateQuery rejects the inputs
// MongoDB or the driver refuse as a pattern, so handlers can answer them with a
// client error instead of a database failure logged at the alerting level.
package mongosearch

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ValidateQuery checks that query can be searched with LiteralSubstring.
//
// It rejects queries longer than maxBytes, invalid UTF-8 (MongoDB 8.0 answers
// Location51091) and NUL bytes (the driver refuses to encode them in a BSON
// regular expression). maxBytes should stay far below MongoDB's ~32 KiB
// pattern limit; QuoteMeta can double the length of a query.
//
// Parameters:
//   - query: untrusted search text.
//   - maxBytes: largest accepted query length in bytes.
//
// Returns:
//   - error: why query cannot be searched, or nil when it is acceptable.
func ValidateQuery(query string, maxBytes int) error {
	switch {
	case len(query) > maxBytes:
		return errors.Errorf("query must be at most %d bytes", maxBytes)
	case !utf8.ValidString(query):
		return errors.New("query must be valid UTF-8")
	case strings.ContainsRune(query, 0):
		return errors.New("query must not contain NUL")
	default:
		return nil
	}
}

// LiteralSubstring returns a case-insensitive BSON regular expression that
// matches documents whose field contains query literally.
//
// Use it as a filter value, e.g. bson.D{{Key: "title", Value:
// LiteralSubstring(q)}}. Go's regexp.QuoteMeta escapes only with `\` followed
// by punctuation, which MongoDB's PCRE engine also reads as literal
// characters, and no option that changes that reading (such as `x`) is set.
//
// Parameters:
//   - query: untrusted search text; check it with ValidateQuery first.
//
// Returns:
//   - primitive.Regex: pattern regexp.QuoteMeta(query) with option "i".
func LiteralSubstring(query string) primitive.Regex {
	return primitive.Regex{Pattern: regexp.QuoteMeta(query), Options: "i"}
}
