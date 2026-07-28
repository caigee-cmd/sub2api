package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripQwenXMLToolCallTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Tool-call tag regression
		{"tool_response tag", "<tool_response>body</tool_response>", "body"},
		{"invoke tag", "<invoke>body</invoke>", "body"},
		{"parameter tag", "<parameter>body</parameter>", "body"},
		{"tool_call tag", "<tool_call>body</tool_call>", "body"},
		{"function_call tag", "<function_call>body</function_call>", "body"},
		{"antml_invoke tag", "<antml:invoke>body</antml:invoke>", "body"},
		{"antml_parameter tag", "<antml:parameter>body</antml:parameter>", "body"},

		// Self-closing and attributes (tool-call tags only)
		{"self-closing invoke", "<invoke/>", ""},
		{"invoke with attrs", "<invoke name=\"x\">y</invoke>", "y"},

		// Thinking/reasoning-style tags are NOT stripped: clients legitimately
		// instruct models to wrap output in them (e.g. Claude Code compact).
		{"analysis tag kept", "<analysis>hidden</analysis>", "<analysis>hidden</analysis>"},
		{"summary tag kept", "<summary>hidden</summary>", "<summary>hidden</summary>"},
		{"think tag kept", "<think>hidden</think>", "<think>hidden</think>"},
		{"thinking tag kept", "<thinking>hidden</thinking>", "<thinking>hidden</thinking>"},
		{"thought tag kept", "<thought>hidden</thought>", "<thought>hidden</thought>"},
		{"reflection tag kept", "<reflection>hidden</reflection>", "<reflection>hidden</reflection>"},
		{"reasoning tag kept", "<reasoning>hidden</reasoning>", "<reasoning>hidden</reasoning>"},

		// Non-tag content untouched
		{"markdown bold", "**bold** text", "**bold** text"},
		{"heading", "### Summary", "### Summary"},
		{"no angle brackets", "plain text here", "plain text here"},

		// Mixed: tool-call tags stripped, thinking tags kept
		{"mixed tags and text", "<invoke>think</invoke><summary>sum</summary>", "think<summary>sum</summary>"},

		// Regression: a compact summary wrapped in <summary> must survive intact.
		{"compact summary intact", "<summary>\n1. Primary Request and Intent:\n   X.\n</summary>", "<summary>\n1. Primary Request and Intent:\n   X.\n</summary>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripQwenXMLToolCallTags(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestQwenXMLStreamFilter(t *testing.T) {
	t.Run("cross-chunk tag", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(true)
		out1 := f.Write("<inv")
		require.Equal(t, "", out1)
		out2 := f.Write("oke>content</invoke>")
		require.Equal(t, "content", out2)
	})

	t.Run("complete tag in one chunk", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(true)
		out := f.Write("<invoke>x</invoke>")
		require.Equal(t, "x", out)
	})

	t.Run("plain text passthrough", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(true)
		require.Equal(t, "hello", f.Write("hello"))
		require.Equal(t, " world", f.Write(" world"))
	})

	t.Run("thinking tag passthrough", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(true)
		inp := "<summary>x</summary>"
		out := f.Write(inp) + f.Flush()
		require.Equal(t, inp, out)
	})

	t.Run("disabled passthrough", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(false)
		inp := "<invoke>x</invoke>"
		require.Equal(t, inp, f.Write(inp))
	})

	t.Run("flush discards partial tag", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(true)
		f.Write("<inv")
		require.Equal(t, "", f.Flush())
	})
}
