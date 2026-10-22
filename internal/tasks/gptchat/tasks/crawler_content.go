package tasks

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/Laisky/zap"
)

// crawlerProviderIsHTML recognizes a leading HTML document or standard element,
// ignoring comments/whitespace while leaving Markdown prose and autolinks intact.
func crawlerProviderIsHTML(content string) bool {
	tokens := html.NewTokenizer(strings.NewReader(content))
	for {
		switch tokens.Next() {
		case html.CommentToken:
		case html.DoctypeToken:
			return true
		case html.TextToken:
			if strings.TrimSpace(string(tokens.Text())) != "" {
				return false
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			return tokens.Token().DataAtom != 0
		default:
			return false
		}
	}
}

// crawlerHTMLHasContent checks whether the body contains text or meaningful
// media/form content; comments and empty structural markup are not a result.
func crawlerHTMLHasContent(content []byte) bool {
	document, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return false
	}
	body := findHTMLBody(document)
	return body != nil && crawlerNodeHasContent(body)
}

// crawlerNodeHasContent walks a body subtree without treating scripts, styles
// or comments as content, while retaining image-only and interactive HTML.
func crawlerNodeHasContent(node *html.Node) bool {
	if node.Type == html.TextNode {
		return strings.TrimSpace(node.Data) != ""
	}
	if node.Type == html.ElementNode {
		switch node.DataAtom {
		case atom.Script, atom.Style, atom.Template:
			return false
		case atom.Img, atom.Svg, atom.Canvas, atom.Video, atom.Audio, atom.Iframe, atom.Object, atom.Embed,
			atom.Input, atom.Button, atom.Select, atom.Textarea:
			return true
		default:
			// Structural elements are useful only if a child has content.
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if crawlerNodeHasContent(child) {
			return true
		}
	}
	return false
}

// crawlerProviderFailureFields retains every attempted provider's finite outcome
// at the terminal log boundary even when typed direct errors dominate its text.
func crawlerProviderFailureFields(err error) []zap.Field {
	var attempts []string
	var visit func(error)
	visit = func(cause error) {
		if cause == nil {
			return
		}
		switch failure := cause.(type) {
		case *crawlerProviderAttemptError:
			attempts = append(attempts, failure.provider+":"+failure.code)
		case interface{ Unwrap() []error }:
			for _, child := range failure.Unwrap() {
				visit(child)
			}
		case interface{ Unwrap() error }:
			visit(failure.Unwrap())
		}
	}
	visit(err)
	if len(attempts) == 0 {
		return nil
	}
	return []zap.Field{zap.Strings("provider_attempts", attempts)}
}
