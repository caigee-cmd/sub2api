//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openAIVideoTestAccount() *Account {
	return &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":             "sk-upstream",
		"base_url":            "https://yingman.example/v1",
		"openai_capabilities": []string{"videos"},
		"model_mapping":       map[string]any{"video": "upstream-video"},
	}}
}

func TestOpenAIVideosForwardingPreservesBodyAndMapsModel(t *testing.T) {
	body := []byte(`{"model":"video","prompt":"一只猫在草地上奔跑","seconds":"8","size":"1280x720","input_images":["https://example.com/a.png"],"face_processing":"model"}`)
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"video_123","status":"queued"}`)}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c, w := grokMediaContentTestContext(http.MethodPost, "/v1/videos", nil)

	result, err := svc.ForwardOpenAIVideos(context.Background(), c, openAIVideoTestAccount(), OpenAIVideosEndpointCreate, "", body, "application/json")

	require.NoError(t, err)
	require.JSONEq(t, `{"id":"video_123","status":"queued"}`, w.Body.String())
	require.Equal(t, "openai-video:video_123", result.ResponseID)
	require.Equal(t, "video", result.BillingModel)
	require.Equal(t, "upstream-video", result.UpstreamModel)
	require.Equal(t, "720p", result.VideoResolution)
	require.Equal(t, 8, result.VideoDurationSeconds)
	require.Zero(t, result.VideoCount, "create must not bill")
	require.Equal(t, "https://yingman.example/v1/videos", upstream.request.URL.String())
	require.Equal(t, "Bearer sk-upstream", upstream.request.Header.Get("Authorization"))
	forwarded, err := io.ReadAll(upstream.request.Body)
	require.NoError(t, err)
	require.Equal(t, "upstream-video", gjson.GetBytes(forwarded, "model").String())
	for _, field := range []string{"prompt", "seconds", "size", "input_images", "face_processing"} {
		require.Equal(t, gjson.GetBytes(body, field).Raw, gjson.GetBytes(forwarded, field).Raw, field)
	}
}

func TestOpenAIVideosStatusBillsOnlyCompleted(t *testing.T) {
	for _, status := range []string{"queued", "in_progress", "failed", "completed"} {
		t.Run(status, func(t *testing.T) {
			body := `{"id":"video_123","object":"video","status":"` + status + `","model":"upstream-video","seconds":"10","size":"720x1280"}`
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(body)}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/video_123", nil)

			result, err := svc.ForwardOpenAIVideos(context.Background(), c, openAIVideoTestAccount(), OpenAIVideosEndpointStatus, "openai-video:video_123", nil, "")

			require.NoError(t, err)
			require.JSONEq(t, body, w.Body.String())
			require.Equal(t, "/v1/videos/video_123", upstream.request.URL.Path)
			require.Equal(t, "openai-video:video_123", result.ResponseID)
			if status == "completed" {
				require.Equal(t, 1, result.VideoCount)
				require.Equal(t, 10, result.VideoDurationSeconds)
				require.Equal(t, "720p", result.VideoResolution)
			} else {
				require.Zero(t, result.VideoCount)
				require.Zero(t, result.VideoDurationSeconds)
			}
		})
	}
}

func TestOpenAIVideosContentStreamsUpstream(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":        []string{"video/mp4"},
			"Content-Disposition": []string{`attachment; filename="video_123.mp4"`},
		},
		Body: io.NopCloser(bytes.NewReader([]byte("video-bytes"))),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/video_123/content", map[string]string{"Range": "bytes=0-4"})

	result, err := svc.ForwardOpenAIVideos(context.Background(), c, openAIVideoTestAccount(), OpenAIVideosEndpointContent, "openai-video:video_123", nil, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Zero(t, result.VideoCount, "download must not bill on its own")
	require.Equal(t, "https://yingman.example/v1/videos/video_123/content", upstream.request.URL.String())
	require.Equal(t, "bytes=0-4", upstream.request.Header.Get("Range"))
	require.Equal(t, "video-bytes", w.Body.String())
	require.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
}

func TestOpenAIVideosMultipartKeepsReferenceFiles(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "video"))
	require.NoError(t, writer.WriteField("prompt", "跟着镜头跑"))
	require.NoError(t, writer.WriteField("seconds", "8"))
	require.NoError(t, writer.WriteField("size", "16:9"))
	file, err := writer.CreateFormFile("input_reference", "first.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("png-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"video_456","status":"queued"}`)}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c, _ := grokMediaContentTestContext(http.MethodPost, "/v1/videos", nil)

	result, err := svc.ForwardOpenAIVideos(context.Background(), c, openAIVideoTestAccount(), OpenAIVideosEndpointCreate, "", body.Bytes(), writer.FormDataContentType())

	require.NoError(t, err)
	require.Equal(t, "openai-video:video_456", result.ResponseID)
	require.Empty(t, result.VideoResolution, "aspect ratio is not a resolution tier")
	require.Equal(t, 8, result.VideoDurationSeconds)
	forwarded, err := io.ReadAll(upstream.request.Body)
	require.NoError(t, err)
	require.Contains(t, string(forwarded), "upstream-video")
	require.Contains(t, string(forwarded), "png-bytes")
	require.Contains(t, string(forwarded), "跟着镜头跑")
	require.NotContains(t, string(forwarded), `name="model"`+"\r\n\r\nvideo")
}

