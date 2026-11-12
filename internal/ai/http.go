package ai

import (
	"net/http"
	"strings"
	"sync/atomic"
)

// noRedirects stops a provider client at the first answer. A redirect would carry the request, its prompt and any key to
// another host, so an endpoint that redirects is an error.
func noRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// errorTextFilter is the filter SetErrorTextFilter installed, or nil.
var errorTextFilter atomic.Pointer[func(string) string]

// SetErrorTextFilter installs f as the filter that provider text passes through before it is cut, in every error message, in a
// verdict's reasoning and in the doctor report. A program that has its own redaction, such as Studio with its credential gate,
// installs it once at start, so a secret is redacted before the cut and a secret that straddles the cut leaves nothing of
// itself. f must return the text, changed or not, and must not retain it. With no filter, or with nil, the text is only cut, as
// it always was: the command line does not redact other secrets. Install it before the first request; replacing it while
// requests run is safe, but a request in flight may use either filter.
func SetErrorTextFilter(f func(string) string) {
	if f == nil {
		errorTextFilter.Store(nil)
		return
	}
	errorTextFilter.Store(&f)
}

// filterErrorText applies the installed filter, if any.
func filterErrorText(s string) string {
	if f := errorTextFilter.Load(); f != nil {
		return (*f)(s)
	}
	return s
}

// errorBodyWithout is errorBody for a request that carried secret. The exact secret is removed first, then the installed filter
// runs, and only then is the text cut, so a secret that straddles the cut cannot leave a recognisable start of itself.
func errorBodyWithout(b []byte, secret string) string {
	s := string(b)
	if secret != "" {
		s = strings.ReplaceAll(s, secret, "[redacted]")
	}
	return errorBody([]byte(filterErrorText(s)))
}

// errorString is errorBodyWithout for a string a provider returned, such as the message of an error object in a 200 answer.
func errorString(s, secret string) string { return errorBodyWithout([]byte(s), secret) }

// secretOf returns the key a client sends, so an error built from the client's output can remove it. A client that sends
// no key returns "".
func secretOf(c any) string {
	if k, ok := c.(interface{ keySecret() string }); ok {
		return k.keySecret()
	}
	return ""
}

// errorBody is the start of a provider's error body, for an error message. A body can echo what it was sent, so it is cut.
func errorBody(b []byte) string {
	const max = 200
	if len(b) > max {
		b = b[:max]
	}
	return strings.ToValidUTF8(string(b), "?")
}
