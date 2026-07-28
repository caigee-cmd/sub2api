package apicompat

import (
	"regexp"
	"strings"
)

// Qwen XML leak filter.
//
// Qwen models trained with XML chat-template tool-calling sometimes leak
// internal markup into text output (e.g. tool_response, invoke, parameter
// tags and their closing variants). This happens primarily under tool-heavy
// prompts when thinking_budget truncates reasoning mid-tool-selection.
//
// Additionally, client system prompts sometimes instruct the model to wrap
// thinking in XML tags (analysis, summary, think, thinking, thought,
// reflection, reasoning). When enable_thinking=true forces reasoning into
// reasoning_content, those tags leak into the client's reasoning fold as
// visible XML markup.
//
// This filter strips all such leaked tags from response text. It is applied
// only to Qwen upstream models and is a no-op for all other providers.

// qwenXMLTagNames enumerates all tag names this filter strips: tool-call
// tags plus thinking/reasoning tags triggered by client prompts.
var qwenXMLTagNames = `tool_response|invoke|parameter|tool_call|function_call|antml:invoke|antml:parameter|analysis|summary|think|thinking|thought|reflection|reasoning`

// qwenXMLToolCallPattern matches complete XML tags (tool-call + thinking):
// opening tags (with optional attributes), closing tags, and self-closing tags.
var qwenXMLToolCallPattern = regexp.MustCompile(
	`</?(?:` + qwenXMLTagNames + `)(?:\s[^>]*)?/?>`,
)

// qwenXMLToolCallPrefixPattern matches a trailing partial tag that might be
// completed in the next streaming chunk.
var qwenXMLToolCallPrefixPattern = regexp.MustCompile(
	`<[/]?(?:` + qwenXMLTagNames + `)?[^>]*$`,
)

// StripQwenXMLToolCallTags removes all leaked Qwen XML tags from text —
// both tool-call tags and thinking/reasoning tags (see qwenXMLTagNames).
// Used for non-streaming responses where the full text is available.
// Name retained for backward compatibility with existing callers.
func StripQwenXMLToolCallTags(text string) string {
	if !strings.Contains(text, "<") {
		return text
	}
	return qwenXMLToolCallPattern.ReplaceAllString(text, "")
}

// QwenXMLStreamFilter buffers streaming text deltas to strip XML tool-call
// tags that may span chunk boundaries.
type QwenXMLStreamFilter struct {
	buf     strings.Builder
	enabled bool
}

// NewQwenXMLStreamFilter creates a filter. enabled=false makes all methods
// pass-through (zero overhead for non-Qwen models).
func NewQwenXMLStreamFilter(enabled bool) *QwenXMLStreamFilter {
	return &QwenXMLStreamFilter{enabled: enabled}
}

// Write accepts a text delta and returns the safe-to-emit portion.
// It holds back trailing bytes that look like the start of an XML tag.
func (f *QwenXMLStreamFilter) Write(delta string) string {
	if !f.enabled || delta == "" {
		return delta
	}

	f.buf.WriteString(delta)
	s := f.buf.String()

	// Strip complete XML tags.
	s = qwenXMLToolCallPattern.ReplaceAllString(s, "")

	// Check if the tail looks like a partial XML tag — hold it back.
	loc := qwenXMLToolCallPrefixPattern.FindStringIndex(s)
	if loc != nil {
		tail := s[loc[0]:]
		if strings.HasPrefix(tail, "<") {
			safe := s[:loc[0]]
			f.buf.Reset()
			f.buf.WriteString(tail)
			return safe
		}
	}

	// No partial tag — flush everything.
	f.buf.Reset()
	return s
}

// Flush returns any remaining buffered content (called at stream end).
// Partial XML tags that never completed are stripped.
func (f *QwenXMLStreamFilter) Flush() string {
	if !f.enabled {
		return ""
	}
	s := f.buf.String()
	f.buf.Reset()
	if s == "" {
		return ""
	}
	// Strip any complete tags.
	s = qwenXMLToolCallPattern.ReplaceAllString(s, "")
	// Strip any remaining partial tag at the end (never completed).
	if loc := qwenXMLToolCallPrefixPattern.FindStringIndex(s); loc != nil {
		if strings.HasPrefix(s[loc[0]:], "<") {
			s = s[:loc[0]]
		}
	}
	return s
}
