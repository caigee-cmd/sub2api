package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsCodexCLIOnlyAppServerAllowed(t *testing.T) {
	t.Run("codex_cli_only 开 + allow_app_server=true → true", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true, "codex_cli_only_allow_app_server": true},
		}
		require.True(t, account.IsCodexCLIOnlyAppServerAllowed())
	})

	t.Run("codex_cli_only 开 + allow_app_server=false → false", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true, "codex_cli_only_allow_app_server": false},
		}
		require.False(t, account.IsCodexCLIOnlyAppServerAllowed())
	})

	t.Run("codex_cli_only 开 + 字段缺失 → false", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}
		require.False(t, account.IsCodexCLIOnlyAppServerAllowed())
	})

		t.Run("codex_cli_only 关 → 即便 allow_app_server=true 也 false", func(t *testing.T) {
			account := &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra:    map[string]any{"codex_cli_only_allow_app_server": true},
			}
			require.False(t, account.IsCodexCLIOnlyAppServerAllowed())
		})
	}

	func TestAccount_GetCodexCLIOnlyBlacklist(t *testing.T) {
		t.Run("[]any JSON 数组正常解析", func(t *testing.T) {
			account := &Account{
				Extra: map[string]any{
					"codex_cli_only_blacklist": []any{
						map[string]any{"ua_contains": []any{"multica-agent-sdk"}},
						map[string]any{"model_patterns": []any{"*gpt*"}},
					},
				},
			}
			entries := account.GetCodexCLIOnlyBlacklist()
			require.Len(t, entries, 2)
			require.Equal(t, []string{"multica-agent-sdk"}, entries[0].UAContains)
			require.Equal(t, []string{"*gpt*"}, entries[1].ModelPatterns)
		})

		t.Run("字符串 JSON 正常解析", func(t *testing.T) {
			account := &Account{
				Extra: map[string]any{
					"codex_cli_only_blacklist": `[{"ua_contains":["multica-agent-sdk"]}]`,
				},
			}
			entries := account.GetCodexCLIOnlyBlacklist()
			require.Len(t, entries, 1)
			require.Equal(t, []string{"multica-agent-sdk"}, entries[0].UAContains)
		})

		t.Run("缺失 / 空 / 非法 → nil", func(t *testing.T) {
			require.Nil(t, (&Account{}).GetCodexCLIOnlyBlacklist())
			require.Nil(t, (&Account{Extra: map[string]any{}}).GetCodexCLIOnlyBlacklist())
			require.Nil(t, (&Account{Extra: map[string]any{"codex_cli_only_blacklist": ""}}).GetCodexCLIOnlyBlacklist())
			require.Nil(t, (&Account{Extra: map[string]any{"codex_cli_only_blacklist": "not-json"}}).GetCodexCLIOnlyBlacklist())
			require.Nil(t, (&Account{Extra: map[string]any{"codex_cli_only_blacklist": []any{}}}).GetCodexCLIOnlyBlacklist())
		})
	}
