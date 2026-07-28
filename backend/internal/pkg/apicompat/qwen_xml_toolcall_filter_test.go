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

		// Thinking tags (new)
		{"analysis tag", "<analysis>hidden</analysis>", "hidden"},
		{"summary tag", "<summary>hidden</summary>", "hidden"},
		{"think tag", "<think>hidden</think>", "hidden"},
		{"thinking tag", "<thinking>hidden</thinking>", "hidden"},
		{"thought tag", "<thought>hidden</thought>", "hidden"},
		{"reflection tag", "<reflection>hidden</reflection>", "hidden"},
		{"reasoning tag", "<reasoning>hidden</reasoning>", "hidden"},

		// Self-closing and attributes
		{"self-closing analysis", "<analysis/>", ""},
		{"analysis with attrs", "<analysis type=\"deep\">x</analysis>", "x"},

		// Non-tag content untouched
		{"markdown bold", "**bold** text", "**bold** text"},
		{"heading", "### Summary", "### Summary"},
		{"no angle brackets", "plain text here", "plain text here"},

		// Mixed
		{"mixed tags and text", "<analysis>think</analysis><summary>sum</summary>", "thinksum"},
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
		out1 := f.Write("<anal")
		require.Equal(t, "", out1)
		out2 := f.Write("ysis>content</analysis>")
		require.Equal(t, "content", out2)
	})

	t.Run("complete tag in one chunk", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(true)
		out := f.Write("<analysis>x</analysis>")
		require.Equal(t, "x", out)
	})

	t.Run("plain text passthrough", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(true)
		require.Equal(t, "hello", f.Write("hello"))
		require.Equal(t, " world", f.Write(" world"))
	})

	t.Run("disabled passthrough", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(false)
		inp := "<analysis>x</analysis>"
		require.Equal(t, inp, f.Write(inp))
	})

	t.Run("flush discards partial tag", func(t *testing.T) {
		f := NewQwenXMLStreamFilter(true)
		f.Write("<analy")
		require.Equal(t, "", f.Flush())
	})
}
