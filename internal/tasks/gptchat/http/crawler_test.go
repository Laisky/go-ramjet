package http

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchDynamicURLContent(t *testing.T) {
	ctx := context.Background()
	url := "https://medium.com/applied-mpc/a-crash-course-on-mpc-part-3-c3f302153929"

	content, err := fetchDynamicURLContent(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, content)
}
