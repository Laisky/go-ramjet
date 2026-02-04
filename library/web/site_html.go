package web

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	reTitle       = regexp.MustCompile(`(?i)<title>.*?</title>`)
	reFavicon     = regexp.MustCompile(`(?i)<link[^>]*?rel="icon"[^>]*?>`)
	reHref        = regexp.MustCompile(`(?i)href="[^"]*"`)
	reHeadClose   = regexp.MustCompile(`(?i)</head>`)
	reMetaContent = regexp.MustCompile(`(?i)content="[^"]*"`)
)

// applySiteMetadataToHTML injects meta into htmlDoc and returns the updated HTML string.
func applySiteMetadataToHTML(htmlDoc string, meta SiteMetadata) string {
	htmlDoc = replaceOrInsertTitle(htmlDoc, meta.Title)
	htmlDoc = replaceOrInsertFavicon(htmlDoc, meta.Favicon)

	htmlDoc = upsertMetaTag(htmlDoc, "name", "ramjet-site", meta.ID)
	htmlDoc = upsertMetaTag(htmlDoc, "name", "ramjet-theme", meta.Theme)

	htmlDoc = upsertMetaTag(htmlDoc, "name", "description", meta.Description)
	htmlDoc = upsertMetaTag(htmlDoc, "name", "theme-color", meta.ThemeColor)

	ogTitle := meta.OGTitle
	if ogTitle == "" {
		ogTitle = meta.Title
	}
	ogDescription := meta.OGDescription
	if ogDescription == "" {
		ogDescription = meta.Description
	}

	htmlDoc = upsertMetaTag(htmlDoc, "property", "og:title", ogTitle)
	htmlDoc = upsertMetaTag(htmlDoc, "property", "og:description", ogDescription)
	htmlDoc = upsertMetaTag(htmlDoc, "property", "og:image", meta.OGImage)

	return htmlDoc
}

// replaceOrInsertTitle updates htmlDoc with the provided title and returns the updated HTML string.
func replaceOrInsertTitle(htmlDoc string, title string) string {
	if strings.TrimSpace(title) == "" {
		return htmlDoc
	}
	escaped := html.EscapeString(title)

	if reTitle.MatchString(htmlDoc) {
		return reTitle.ReplaceAllString(htmlDoc, "<title>"+escaped+"</title>")
	}

	return insertIntoHead(htmlDoc, "<title>"+escaped+"</title>")
}

// replaceOrInsertFavicon updates htmlDoc with the provided favicon and returns the updated HTML string.
func replaceOrInsertFavicon(htmlDoc string, favicon string) string {
	if strings.TrimSpace(favicon) == "" {
		return htmlDoc
	}
	escaped := html.EscapeString(favicon)

	if reFavicon.MatchString(htmlDoc) {
		return reFavicon.ReplaceAllStringFunc(htmlDoc, func(s string) string {
			return reHref.ReplaceAllString(s, `href="`+escaped+`"`)
		})
	}

	return insertIntoHead(htmlDoc, `<link rel="icon" href="`+escaped+`">`)
}

// upsertMetaTag updates or inserts a meta tag in htmlDoc for attrKey/attrValue with content and returns the updated HTML string.
func upsertMetaTag(htmlDoc, attrKey, attrValue, content string) string {
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

// insertIntoHead inserts snippet before the closing head tag in htmlDoc and returns the updated HTML string.
func insertIntoHead(htmlDoc, snippet string) string {
	if reHeadClose.MatchString(htmlDoc) {
		return reHeadClose.ReplaceAllString(htmlDoc, snippet+"</head>")
	}
	return snippet + htmlDoc
}
