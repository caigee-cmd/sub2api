package logredact

import "regexp"

// upstreamURLPattern matches any http(s):// URL embedded in a string. Used to
// detect error messages that would leak upstream account/deployment info
// (base URLs of upstream LLM providers) to end users.
var upstreamURLPattern = regexp.MustCompile(`(?i)https?://`)

// UpstreamRequestFailedMessage is the generic fallback returned when an error
// message would otherwise expose an upstream URL. Exported so handlers can use
// it directly when they need to build the fallback themselves.
const UpstreamRequestFailedMessage = "Upstream request failed"

// RedactUpstreamURL returns UpstreamRequestFailedMessage whenever msg contains
// an http(s):// URL; otherwise it returns msg unchanged.
//
// Unlike the finer-grained RedactText (which strips known sensitive keys and
// token patterns for log output), this is an all-or-nothing guard for
// client-facing error messages: if the message would leak an upstream URL we
// discard the whole message and return a generic one. The full original is
// expected to already be logged upstream (e.g. via response.ErrorFrom or an
// ops_error_logger), so observability is preserved while the client never sees
// the URL.
func RedactUpstreamURL(msg string) string {
	if msg == "" {
		return msg
	}
	if upstreamURLPattern.MatchString(msg) {
		return UpstreamRequestFailedMessage
	}
	return msg
}
