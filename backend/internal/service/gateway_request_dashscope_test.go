//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsDashScopeUpstream(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		baseURL  string
		expected bool
	}{
		{"qwen model prefix", "qwen3-235b-a22b", "", true},
		{"qwen model prefix uppercase", "Qwen-Plus", "", true},
		{"dashscope URL with non-qwen model", "glm-5.2", "https://dashscope.aliyuncs.com/compatible-mode/v1", true},
		{"aliyuncs URL with non-qwen model", "glm-5.2", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", true},
		{"non-qwen model without dashscope URL", "glm-5.2", "https://api.openai.com", false},
		{"non-qwen model empty URL", "gpt-4o", "", false},
		{"kimi model", "kimi-k2.6", "", false},
		{"empty model with dashscope URL", "", "https://dashscope.aliyuncs.com/compatible-mode/v1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDashScopeUpstream(tt.model, tt.baseURL)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestStripQwenReasoningEffort_DashScopeURL(t *testing.T) {
	// Non-qwen model (glm-5.2) but DashScope upstream → should strip.
	body := []byte(`{"model":"glm-5.2","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}`)
	result, changed := StripQwenReasoningEffort(body, "glm-5.2", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	require.True(t, changed)
	assert.False(t, gjson.GetBytes(result, "reasoning_effort").Exists())
	assert.Equal(t, "glm-5.2", gjson.GetBytes(result, "model").String())
}

func TestStripQwenReasoningEffort_QwenPrefix(t *testing.T) {
	// Qwen model without explicit base URL → fallback to prefix match.
	body := []byte(`{"model":"qwen3-235b-a22b","reasoning_effort":"medium","messages":[]}`)
	result, changed := StripQwenReasoningEffort(body, "qwen3-235b-a22b", "")
	require.True(t, changed)
	assert.False(t, gjson.GetBytes(result, "reasoning_effort").Exists())
}

func TestStripQwenReasoningEffort_NestedReasoning(t *testing.T) {
	body := []byte(`{"model":"glm-5.2","reasoning":{"effort":"high","summary":"auto"},"messages":[]}`)
	result, changed := StripQwenReasoningEffort(body, "glm-5.2", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	require.True(t, changed)
	assert.False(t, gjson.GetBytes(result, "reasoning.effort").Exists())
	// "summary" remains so "reasoning" object is not empty → not cleaned up.
	assert.True(t, gjson.GetBytes(result, "reasoning.summary").Exists())
}

func TestStripQwenReasoningEffort_EmptyReasoningCleanup(t *testing.T) {
	body := []byte(`{"model":"glm-5.2","reasoning":{"effort":"low"},"messages":[]}`)
	result, changed := StripQwenReasoningEffort(body, "glm-5.2", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	require.True(t, changed)
	// Entire "reasoning" object removed because it became empty.
	assert.False(t, gjson.GetBytes(result, "reasoning").Exists())
}

func TestStripQwenReasoningEffort_NonDashScope_NoStrip(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","reasoning_effort":"high","messages":[]}`)
	result, changed := StripQwenReasoningEffort(body, "gpt-4o", "https://api.openai.com")
	require.False(t, changed)
	assert.Equal(t, "high", gjson.GetBytes(result, "reasoning_effort").String())
}

func TestStripQwenReasoningEffort_NoReasoningField(t *testing.T) {
	body := []byte(`{"model":"glm-5.2","messages":[]}`)
	_, changed := StripQwenReasoningEffort(body, "glm-5.2", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	require.False(t, changed)
}
