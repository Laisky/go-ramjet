package blog

import "regexp"

var (
	discardWordsRegex = regexp.MustCompile(`^(.|[\<\>\\[\]\{\}\(\)\.\"\'\+\-\*\/,（）：；，。！？=]*|[a-zA-Z0-9]{1,4})$`)
	discardFmtRegex   = regexp.MustCompile(`\<[^\>]*\>`)
	//nolint: gosmopolitan
	discardWords = []string{
		"一个", "所以", "如果", "可以", "这个", "那个",
	}
)
