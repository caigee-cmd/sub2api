package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchModelPattern(t *testing.T) {
	cases := []struct {
		pattern string
		model   string
		want    bool
	}{
		// 通配命中
		{"*gpt*", "gpt-5.5", true},
		{"*gpt*", "gpt-5.6-sol", true},
		{"*gpt*", "GPT-5.6-TERRA", true}, // 大小写不敏感
		{"gpt-*", "gpt-5.5", true},
		{"gpt-5*", "gpt-5.6-sol", true},
		{"gpt-5.6-???", "gpt-5.6-sol", true}, // ? 单字符
		{"doubao*", "doubao-seed-evolving", true},
		// 精确等值
		{"gpt-5.5", "gpt-5.5", true},
		{"*gpt-5.5", "gpt-5.5", true},
		// 不命中
		{"*gpt*", "glm-5.2", false},
		{"*gpt*", "", false},
		{"gpt-5*", "claude-4.5", false},
		{"gpt-5.6-???", "gpt-5.6-solx", false}, // 长度不齐
		{"", "gpt-5.5", false},                 // 空模式安全忽略
		{"*gpt*", "  ", false},                 // 空白模型安全忽略
	}
	for _, c := range cases {
		got := MatchModelPattern(c.pattern, c.model)
		require.Equal(t, c.want, got, "pattern=%q model=%q", c.pattern, c.model)
	}
}

func TestIsDeniedModelMatch(t *testing.T) {
	require.False(t, IsDeniedModelMatch("gpt-5.5", nil))
	require.False(t, IsDeniedModelMatch("gpt-5.5", []string{}))
	require.True(t, IsDeniedModelMatch("gpt-5.5", []string{"*gpt*"}))
	require.True(t, IsDeniedModelMatch("glm-5.2", []string{"*gpt*", "glm-*"}))
	require.False(t, IsDeniedModelMatch("glm-5.2", []string{"*gpt*"}))
}

func TestIsDeniedEntryMatch_WithModel(t *testing.T) {
	t.Run("模型维度命中", func(t *testing.T) {
		e := DeniedClientEntry{ModelPatterns: []string{"*gpt*"}}
		require.True(t, IsDeniedEntryMatch("Mozilla/5.0", "", "gpt-5.5", e))
		require.False(t, IsDeniedEntryMatch("Mozilla/5.0", "", "glm-5.2", e))
	})
	t.Run("UA 维度命中(既有语义不变)", func(t *testing.T) {
		e := DeniedClientEntry{UAContains: []string{"multica-agent-sdk"}}
		require.True(t, IsDeniedEntryMatch("multica-agent-sdk/0.146.0", "", "glm-5.2", e))
	})
	t.Run("originator 维度命中", func(t *testing.T) {
		e := DeniedClientEntry{Originator: "evil"}
		require.True(t, IsDeniedEntryMatch("x", "evil", "glm-5.2", e))
	})
	t.Run("全空条目安全忽略", func(t *testing.T) {
		e := DeniedClientEntry{}
		require.False(t, IsDeniedEntryMatch("x", "y", "gpt-5.5", e))
	})
	t.Run("模型模式全空白安全忽略", func(t *testing.T) {
		e := DeniedClientEntry{ModelPatterns: []string{"  ", ""}}
		require.False(t, IsDeniedEntryMatch("x", "y", "gpt-5.5", e))
	})
}

func TestMatchDenyEntriesWithModel(t *testing.T) {
	entries := []DeniedClientEntry{
		{UAContains: []string{"multica-agent-sdk"}},
		{ModelPatterns: []string{"*gpt*"}},
	}
	require.True(t, MatchDenyEntriesWithModel("multica-agent-sdk/0.146.0", "", "glm-5.2", entries))
	require.True(t, MatchDenyEntriesWithModel("Mozilla/5.0", "", "gpt-5.5", entries))
	require.False(t, MatchDenyEntriesWithModel("Mozilla/5.0", "", "glm-5.2", entries))
	require.False(t, MatchDenyEntriesWithModel("Mozilla/5.0", "", "gpt-5.5", nil))
}

func TestHasDenyModelPatterns(t *testing.T) {
	require.False(t, HasDenyModelPatterns(nil))
	require.False(t, HasDenyModelPatterns([]DeniedClientEntry{{UAContains: []string{"x"}}}))
	require.True(t, HasDenyModelPatterns([]DeniedClientEntry{{ModelPatterns: []string{"*gpt*"}}}))
}

// 向后兼容:旧 JSON(仅 originator/ua_contains)Unmarshal 到 DeniedClientEntry 后行为不变。
func TestDeniedClientEntry_BackwardCompatibleJSON(t *testing.T) {
	raw := `[{"originator":"evil"},{"ua_contains":["badbot/"]}]`
	var entries []DeniedClientEntry
	require.NoError(t, json.Unmarshal([]byte(raw), &entries))
	require.Len(t, entries, 2)
	require.True(t, IsDeniedEntryMatch("x", "evil", "gpt-5.5", entries[0]))
	require.True(t, IsDeniedEntryMatch("badbot/1.0", "", "gpt-5.5", entries[1]))
	require.False(t, IsDeniedEntryMatch("good/1.0", "fine", "glm-5.2", entries[0]))
}
