package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// denyBodyCtx 构造带 body 的测试请求上下文(模型名在 body 里)。
func denyBodyCtx(h map[string]string, body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	for k, v := range h {
		c.Request.Header.Set(k, v)
	}
	return c
}

func denyPolicy() CodexRestrictionPolicy {
	return CodexRestrictionPolicy{
		Blacklist: []openai.DeniedClientEntry{
			{UAContains: []string{"multica-agent-sdk"}},
			{ModelPatterns: []string{"*gpt*"}},
		},
	}
}

// apikey 账号(codex_cli_only 关闭)请求 gpt 模型 → 全局黑名单模型维度拦截。
func TestDetect_GlobalBlacklistModel_AnyAccount(t *testing.T) {
	det := NewOpenAICodexClientRestrictionDetector(nil)
	apikeyAcc := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}
	body := `{"model":"gpt-5.5","messages":[]}`

	r := det.Detect(denyBodyCtx(map[string]string{"User-Agent": "Mozilla/5.0"}, body), apikeyAcc, denyPolicy(), []byte(body))
	require.True(t, r.Enabled)
	require.False(t, r.Matched)
	require.Equal(t, CodexClientRestrictionReasonBlacklistedModel, r.Reason)
	require.Equal(t, CodexModelBlockedMessage, CodexClientRestrictionMessage(r))
}

// apikey 账号请求非黑名单模型 → 不拦(Disabled,黑名单未命中且开关未开)。
func TestDetect_GlobalBlacklistModel_NonMatchedModelPasses(t *testing.T) {
	det := NewOpenAICodexClientRestrictionDetector(nil)
	apikeyAcc := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}
	body := `{"model":"glm-5.2","messages":[]}`

	r := det.Detect(denyBodyCtx(map[string]string{"User-Agent": "Mozilla/5.0"}, body), apikeyAcc, denyPolicy(), []byte(body))
	require.False(t, r.Enabled)
	require.Equal(t, CodexClientRestrictionReasonDisabled, r.Reason)
}

// UA 维度黑名单对 apikey 账号同样全局生效(与模型无关)。
func TestDetect_GlobalBlacklistUA_AnyAccount(t *testing.T) {
	det := NewOpenAICodexClientRestrictionDetector(nil)
	apikeyAcc := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}
	body := `{"model":"glm-5.2","messages":[]}`

	r := det.Detect(denyBodyCtx(map[string]string{"User-Agent": "multica-agent-sdk/0.146.0 (Mac)"}, body), apikeyAcc, denyPolicy(), []byte(body))
	require.True(t, r.Enabled)
	require.False(t, r.Matched)
	require.Equal(t, CodexClientRestrictionReasonBlacklisted, r.Reason)
	require.Equal(t, CodexOfficialClientsOnlyMessage, CodexClientRestrictionMessage(r))
}

// OAuth 账号未开 codex_cli_only:黑名单命中仍拒(第 0 步全局生效);未命中走 Disabled(行为不变)。
func TestDetect_GlobalBlacklist_OAuthSwitchOff(t *testing.T) {
	det := NewOpenAICodexClientRestrictionDetector(nil)
	oauthAcc := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}
	body := `{"model":"gpt-5.5","messages":[]}`

	r := det.Detect(denyBodyCtx(map[string]string{"User-Agent": "Mozilla/5.0"}, body), oauthAcc, denyPolicy(), []byte(body))
	require.True(t, r.Enabled)
	require.False(t, r.Matched)
	require.Equal(t, CodexClientRestrictionReasonBlacklistedModel, r.Reason)

	body2 := `{"model":"glm-5.2","messages":[]}`
	r2 := det.Detect(denyBodyCtx(map[string]string{"User-Agent": "Mozilla/5.0"}, body2), oauthAcc, denyPolicy(), []byte(body2))
	require.False(t, r2.Enabled)
	require.Equal(t, CodexClientRestrictionReasonDisabled, r2.Reason)
}

// 空 body / 非 JSON body:模型维度安全忽略,不误拦。
func TestDetect_GlobalBlacklistModel_EmptyBodySafe(t *testing.T) {
	det := NewOpenAICodexClientRestrictionDetector(nil)
	apikeyAcc := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}

	r := det.Detect(denyBodyCtx(map[string]string{"User-Agent": "Mozilla/5.0"}, ""), apikeyAcc, denyPolicy(), nil)
	require.False(t, r.Enabled)
}

// 黑名单为空时一切照旧。
func TestDetect_EmptyBlacklist_Noop(t *testing.T) {
	det := NewOpenAICodexClientRestrictionDetector(nil)
	apikeyAcc := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}
	body := `{"model":"gpt-5.5","messages":[]}`

	r := det.Detect(denyBodyCtx(map[string]string{"User-Agent": "Mozilla/5.0"}, body), apikeyAcc, CodexRestrictionPolicy{}, []byte(body))
	require.False(t, r.Enabled)
}
