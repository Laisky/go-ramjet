package tasks

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// isCloudflareChallenge detects active challenge structure, excluding prose and ordinary Turnstile forms.
func isCloudflareChallenge(content string) bool {
	document, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return false
	}
	return hasCloudflareChallenge(document)
}

// hasCloudflareChallenge inspects DOM attributes and executable resources without matching quoted documentation.
func hasCloudflareChallenge(node *html.Node) bool {
	if node.Type == html.ElementNode {
		switch node.Data {
		case "pre", "code", "textarea":
			return false
		}
		for _, attribute := range node.Attr {
			if attribute.Key == "id" || attribute.Key == "class" {
				for _, value := range strings.Fields(attribute.Val) {
					if value == "cf-chl-widget" || value == "challenge-error-text" {
						return true
					}
				}
			}
			if node.Data == "script" && attribute.Key == "src" {
				source, err := url.Parse(attribute.Val)
				if err != nil {
					continue
				}
				if strings.HasPrefix(source.Path, "/cdn-cgi/challenge-platform/") {
					return true
				}
			}
		}
		if node.Data == "script" {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.TextNode && hasCloudflareAssignment(child.Data) {
					return true
				}
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if hasCloudflareChallenge(child) {
			return true
		}
	}
	return false
}

// hasCloudflareAssignment scans assignment tokens while excluding quoted strings and comments.
func hasCloudflareAssignment(script string) bool {
	for i := 0; i < len(script); {
		switch script[i] {
		case '\'', '"', '`':
			quote := script[i]
			i++
			for i < len(script) {
				if script[i] == '\\' {
					i += 2
					continue
				}
				if script[i] == quote {
					i++
					break
				}
				i++
			}
			continue
		case '/':
			if strings.HasPrefix(script[i:], "//") {
				next := strings.IndexByte(script[i:], '\n')
				if next < 0 {
					return false
				}
				i += next + 1
				continue
			}
			if strings.HasPrefix(script[i:], "/*") {
				next := strings.Index(script[i+2:], "*/")
				if next < 0 {
					return false
				}
				i += next + 4
				continue
			}
			if crawlerJavaScriptRegexStart(script, i) {
				i++
				inClass := false
				for i < len(script) {
					if script[i] == '\\' {
						i += 2
						continue
					}
					if script[i] == '[' {
						inClass = true
					}
					if script[i] == ']' {
						inClass = false
					}
					if script[i] == '/' && !inClass {
						i++
						break
					}
					i++
				}
				continue
			}
		}
		if !crawlerJavaScriptIdentifier(script[i], false) {
			i++
			continue
		}
		start := i
		for i < len(script) && crawlerJavaScriptIdentifier(script[i], true) {
			i++
		}
		if script[start:i] != "_cf_chl_opt" {
			continue
		}
		for i < len(script) && strings.ContainsRune(" \t\r\n", rune(script[i])) {
			i++
		}
		if i < len(script) && script[i] == '=' &&
			(i+1 >= len(script) || script[i+1] != '=' && script[i+1] != '>') {
			return true
		}
	}
	return false
}

// crawlerJavaScriptIdentifier recognizes ASCII identifier bytes used by the reviewed challenge marker.
func crawlerJavaScriptIdentifier(char byte, allowDigit bool) bool {
	return char == '_' || char == '$' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
		allowDigit && char >= '0' && char <= '9'
}

// crawlerJavaScriptRegexStart recognizes an expression-start slash without treating division as a literal.
func crawlerJavaScriptRegexStart(script string, position int) bool {
	for i := position - 1; i >= 0; i-- {
		if strings.ContainsRune(" \t\r\n", rune(script[i])) {
			continue
		}
		if strings.ContainsRune("=([{,:;!?&|+-*%^<>~", rune(script[i])) {
			return true
		}
		if crawlerJavaScriptIdentifier(script[i], true) {
			end := i + 1
			for i >= 0 && crawlerJavaScriptIdentifier(script[i], true) {
				i--
			}
			switch script[i+1 : end] {
			case "return", "throw", "yield", "void", "delete":
				return true
			}
		}
		return false
	}
	return true
}
