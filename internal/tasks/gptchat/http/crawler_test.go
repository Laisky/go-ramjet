package http

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestFetchDynamicURLContent(t *testing.T) {
	ctx := context.Background()
	url := "https://medium.com/applied-mpc/a-crash-course-on-mpc-part-3-c3f302153929"

	content, err := fetchDynamicURLContent(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, content)
}

func TestGoogleSearchBasic(t *testing.T) {
	ctx := context.Background()
	url := "https://www.google.com/search?q=site%3Amedium.com+applied+mpc"

	content, err := fetchDynamicURLContent(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, content)

	// fmt.Println(string(content))

	doc, err := html.Parse(bytes.NewReader(content))
	require.NoError(t, err)

	// find div#search
	var searchDiv *html.Node
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, attr := range n.Attr {
				if attr.Key == "id" && attr.Val == "search" {
					searchDiv = n
					return
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	require.NotNil(t, searchDiv)

	// marshal searchDiv
	var buf bytes.Buffer
	require.NoError(t, html.Render(&buf, searchDiv))

	re := regexp.MustCompile(`href="([^"]*)"`)
	matches := re.FindAllStringSubmatch(buf.String(), -1)
	for _, match := range matches {
		fmt.Println(match[1])
	}
}

func TestGoogleSearch(t *testing.T) {
	SetupHTTPCli()

	ctx := context.Background()
	query := "how to install tpm on my motherboard"

	content, err := googleSearch(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, content)

	t.Logf("result:\n%s", string(content))
}
