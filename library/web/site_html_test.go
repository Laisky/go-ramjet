package web

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var benchmarkSiteHTMLResult string

// TestUpsertMetaTagMatchesLegacy verifies the optimized selector lookup preserves exact output semantics.
func TestUpsertMetaTagMatchesLegacy(t *testing.T) {
	testCases := []struct {
		name      string
		htmlDoc   string
		attrKey   string
		attrValue string
		content   string
	}{
		{
			name:      "replace existing content",
			htmlDoc:   `<html><head><meta name="ramjet-site" content="old"></head></html>`,
			attrKey:   "name",
			attrValue: "ramjet-site",
			content:   `new & improved`,
		},
		{
			name:      "append missing content attribute",
			htmlDoc:   `<html><head><meta property="og:title"></head></html>`,
			attrKey:   "property",
			attrValue: "og:title",
			content:   `Title`,
		},
		{
			name:      "insert missing tag",
			htmlDoc:   `<html><head></head></html>`,
			attrKey:   "name",
			attrValue: "description",
			content:   `Description`,
		},
		{
			name:      "match tag case insensitively",
			htmlDoc:   `<HTML><HEAD><META NAME="THEME-COLOR" CONTENT="#000"></HEAD></HTML>`,
			attrKey:   "name",
			attrValue: "theme-color",
			content:   `#fff`,
		},
		{
			name:      "preserve dynamic selector behavior",
			htmlDoc:   `<html><head><meta name="custom[1]" content="old"></head></html>`,
			attrKey:   "name",
			attrValue: "custom[1]",
			content:   `new`,
		},
		{
			name:      "ignore blank content",
			htmlDoc:   `<html><head><meta name="ramjet-theme" content="old"></head></html>`,
			attrKey:   "name",
			attrValue: "ramjet-theme",
			content:   `   `,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			expected := legacyUpsertMetaTag(
				testCase.htmlDoc,
				testCase.attrKey,
				testCase.attrValue,
				testCase.content,
			)
			actual := upsertMetaTag(
				testCase.htmlDoc,
				testCase.attrKey,
				testCase.attrValue,
				testCase.content,
			)
			require.Equal(t, expected, actual)
		})
	}
}

// TestApplySiteMetadataToHTMLMatchesLegacy verifies the complete rendered document remains byte-for-byte identical.
func TestApplySiteMetadataToHTMLMatchesLegacy(t *testing.T) {
	htmlDoc := `<!doctype html><html><head><title>Default</title><link rel="icon" href="/favicon.ico"><meta name="ramjet-site" content="default"><meta name="ramjet-theme"><meta property="og:title" content="Default"></head><body><div id="root"></div></body></html>`
	meta := SiteMetadata{
		ID:               "chat",
		Theme:            "midnight",
		Title:            "Ramjet & Chat",
		Favicon:          "/chat?a=1&b=2",
		Description:      "Fast & safe",
		OGImage:          "/chat.png",
		ThemeColor:       "#101010",
		HeadHTML:         `<link rel="canonical" href="https://chat.example.com/">`,
		RootFallbackHTML: `<main>Loading...</main>`,
	}
	expected := `<!doctype html><html><head><title>Ramjet &amp; Chat</title><link rel="icon" href="/chat?a=1&amp;b=2"><meta name="ramjet-site" content="chat"><meta name="ramjet-theme" content="midnight"><meta property="og:title" content="Ramjet &amp; Chat"><meta name="description" content="Fast &amp; safe"><meta name="theme-color" content="#101010"><meta property="og:description" content="Fast &amp; safe"><meta property="og:image" content="/chat.png"><link rel="canonical" href="https://chat.example.com/"></head><body><div id="root"><main>Loading...</main></div></body></html>`

	require.Equal(t, expected, legacyApplySiteMetadataToHTML(htmlDoc, meta))
	require.Equal(t, expected, applySiteMetadataToHTML(htmlDoc, meta))
}

// BenchmarkApplySiteMetadataToHTML compares the exact legacy and optimized request paths under one toolchain and fixture.
func BenchmarkApplySiteMetadataToHTML(b *testing.B) {
	htmlDoc, meta := benchmarkSiteHTMLFixture()

	b.Run("legacy", func(b *testing.B) {
		benchmarkSiteHTMLTransform(b, htmlDoc, meta, legacyApplySiteMetadataToHTML)
	})
	b.Run("optimized", func(b *testing.B) {
		benchmarkSiteHTMLTransform(b, htmlDoc, meta, applySiteMetadataToHTML)
	})
}

