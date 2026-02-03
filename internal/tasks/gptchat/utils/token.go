// Package utils provides some utility functions for gptchat
package utils

import (
	"fmt"

	galgo "github.com/Laisky/go-utils/v6/algorithm"
	tiktoken "github.com/pkoukk/tiktoken-go"
)

const defaultTokenEncoding = "gpt-3.5-turbo" //nolint:gosec //G101: Potential hardcoded credentials

// CalculateTokens calculate tokens for given model and content
func CalculateTokens(model, content string) int {
	if model == "" {
		model = defaultTokenEncoding
	}

	tokener, err := tiktoken.EncodingForModel(model)
	if err != nil {
		tokener, err = tiktoken.EncodingForModel(defaultTokenEncoding)
		if err != nil {
			panic(fmt.Sprintf("failed to get tokener for model `%s`", model))
		}
	}

	return len(tokener.EncodeOrdinary(content))
}

// TrimByTokens trim content by tokens
//
// find the index to split the content by bi-search
func TrimByTokens(model, content string, nTokens int) string {
	if nTokens == 0 || content == "" {
		return ""
	}

	if contentNTokens := CalculateTokens(model, content); contentNTokens <= nTokens {
		return content
	}

	rcontent := []rune(content)
	idx := galgo.BinarySearch([]rune(content), func(idx int, _ rune) int {
		tokens := CalculateTokens("", string(rcontent[:idx]))

		rem := nTokens - tokens
		if rem >= 0 && rem < 1 {
			return 0
		}

		return rem
	})

	if idx == -1 {
		return ""
	}

	return string(rcontent[:idx])
}
