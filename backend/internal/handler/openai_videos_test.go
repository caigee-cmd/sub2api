//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVideosHandlerBillsCompletedOnceAndKeepsOwner(t *testing.T) {
	h, slots, bindings, upstream := newGrokMediaSlotHandler(t, false, false, service.PlatformOpenAI)
	upstream.call = func(req *http.Request, id int64) (*http.Response, error) {
		body := `{"id":"video_123","status":"queued"}`
		if req.Method != http.MethodPost {
			body = `{"id":"video_123","status":"completed","model":"upstream-video","seconds":"8","size":"1280x720"}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	newContext := func(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
		c, w := grokMediaSlotContext(context.Background(), method == http.MethodPost)
		key, _ := middleware.GetAPIKeyFromContext(c)
		key.Group.Platform = service.PlatformOpenAI
		body := ""
		if method == http.MethodPost {
			body = `{"model":"video","prompt":"waves","seconds":"8","size":"1280x720"}`
		}
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "request_id", Value: "video_123"}}
		return c, w
	}

	c, w := newContext(http.MethodPost, "/v1/videos")
	h.OpenAIVideos(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Len(t, bindings.pending, 1)
	require.Empty(t, bindings.billed)
	slots.assertReleased(t)

	c, w = newContext(http.MethodGet, "/v1/videos/video_123")
	h.OpenAIVideos(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Len(t, bindings.billed, 1)
	slots.assertReleased(t)

	c, w = newContext(http.MethodGet, "/v1/videos/video_123")
	h.OpenAIVideos(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Len(t, bindings.billed, 1, "repeated polls must not bill again")

	c, w = newContext(http.MethodGet, "/v1/videos/video_123")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 11, Concurrency: 5})
	before := upstream.calls
	h.OpenAIVideos(c)
	require.Equal(t, 404, w.Code, w.Body.String())
	require.Equal(t, before, upstream.calls, "another user must not reach the upstream")
	slots.assertReleased(t)
}
