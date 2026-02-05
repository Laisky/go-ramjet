package openai

import (
	"context"
	"fmt"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v6"
)

var rewriteToMarkdownPrompt = gutils.Dedent(`
	<task>You are a professional content editor. Please help me rewrite the
	following content into a well-structured markdown article. Ensure that the
	content is organized with appropriate headings, subheadings, bullet points,
	and numbered lists where necessary. The rewritten article should be clear,
	concise, and engaging for readers.</task>

	<article>%s</article>`)

// HTMLBodyToMarkdown converts an HTML body (or any raw textual content) into Markdown.
//
// Args:
//   - apiBase: OpenAI-compatible base URL.
//   - apiKey: Bearer token.
//   - htmlBody: Raw HTML body bytes.
//
// Returns:
//   - markdown: The rewritten Markdown.
func HTMLBodyToMarkdown(ctx context.Context, apiBase, apiKey string, htmlBody []byte) (markdown string, err error) {
	if len(htmlBody) == 0 {
		return "", errors.New("htmlBody is empty")
	}

	prompt := fmt.Sprintf(rewriteToMarkdownPrompt, string(htmlBody))
	markdown, err = OneshotChat(ctx, apiBase, apiKey, "", "", prompt)
	if err != nil {
		return "", errors.Wrap(err, "oneshot chat")
	}

	return markdown, nil
}
