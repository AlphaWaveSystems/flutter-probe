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

// errorBodyWithout is errorBody for a request that carried secret. A body that echoes the secret is cleaned before it is cut,
// so a secret that straddles the cut cannot leave a recognisable start of itself in the message.
func errorBodyWithout(b []byte, secret string) string {
	if secret != "" {
		b = []byte(strings.ReplaceAll(string(b), secret, "[redacted]"))
	}
	return errorBody(b)
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
