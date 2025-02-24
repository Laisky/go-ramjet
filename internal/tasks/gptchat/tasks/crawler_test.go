package tasks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_dynamicFetchWorker(t *testing.T) {
	ctx := context.Background()
	url := "https://blog.laisky.com/pages/0/"

	content, err := dynamicFetchWorker(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, content)

	t.Log(string(content))
	t.Error()
}
