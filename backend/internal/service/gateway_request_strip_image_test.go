//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func stripImageTestAccount(models []any) *Account {
	return &Account{
		Platform: "openai",
		Type:     "apikey",
		Extra:    map[string]any{"strip_image_input_models": models},
	}
}

func TestStripImageInputAsText_CCFormat(t *testing.T) {
	body := []byte(`{"model":"glm-5.2","messages":[{"role":"user","content":[{"type":"text","text":"describe this"},{"type":"image_url","image_url":{"url":"https://example.com/img.png"}}]}]}`)
	account := stripImageTestAccount([]any{"glm-5.2"})

	result, changed := StripImageInputAsText(body, "glm-5.2", account)
	require.True(t, changed)

	// Text part preserved
	assert.Equal(t, "describe this", gjson.GetBytes(result, "messages.0.content.0.text").String())
	// Image part replaced with placeholder text
	assert.Equal(t, "text", gjson.GetBytes(result, "messages.0.content.1.type").String())
	assert.Equal(t, imageStrippedPlaceholder, gjson.GetBytes(result, "messages.0.content.1.text").String())
	// image_url field gone
	assert.False(t, gjson.GetBytes(result, "messages.0.content.1.image_url").Exists())
}

func TestStripImageInputAsText_ResponsesFormat(t *testing.T) {
	body := []byte(`{"model":"glm-5.2","input":[{"role":"user","content":[{"type":"input_text","text":"what is this"},{"type":"input_image","image_url":"data:image/png;base64,abc123"}]}]}`)
	account := stripImageTestAccount([]any{"glm-5.2"})

	result, changed := StripImageInputAsText(body, "glm-5.2", account)
	require.True(t, changed)

	assert.Equal(t, "what is this", gjson.GetBytes(result, "input.0.content.0.text").String())
	assert.Equal(t, "input_text", gjson.GetBytes(result, "input.0.content.1.type").String())
	assert.Equal(t, imageStrippedPlaceholder, gjson.GetBytes(result, "input.0.content.1.text").String())
	assert.False(t, gjson.GetBytes(result, "input.0.content.1.image_url").Exists())
}

func TestStripImageInputAsText_TextOnly_NoChange(t *testing.T) {
	body := []byte(`{"model":"glm-5.2","messages":[{"role":"user","content":"hello"}]}`)
	account := stripImageTestAccount([]any{"glm-5.2"})

	_, changed := StripImageInputAsText(body, "glm-5.2", account)
	assert.False(t, changed)
}

func TestStripImageInputAsText_ModelNotConfigured(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://x.com/a.png"}}]}]}`)
	account := stripImageTestAccount([]any{"glm-5.2"})

	_, changed := StripImageInputAsText(body, "gpt-4o", account)
	assert.False(t, changed)
}

func TestStripImageInputAsText_NoConfig(t *testing.T) {
	body := []byte(`{"model":"glm-5.2","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://x.com/a.png"}}]}]}`)
	account := &Account{Platform: "openai", Type: "apikey", Extra: map[string]any{}}

	_, changed := StripImageInputAsText(body, "glm-5.2", account)
	assert.False(t, changed)
}

func TestStripImageInputAsText_MultipleMessages(t *testing.T) {
	body := []byte(`{"model":"glm-5.2","messages":[{"role":"user","content":[{"type":"text","text":"first"},{"type":"image_url","image_url":{"url":"https://a.com/1.png"}}]},{"role":"assistant","content":"ok"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://a.com/2.png"}},{"type":"text","text":"second"}]}]}`)
	account := stripImageTestAccount([]any{"glm-5.2"})

	result, changed := StripImageInputAsText(body, "glm-5.2", account)
	require.True(t, changed)

	// First message: image replaced
	assert.Equal(t, "text", gjson.GetBytes(result, "messages.0.content.1.type").String())
	assert.Equal(t, imageStrippedPlaceholder, gjson.GetBytes(result, "messages.0.content.1.text").String())
	// Assistant message untouched
	assert.Equal(t, "ok", gjson.GetBytes(result, "messages.1.content").String())
	// Third message: image replaced, text preserved
	assert.Equal(t, "text", gjson.GetBytes(result, "messages.2.content.0.type").String())
	assert.Equal(t, imageStrippedPlaceholder, gjson.GetBytes(result, "messages.2.content.0.text").String())
	assert.Equal(t, "second", gjson.GetBytes(result, "messages.2.content.1.text").String())
}

func TestStripImageInputAsText_MultipleModels(t *testing.T) {
	account := stripImageTestAccount([]any{"glm-5.2", "glm-4v"})

	body := []byte(`{"model":"glm-4v","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://x.com/a.png"}}]}]}`)
	_, changed := StripImageInputAsText(body, "glm-4v", account)
	assert.True(t, changed)

	body2 := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://x.com/a.png"}}]}]}`)
	_, changed2 := StripImageInputAsText(body2, "gpt-4o", account)
	assert.False(t, changed2)
}