func TestOpenAIVideosSizeTierAndCapability(t *testing.T) {
	require.Equal(t, "480p", openAIVideoSizeTier("480x854"))
	require.Equal(t, "720p", openAIVideoSizeTier("1280x720"))
	require.Equal(t, "1080p", openAIVideoSizeTier("1080x1920"))
	require.Equal(t, "720p", openAIVideoSizeTier("720p"))
	require.Empty(t, openAIVideoSizeTier("16:9"))
	require.Empty(t, openAIVideoSizeTier("4k"))

	for _, id := range []string{"", "..", "a/b", "a?b"} {
		_, err := buildOpenAIVideosURL("https://yingman.example/v1", OpenAIVideosEndpointStatus, id)
		require.Error(t, err, id)
	}
	for _, tc := range []struct{ base, want string }{
		{"https://yingman.example", "https://yingman.example/v1/videos"},
		{"https://yingman.example/v1", "https://yingman.example/v1/videos"},
	} {
		got, err := buildOpenAIVideosURL(tc.base, OpenAIVideosEndpointCreate, "")
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}

	account := openAIVideoTestAccount()
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilitySeedance))
	account.Type = AccountTypeOAuth
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos))
	account.Type = AccountTypeAPIKey
	delete(account.Credentials, "openai_capabilities")
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos))
}

func stringsTrimRightVideos(base string) string {
	if len(base) > 3 && base[len(base)-3:] == "/v1" {
		return base + "/videos"
	}
	return base + "/v1/videos"
}

func TestOpenAIVideosContentErrorStreamsWithoutSizeCap(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"video/mp4"}},
		Body:       io.NopCloser(bytes.NewReader([]byte("partial-video"))),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/video_123/content", nil)

	_, err := svc.ForwardOpenAIVideos(context.Background(), c, openAIVideoTestAccount(), OpenAIVideosEndpointContent, "openai-video:video_123", nil, "")

	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Equal(t, "partial-video", w.Body.String())
	require.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
}

func TestOpenAIVideosUpstreamErrorIsNotRetried(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"error":{"message":"余额不足","code":"insufficient_quota"}}`)}
	upstream.response.StatusCode = 429
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c, w := grokMediaContentTestContext(http.MethodPost, "/v1/videos", nil)

	_, err := svc.ForwardOpenAIVideos(context.Background(), c, openAIVideoTestAccount(), OpenAIVideosEndpointCreate, "", []byte(`{"model":"video","prompt":"waves","seconds":"8"}`), "application/json")

	require.Error(t, err)
	require.Equal(t, 429, w.Code)
	require.Contains(t, w.Body.String(), "insufficient_quota")
	require.Len(t, upstream.requests, 1)
}
