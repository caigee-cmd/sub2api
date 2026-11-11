package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	// OpenAIEndpointCapabilityVideos 表示该 OpenAI API Key 账号承接
	// OpenAI 兼容的异步视频协议（POST /v1/videos）。与 Seedance、Grok 视频分开：
	// 只有显式声明了该能力的账号会被调度到，避免视频请求落到普通对话账号上。
	OpenAIEndpointCapabilityVideos OpenAIEndpointCapability = "videos"

	OpenAIVideosEndpointCreate  GrokMediaEndpoint = "openai_videos_create"
	OpenAIVideosEndpointStatus  GrokMediaEndpoint = "openai_videos_status"
	OpenAIVideosEndpointContent GrokMediaEndpoint = "openai_videos_content"

	openAIVideosUpstreamPath = "/v1/videos"
)

func (e GrokMediaEndpoint) IsOpenAIVideos() bool {
	return e == OpenAIVideosEndpointCreate || e == OpenAIVideosEndpointStatus || e == OpenAIVideosEndpointContent
}

// OpenAIVideoTaskKey 把任务归属与计费键同 Grok、Seedance 隔开。
// 同一个上游任务号出现在不同协议里时不会串号。
func OpenAIVideoTaskKey(id string) string { return "openai-video:" + strings.TrimSpace(id) }

// ParseOpenAIVideoRequest 只取调度、审核和计费快照需要的字段，其余原样转发。
// JSON 与 multipart 都认 model / prompt / seconds / size。
func ParseOpenAIVideoRequest(contentType string, body []byte) (GrokMediaRequestInfo, error) {
	var info GrokMediaRequestInfo
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		parseOpenAIVideoMultipart(&info, contentType, body)
	} else if gjson.ValidBytes(body) && gjson.ParseBytes(body).IsObject() {
		info.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
		info.Prompt = gjson.GetBytes(body, "prompt").String()
		info.DurationSeconds = openAIVideoSeconds(gjson.GetBytes(body, "seconds"))
		info.Resolution = openAIVideoSizeTier(gjson.GetBytes(body, "size").String())
	} else if len(bytes.TrimSpace(body)) > 0 {
		return info, fmt.Errorf("request body must be a JSON object")
	}
	if strings.TrimSpace(info.Model) == "" {
		return info, fmt.Errorf("model is required")
	}
	return info, nil
}

func parseOpenAIVideoMultipart(info *GrokMediaRequestInfo, contentType string, body []byte) {
	if info == nil {
		return
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err != nil {
			return
		}
		name := strings.TrimSpace(part.FormName())
		fileName := strings.TrimSpace(part.FileName())
		data, readErr := io.ReadAll(part)
		_ = part.Close()
		if readErr != nil || name == "" || fileName != "" {
			continue
		}
		switch name {
		case "model":
			info.Model = strings.TrimSpace(string(data))
		case "prompt":
			info.Prompt = string(data)
		case "seconds":
			info.DurationSeconds = openAIVideoSeconds(gjson.Parse(strconv.Quote(strings.TrimSpace(string(data)))))
		case "size":
			info.Resolution = openAIVideoSizeTier(string(data))
		}
	}
}

func openAIVideoSeconds(value gjson.Result) int {
	text := strings.TrimSpace(value.String())
	if text == "" {
		return 0
	}
	seconds, err := strconv.Atoi(text)
	if err != nil || seconds <= 0 {
		return 0
	}
	return seconds
}

// openAIVideoSizeTier 把上游的尺寸换成现有计费档位标签。
// 像素尺寸按短边换算（480 / 720 / 1080），比例尺寸无法换算，留给渠道按次价。
// 识别不了的值原样留空，不折算成低档。
func openAIVideoSizeTier(size string) string {
	size = strings.ToLower(strings.TrimSpace(size))
	if size == "" {
		return ""
	}
	if tier, ok := LookupVideoBillingResolution(size); ok {
		return tier
	}
	short := shortEdgePixels(size)
	switch {
	case short <= 0:
		return ""
	case short <= 480:
		return VideoBillingResolution480P
	case short <= 720:
		return VideoBillingResolution720P
	case short <= 1080:
		return VideoBillingResolution1080P
	default:
		return ""
	}
}

func shortEdgePixels(size string) int {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return 0
	}
	if width < height {
		return width
	}
	return height
}

func buildOpenAIVideosURL(base string, endpoint GrokMediaEndpoint, taskID string) (string, error) {
	target := buildOpenAIEndpointURL(base, openAIVideosUpstreamPath)
	if endpoint == OpenAIVideosEndpointCreate {
		return target, nil
	}
	id := strings.TrimPrefix(strings.TrimSpace(taskID), "openai-video:")
	if err := validateUpstreamPathSegment("video task ID", id); err != nil {
		return "", fmt.Errorf("invalid video task ID")
	}
	target += "/" + id
	if endpoint == OpenAIVideosEndpointContent {
		target += "/content"
	}
	return target, nil
}

