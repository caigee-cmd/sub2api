package handler

import (
	"context"
	"mime"
	"net/http"
	"strings"
	"time"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SeedanceTasks exposes Ark's native asynchronous video task protocol.
func (h *OpenAIGatewayHandler) SeedanceTasks(c *gin.Context) {
	if c.Request.Method == http.MethodPost && c.GetHeader("Content-Type") != "" {
		mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || mediaType != "application/json" {
			h.errorResponse(c, http.StatusUnsupportedMediaType, "invalid_request_error", "Seedance requires application/json")
			return
		}
	}
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key.Group == nil || (key.Group.Platform != service.PlatformOpenAI && key.Group.Platform != service.PlatformComposite) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "Seedance requires an OpenAI or composite group")
		return
	}
	endpoint := service.SeedanceEndpointCreate
	setActualUpstreamEndpoint(c, EndpointSeedanceTasks)
	taskID := ""
	if c.Request.Method != http.MethodPost {
		taskID = service.SeedanceTaskKey(c.Param("task_id"))
		endpoint = service.SeedanceEndpointStatus
		if c.Request.Method == http.MethodDelete {
			endpoint = service.SeedanceEndpointDelete
		}
	}
	h.handleGrokMedia(c, endpoint, taskID)
}

// Ark reports actual completion tokens. Never infer tokens from duration or use
// Grok's per-second video tariff. Repeated polls share the durable task dedup key.
func prepareSeedanceCompletionBilling(ctx context.Context, h *OpenAIGatewayHandler, key *service.APIKey, subject middleware.AuthSubject, taskID string, result *service.OpenAIForwardResult) *service.OpenAIForwardResult {
	if result == nil || result.Usage.OutputTokens <= 0 {
		return nil
	}
	pending, err := h.gatewayService.LoadGrokVideoPendingBilling(ctx, taskID, subject.UserID, key.ID)
	if err != nil || pending == nil {
		return nil
	}
	claimed, err := h.gatewayService.ClaimGrokVideoBilling(ctx, taskID, subject.UserID, key.ID)
	if err != nil || !claimed {
		return nil
	}
	merged := *result
	merged.Model = pending.Model
	merged.BillingModel = firstNonEmptyString(pending.BillingModel, pending.Model)
	merged.UpstreamModel = firstNonEmptyString(pending.UpstreamModel, result.UpstreamModel)
	merged.RequestID = service.StableGrokVideoBillingRequestID(taskID)
	merged.ResponseID = taskID
	merged.Duration = service.GrokVideoE2EDuration(pending.CreatedAt, time.Now())
	return &merged
}

// prepareOpenAIVideoCompletionBilling 在上游返回 completed 时扣一次。
// 秒数和分辨率优先用完成回显，缺失时用创建时记下的值。两边都没有就先不扣，
// 避免套用 Grok 的默认 8 秒 / 480p。单价交给现有的渠道与分组计费，这里不定价。
// 重复轮询共用同一个任务去重键。
func prepareOpenAIVideoCompletionBilling(ctx context.Context, h *OpenAIGatewayHandler, reqLog *zap.Logger, key *service.APIKey, subject middleware.AuthSubject, taskID string, result *service.OpenAIForwardResult) *service.OpenAIForwardResult {
	if h == nil || h.gatewayService == nil || key == nil || result == nil || result.VideoCount <= 0 {
		return nil
	}
	taskID = strings.TrimSpace(firstNonEmptyString(taskID, result.ResponseID))
	if taskID == "" {
		return nil
	}
	pending, loadErr := h.gatewayService.LoadGrokVideoPendingBilling(ctx, taskID, subject.UserID, key.ID)
	if loadErr != nil && reqLog != nil {
		reqLog.Warn("openai_videos.pending_billing_load_failed", zap.String("request_id", taskID), zap.Error(loadErr))
	}
	merged := *result
	if pending != nil {
		merged.Model = firstNonEmptyString(pending.BillingModel, pending.Model, merged.Model)
		merged.BillingModel = firstNonEmptyString(pending.BillingModel, pending.Model, merged.BillingModel)
		merged.UpstreamModel = firstNonEmptyString(merged.UpstreamModel, pending.UpstreamModel)
		if merged.VideoDurationSeconds <= 0 {
			merged.VideoDurationSeconds = pending.VideoDurationSeconds
		}
		if strings.TrimSpace(merged.VideoResolution) == "" {
			merged.VideoResolution = pending.VideoResolution
		}
		if e2e := service.GrokVideoE2EDuration(pending.CreatedAt, time.Now()); e2e > 0 {
			merged.Duration = e2e
		}
	}
	if merged.VideoDurationSeconds <= 0 || strings.TrimSpace(merged.VideoResolution) == "" {
		if reqLog != nil {
			reqLog.Error("openai_videos.billing_skipped_missing_meter",
				zap.String("request_id", taskID),
				zap.Int("duration_seconds", merged.VideoDurationSeconds),
				zap.String("resolution", merged.VideoResolution),
			)
		}
		return nil
	}
	if _, ok := service.LookupVideoBillingResolution(merged.VideoResolution); !ok {
		if reqLog != nil {
			reqLog.Error("openai_videos.billing_skipped_unknown_resolution",
				zap.String("request_id", taskID),
				zap.String("resolution", merged.VideoResolution),
			)
		}
		return nil
	}
	claimed, err := h.gatewayService.ClaimGrokVideoBilling(ctx, taskID, subject.UserID, key.ID)
	if err != nil || !claimed {
		return nil
	}
	merged.Model = firstNonEmptyString(merged.Model, merged.BillingModel)
	merged.BillingModel = firstNonEmptyString(merged.BillingModel, merged.Model)
	merged.RequestID = service.StableGrokVideoBillingRequestID(taskID)
	merged.ResponseID = taskID
	merged.VideoCount = 1
	merged.ImageCount = 0
	return &merged
}

// OpenAIVideos 承接 OpenAI 兼容的异步视频协议。
// 仅 /v1/videos、/videos 以及它们的任务查询和下载；
// /videos/generations 仍是 Grok 的路径，不进这里。
func (h *OpenAIGatewayHandler) OpenAIVideos(c *gin.Context) {
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key.Group == nil || (key.Group.Platform != service.PlatformOpenAI && key.Group.Platform != service.PlatformComposite) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "OpenAI videos require an OpenAI or composite group")
		return
	}
	requestID := ""
	endpoint := service.OpenAIVideosEndpointCreate
	taskID := ""
	// 创建是 POST，没有任务号。测试上下文可能残留别的路由参数，不能拿来判断。
	if c.Request.Method == http.MethodGet {
		requestID = strings.TrimSpace(c.Param("request_id"))
		if requestID == "" {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "request_id is required")
			return
		}
		endpoint = service.OpenAIVideosEndpointStatus
		if strings.HasSuffix(c.Request.URL.Path, "/content") {
			endpoint = service.OpenAIVideosEndpointContent
		}
		taskID = service.OpenAIVideoTaskKey(requestID)
	}
	setActualUpstreamEndpoint(c, "/v1/videos")
	h.handleGrokMedia(c, endpoint, taskID)
}
