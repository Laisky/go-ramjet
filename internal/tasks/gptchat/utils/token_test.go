package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateTokens(t *testing.T) {
	model := "gpt-3.5-turbo"
	content := "Hello, world!"

	tokens := CalculateTokens(model, content)
	assert.Equal(t, 4, tokens)
}

func TestTrimByTokens(t *testing.T) {
	model := "gpt-3.5-turbo"
	content := "Hello, world! How are you? I am fine, thank you."

	t.Run("found", func(t *testing.T) {
		nTokens := 2
		trimmedContent := TrimByTokens(model, content, nTokens)
		assert.Equal(t, "Hello,", trimmedContent)
	})

	t.Run("not found", func(t *testing.T) {
		nTokens := 100
		trimmedContent := TrimByTokens(model, content, nTokens)
		assert.Equal(t, content, trimmedContent)
	})
}