// benchmarkSiteHTMLFixture returns the shared document and metadata used by both benchmark variants.
func benchmarkSiteHTMLFixture() (string, SiteMetadata) {
	htmlDoc := `<!doctype html><html><head>` +
		`<title>Default</title>` +
		`<link rel="icon" href="/favicon.ico">` +
		`<meta name="ramjet-site" content="default">` +
		`<meta name="ramjet-theme" content="default">` +
		`<meta name="description" content="default description">` +
		`<meta name="theme-color" content="#000000">` +
		`<meta property="og:title" content="Default">` +
		`<meta property="og:description" content="default description">` +
		`<meta property="og:image" content="/default.png">` +
		strings.Repeat(`<script type="module" src="/assets/index.js"></script>`, 64) +
		`</head><body><div id="root"></div></body></html>`
	meta := SiteMetadata{
		ID:               "chat",
		Theme:            "midnight",
		Title:            "Ramjet Chat",
		Favicon:          "/chat.ico",
		Description:      "A performance-sensitive chat application",
		OGTitle:          "Ramjet Chat",
		OGDescription:    "A performance-sensitive chat application",
		OGImage:          "/chat.png",
		ThemeColor:       "#101010",
		HeadHTML:         `<link rel="canonical" href="https://chat.example.com/">`,
		RootFallbackHTML: `<main>Loading chat...</main>`,
	}

	return htmlDoc, meta
}

// benchmarkSiteHTMLTransform runs one metadata transformation implementation and reports allocations and throughput.
func benchmarkSiteHTMLTransform(
	b *testing.B,
	htmlDoc string,
	meta SiteMetadata,
	transform func(string, SiteMetadata) string,
) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(htmlDoc)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSiteHTMLResult = transform(htmlDoc, meta)
	}
}

// legacyApplySiteMetadataToHTML reproduces the complete pre-optimization request path for comparison.
func legacyApplySiteMetadataToHTML(htmlDoc string, meta SiteMetadata) string {
	htmlDoc = replaceOrInsertTitle(htmlDoc, meta.Title)
	htmlDoc = replaceOrInsertFavicon(htmlDoc, meta.Favicon)

	htmlDoc = legacyUpsertMetaTag(htmlDoc, "name", "ramjet-site", meta.ID)
	htmlDoc = legacyUpsertMetaTag(htmlDoc, "name", "ramjet-theme", meta.Theme)

	htmlDoc = legacyUpsertMetaTag(htmlDoc, "name", "description", meta.Description)
	htmlDoc = legacyUpsertMetaTag(htmlDoc, "name", "theme-color", meta.ThemeColor)

	ogTitle := meta.OGTitle
	if ogTitle == "" {
		ogTitle = meta.Title
	}
	ogDescription := meta.OGDescription
	if ogDescription == "" {
		ogDescription = meta.Description
	}

	htmlDoc = legacyUpsertMetaTag(htmlDoc, "property", "og:title", ogTitle)
	htmlDoc = legacyUpsertMetaTag(htmlDoc, "property", "og:description", ogDescription)
	htmlDoc = legacyUpsertMetaTag(htmlDoc, "property", "og:image", meta.OGImage)
	htmlDoc = insertHeadHTML(htmlDoc, meta.HeadHTML)
	htmlDoc = replaceRootFallback(htmlDoc, meta.RootFallbackHTML)

	return htmlDoc
}

// legacyUpsertMetaTag reproduces the pre-optimization implementation for behavioral and benchmark comparison.
func legacyUpsertMetaTag(htmlDoc, attrKey, attrValue, content string) string {
	if strings.TrimSpace(content) == "" {
		return htmlDoc
	}

	escapedContent := html.EscapeString(content)
	escapedAttr := html.EscapeString(attrValue)
	pattern := fmt.Sprintf(`(?i)<meta[^>]*\b%[1]s="%[2]s"[^>]*>`, attrKey, regexp.QuoteMeta(attrValue))
	re := regexp.MustCompile(pattern)

	if re.MatchString(htmlDoc) {
		return re.ReplaceAllStringFunc(htmlDoc, func(s string) string {
			if reMetaContent.MatchString(s) {
				return reMetaContent.ReplaceAllString(s, `content="`+escapedContent+`"`)
			}
			return strings.TrimSuffix(s, ">") + ` content="` + escapedContent + `">`
		})
	}

	snippet := fmt.Sprintf(`<meta %s="%s" content="%s">`, attrKey, escapedAttr, escapedContent)
	return insertIntoHead(htmlDoc, snippet)
}
