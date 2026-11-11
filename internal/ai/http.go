package ai

import (
	"net/http"
	"strings"
)

// noRedirects stops a provider client at the first answer. A redirect would carry the request, its prompt and any key to
// another host, so an endpoint that redirects is an error.
func noRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// errorBody is the start of a provider's error body, for an error message. A body can echo what it was sent, so it is cut.
func errorBody(b []byte) string {
	const max = 200
	if len(b) > max {
		b = b[:max]
	}
	return strings.ToValidUTF8(string(b), "?")
}
