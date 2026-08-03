//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// sseHandler 返回 OpenAI chat completions 风格的 SSE 流。
// 从请求体的 messages 里解析 challenge 题目动态作答，答案拆成两段增量发送，
// 用于验证流式探测的 TTFT 记录与增量文本累积。
// wrongAnswer=true 时给出错误答案，用于触发 challenge mismatch。
type sseHandler struct {
	wrongAnswer bool
	status      int
}

func (h *sseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.status == 0 {
		h.status = http.StatusOK
	}
	defer func() { _ = r.Body.Close() }()
	var parsed map[string]any
	_ = json.NewDecoder(r.Body).Decode(&parsed)
	answer := sseAnswer(parsed, h.wrongAnswer)

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(h.status)
	flusher, _ := w.(http.Flusher)
	for _, chunk := range splitAnswer(answer) {
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":` + jsonString(chunk) + `}}]}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

// sseAnswer 从请求体提取 challenge 题目并作答。
// 错误答案 = 正确答案 + 10：偏移量足够大，错误值的十进制表示不可能包含
// 正确值作为子串（challenge 校验按子串匹配），保证 mismatch 判定成立。
func sseAnswer(body map[string]any, wrong bool) string {
	prompt, _ := body["input"].(string)
	if prompt == "" {
		if messages, ok := body["messages"].([]any); ok && len(messages) > 0 {
			if msg, ok := messages[0].(map[string]any); ok {
				prompt, _ = msg["content"].(string)
			}
		}
	}
	m := challengeQuestionRegex.FindStringSubmatch(prompt)
	if len(m) != 4 {
		return "0"
	}
	left, _ := strconv.Atoi(m[1])
	right, _ := strconv.Atoi(m[3])
	answer := left - right
	if m[2] == "+" {
		answer = left + right
	}
	if wrong {
		answer += 10
	}
	return strconv.Itoa(answer)
}

// splitAnswer 把答案拆成两段，保证走多增量累积路径（单字符答案不拆）。
func splitAnswer(answer string) []string {
	if len(answer) < 2 {
		return []string{answer}
	}
	return []string{answer[:1], answer[1:]}
}

func jsonString(s string) string {
	// 测试内容只有纯 ASCII 数字，手写转义即可。
	return `"` + s + `"`
}

func setupFakeSSE(t *testing.T, h *sseHandler) string {
	t.Helper()
	swapMonitorHTTPClient(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRunCheckForModel_OpenAI_StreamResponse_RecordsFirstToken(t *testing.T) {
	h := &sseHandler{}
	endpoint := setupFakeSSE(t, h)

	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, endpoint, "sk-openai", "gpt-test", nil)

	if res.Status != MonitorStatusOperational {
		t.Fatalf("streamed challenge should pass, got status=%s message=%q", res.Status, res.Message)
	}
	if res.FirstTokenMs == nil {
		t.Fatal("SSE response should record first-token latency")
	}
	if res.LatencyMs == nil {
		t.Fatal("SSE response should still record total latency")
	}
	if *res.FirstTokenMs > *res.LatencyMs {
		t.Errorf("first-token latency (%d) must not exceed total latency (%d)", *res.FirstTokenMs, *res.LatencyMs)
	}
}

func TestRunCheckForModel_OpenAI_StreamResponse_ChallengeMismatchFails(t *testing.T) {
	// 流式路径也必须走 challenge 校验：错误答案 → failed。
	h := &sseHandler{wrongAnswer: true}
	endpoint := setupFakeSSE(t, h)

	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, endpoint, "sk-openai", "gpt-test", nil)

	if res.Status != MonitorStatusFailed {
		t.Fatalf("wrong streamed answer should be failed, got status=%s message=%q", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "challenge mismatch") {
		t.Fatalf("expected challenge mismatch message, got %q", res.Message)
	}
}

func TestRunCheckForModel_OpenAI_StreamErrorResponse_RecordsError(t *testing.T) {
	// 非 2xx 的 SSE 响应走普通错误路径（rawBody 解析）。
	h := &sseHandler{status: http.StatusServiceUnavailable}
	endpoint := setupFakeSSE(t, h)

	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, endpoint, "sk-openai", "gpt-test", nil)

	if res.Status != MonitorStatusError {
		t.Fatalf("503 SSE response should be error, got status=%s message=%q", res.Status, res.Message)
	}
}

func TestExtractSSEDataLine(t *testing.T) {
	cases := []struct {
		line string
		data string
		ok   bool
	}{
		{"data: {}", "{}", true},
		{"data:[DONE]", "", false},
		{"data: ", "", false},
		{": comment", "", false},
		{"event: message", "", false},
	}
	for _, c := range cases {
		got, ok := extractSSEDataLine(c.line)
		if ok != c.ok || got != c.data {
			t.Errorf("extractSSEDataLine(%q) = (%q, %v), want (%q, %v)", c.line, got, ok, c.data, c.ok)
		}
	}
}
