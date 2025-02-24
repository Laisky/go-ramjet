package http

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	gptTasks "github.com/Laisky/go-ramjet/internal/tasks/gptchat/tasks"
)

func TestFetchDynamicURLContent(t *testing.T) {
	gconfig.S.Set("redis.addr", "100.122.41.16:6379")
	gptTasks.RunDynamicWebCrawler()

	ctx := context.Background()
	url := "https://blog.laisky.com/pages/0/"

	content, err := gptTasks.FetchDynamicURLContent(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, content)

	t.Log(string(content))
}

func TestGoogleSearchBasic(t *testing.T) {
	gconfig.S.Set("redis.addr", "100.122.41.16:6379")
	gptTasks.RunDynamicWebCrawler()

	ctx := context.Background()
	url := "https://www.google.com/search?q=site%3Amedium.com+applied+mpc"

	content, err := gptTasks.FetchDynamicURLContent(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, content)

	fmt.Println(string(content))

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
	query := "how about the weather in shanghai"

	content, err := googleSearch(ctx, query, &config.UserConfig{
		IsFree: false,
	})
	require.NoError(t, err)
	require.NotNil(t, content)

	t.Logf("result:\n%s", string(content))
}

func Test_extractHtmlText(t *testing.T) {
	raw := []byte(`
		<html>
			<head>
				<title>Test HTML</title>
			</head>
			<body>
				<h1>Hello, World!</h1>
				<p>This is a test HTML document.</p>
				<script>
					console.log("This is a script tag");
				</script>
			</body>
		</html>
	`)

	expected := "Hello, World!\nThis is a test HTML document."

	result, err := _extrachHtmlText(raw)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}
