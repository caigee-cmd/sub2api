package logredact

import "testing"

func TestRedactUpstreamURL(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{name: "empty", msg: "", want: ""},
		{name: "plain no url", msg: "Invalid API key", want: "Invalid API key"},
		{name: "https url", msg: "dial tcp https://upstream.example.com: timeout", want: UpstreamRequestFailedMessage},
		{name: "http url", msg: "Get \"http://localhost:8080/v1\": connection refused", want: UpstreamRequestFailedMessage},
		{name: "uppercase HTTPS", msg: "request to HTTPS://Example.com failed", want: UpstreamRequestFailedMessage},
		{name: "url mid sentence", msg: "upstream https://api.openai.com/v1/chat/completions returned 502", want: UpstreamRequestFailedMessage},
		{name: "no scheme hostname only", msg: "lookup upstream.example.com: no such host", want: "lookup upstream.example.com: no such host"},
		{name: "generic error", msg: "No available accounts supporting model: gpt-4", want: "No available accounts supporting model: gpt-4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactUpstreamURL(tt.msg); got != tt.want {
				t.Errorf("RedactUpstreamURL(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}
