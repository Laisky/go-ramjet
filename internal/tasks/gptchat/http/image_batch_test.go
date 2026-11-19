package http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// editTestRequest points gctx at the image edit endpoint with a one-pixel PNG
// source image and a paid user with external image billing.
func editTestRequest(t *testing.T, gctx *gin.Context) {
	t.Helper()
	var src bytes.Buffer
	require.NoError(t, png.Encode(&src, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	body, err := json.Marshal(map[string]string{
		"prompt": "make it blue",
		"model":  "gpt-image-1",
		"image":  base64.StdEncoding.EncodeToString(src.Bytes()),
	})
	require.NoError(t, err)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/edits", bytes.NewReader(body))
	gctx.Request.Header.Set("Content-Type", "application/json")
	gctx.Set(ctxKeyUser, billedImageUser(false))
}

// TestEditImageHandlerFetchesOnlyOneImage verifies that the image edit endpoint,
// which charges one image, delivers one image and never downloads the surplus
// entries an upstream adds to its answer.
func TestEditImageHandlerFetchesOnlyOneImage(t *testing.T) {
	const returned = 40
	var downloads atomic.Int32
	f := &imageBillingFixture{}
	gctx := f.install(t)
	editTestRequest(t, gctx)
	f.override = func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == "/v1/images/edits":
			data := make([]map[string]string, returned)
			for i := range data {
				data[i] = map[string]string{"url": "http://" + r.Host + "/edited/" + strconv.Itoa(i) + ".png"}
			}
			f.writeJSON(w, map[string]any{"data": data})
			return true
		case strings.HasPrefix(r.URL.Path, "/edited/"):
			downloads.Add(1)
			f.writeBody(w, []byte("synthetic-image"))
			return true
		}
		return false
	}

	EditImageHandler(gctx)
	require.Equal(t, http.StatusOK, gctx.Writer.Status())
	require.Equal(t, 1, f.delivered())
	require.Equal(t, int64(GetImageModelPrice("gpt-image-1")), f.totalCharged())
	require.LessOrEqual(t, downloads.Load(), int32(1), "surplus edited images were downloaded")
}

// TestEditImageHandlerFailsWithoutImage verifies that an edit answer whose entries
// carry no image data is reported as a failure instead of an empty success.
func TestEditImageHandlerFailsWithoutImage(t *testing.T) {
	f := &imageBillingFixture{empty: 2}
	gctx := f.install(t)
	editTestRequest(t, gctx)

	EditImageHandler(gctx)
	require.GreaterOrEqual(t, gctx.Writer.Status(), http.StatusBadRequest)
	require.Zero(t, f.delivered())
}
