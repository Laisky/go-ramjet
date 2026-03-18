package crawler

import (
	"strings"
	"testing"
)

func TestExtractSearchContext_XSS(t *testing.T) {
	t.Parallel()

	dao := &Dao{} // DB not needed for extractSearchContext

	tests := []struct {
		name    string
		pattern string
		text    string
		// The context output must NOT contain unescaped user-controlled HTML
		mustNotContain []string
		mustContain    []string
	}{
		{
			name:    "normal text has mark tags",
			pattern: "hello",
			text:    "prefix hello suffix",
			mustContain: []string{
				"<mark>hello</mark>",
			},
		},
		{
			name:    "XSS in surrounding text is escaped",
			pattern: "hello",
			text:    `<script>alert(1)</script> hello <img onerror=alert(1)>`,
			mustNotContain: []string{
				"<script>",
				"<img ",
			},
			mustContain: []string{
				"<mark>hello</mark>",
				"&lt;",
			},
		},
		{
			name:    "angle brackets in text are always escaped",
			pattern: "test",
			text:    `<b>test</b>`,
			mustNotContain: []string{
				"<b>",
				"</b>",
			},
			mustContain: []string{
				"<mark>test</mark>",
				"&lt;b&gt;",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			results := dao.extractSearchContext(tt.pattern, []SearchResult{
				{Text: tt.text},
			})

			if len(results) == 0 {
				t.Fatal("expected at least one result")
			}

			ctx := results[0].Context
			for _, s := range tt.mustNotContain {
				if strings.Contains(ctx, s) {
					t.Errorf("context should not contain %q, got %q", s, ctx)
				}
			}
			for _, s := range tt.mustContain {
				if !strings.Contains(ctx, s) {
					t.Errorf("context should contain %q, got %q", s, ctx)
				}
			}
		})
	}
}