// ForwardOpenAIVideos 把 OpenAI 兼容视频协议原样打到账号的 base_url。
// 只改 model（账号映射）和鉴权头；秒数、尺寸、参考图和其余字段保持原样。
// 创建时不产生计费单位。完成状态才回填秒数和分辨率，交给现有计费。
func (s *OpenAIGatewayService) ForwardOpenAIVideos(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint GrokMediaEndpoint,
	taskID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	if account == nil || !account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos) || !endpoint.IsOpenAIVideos() {
		return nil, fmt.Errorf("openai videos require an OpenAI API key account with the videos capability")
	}
	base, err := s.validateUpstreamBaseURL(account.GetCredential("base_url"))
	if err != nil {
		return nil, err
	}
	target, err := buildOpenAIVideosURL(base, endpoint, taskID)
	if err != nil {
		return nil, err
	}
	SetActualOpenAIUpstreamEndpoint(c, openAIVideosUpstreamEndpoint(endpoint))

	model, upstreamModel := "", ""
	method := http.MethodGet
	if endpoint == OpenAIVideosEndpointCreate {
		info, parseErr := ParseOpenAIVideoRequest(contentType, body)
		if parseErr != nil {
			return nil, parseErr
		}
		model = info.Model
		upstreamModel = account.GetMappedModel(model)
		if upstreamModel != model {
			body, contentType, err = rewriteOpenAIVideoModel(body, contentType, upstreamModel)
			if err != nil {
				return nil, err
			}
		}
		method = http.MethodPost
	}
	token := strings.TrimSpace(account.GetCredential("api_key"))
	if token == "" {
		return nil, fmt.Errorf("openai videos account missing api_key")
	}

	var reader io.Reader
	if method == http.MethodPost {
		reader = bytes.NewReader(body)
	}
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if method == http.MethodPost && strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if endpoint == OpenAIVideosEndpointContent {
		req.Header.Set("Accept", "*/*")
		if c != nil && c.Request != nil {
			if rng := strings.TrimSpace(c.GetHeader("Range")); rng != "" {
				req.Header.Set("Range", rng)
			}
		}
	} else {
		req.Header.Set("Accept", "application/json")
	}
	account.ApplyHeaderOverrides(req.Header)

	proxy := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxy = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// 异步创建一旦被上游接受就可能已经计费，不做账号重试。
	// 下载接口的失败响应也可能是视频流，按流返回，不走有大小限制的 JSON 读取。
	if resp.StatusCode >= 300 {
		if endpoint == OpenAIVideosEndpointContent {
			if err := writeGrokMediaContentResponse(c, resp); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("openai videos upstream status %d", resp.StatusCode)
		}
		responseBody, readErr := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
		if readErr != nil {
			return nil, readErr
		}
		writeGrokMediaResponse(c, resp, responseBody, s.responseHeaderFilter)
		return nil, fmt.Errorf("openai videos upstream status %d", resp.StatusCode)
	}

	result := &OpenAIForwardResult{
		Model:            model,
		BillingModel:     model,
		UpstreamModel:    upstreamModel,
		Duration:         time.Since(started),
		ResponseHeaders:  resp.Header.Clone(),
		UpstreamEndpoint: openAIVideosUpstreamEndpoint(endpoint),
	}
	if endpoint == OpenAIVideosEndpointContent {
		if err := writeGrokMediaContentResponse(c, resp); err != nil {
			return nil, err
		}
		return result, nil
	}

	responseBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if endpoint == OpenAIVideosEndpointCreate {
		id := strings.TrimSpace(gjson.GetBytes(responseBody, "id").String())
		if id == "" {
			return nil, fmt.Errorf("openai videos create response missing task ID")
		}
		result.ResponseID = OpenAIVideoTaskKey(id)
		if info, parseErr := ParseOpenAIVideoRequest(contentType, body); parseErr == nil {
			result.VideoResolution = info.Resolution
			result.VideoDurationSeconds = info.DurationSeconds
		}
	}
	if endpoint == OpenAIVideosEndpointStatus {
		result.ResponseID = OpenAIVideoTaskKey(strings.TrimPrefix(strings.TrimSpace(taskID), "openai-video:"))
		applyOpenAIVideoCompletion(result, responseBody)
	}
	writeGrokMediaResponse(c, resp, responseBody, s.responseHeaderFilter)
	return result, nil
}

func openAIVideosUpstreamEndpoint(endpoint GrokMediaEndpoint) string {
	switch endpoint {
	case OpenAIVideosEndpointContent:
		return openAIVideosUpstreamPath + "/{id}/content"
	case OpenAIVideosEndpointStatus:
		return openAIVideosUpstreamPath + "/{id}"
	default:
		return openAIVideosUpstreamPath
	}
}

// applyOpenAIVideoCompletion 只在上游明确返回 completed 时给出计费单位。
// 秒数和分辨率都取上游回显；取不到就留空，由计费挂接点决定不扣，
// 不用 Grok 的 8 秒 / 480p 默认值。
func applyOpenAIVideoCompletion(result *OpenAIForwardResult, body []byte) {
	if result == nil || !gjson.ValidBytes(body) {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(gjson.GetBytes(body, "status").String()), "completed") {
		return
	}
	result.VideoCount = 1
	result.VideoDurationSeconds = openAIVideoSeconds(gjson.GetBytes(body, "seconds"))
	result.VideoResolution = openAIVideoSizeTier(gjson.GetBytes(body, "size").String())
	if model := strings.TrimSpace(gjson.GetBytes(body, "model").String()); model != "" {
		result.UpstreamModel = model
	}
}

func rewriteOpenAIVideoModel(body []byte, contentType, model string) ([]byte, string, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		return rewriteOpenAIImagesMultipartModel(body, contentType, model)
	}
	rewritten, err := sjson.SetBytes(body, "model", model)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite video request model: %w", err)
	}
	return rewritten, contentType, nil
}
