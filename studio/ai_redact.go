package main

import (
	"regexp"
	"strings"
)

// redactedMark replaces what must not leave the machine.
const redactedMark = "[redacted]"

// Credential-looking text: an API key, a GitHub token, an Authorization header value, or a long run of hex or base64.
// The patterns are applied to every piece of screen or run text the AI chat sends.
var (
	apiKeyPattern      = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`)
	githubTokenPattern = regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}`)
	bearerPattern      = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`)
	longHexPattern     = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)
	longBase64Pattern  = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
)

// redactCredentials replaces credential-looking text in s with redactedMark. A long base64 run is redacted only when it
// holds a digit or a base64 symbol, so a long identifier made of letters alone is kept.
func redactCredentials(s string) string {
	for _, re := range []*regexp.Regexp{apiKeyPattern, githubTokenPattern, bearerPattern, longHexPattern} {
		s = re.ReplaceAllString(s, redactedMark)
	}
	return longBase64Pattern.ReplaceAllStringFunc(s, func(run string) string {
		if strings.ContainsAny(run, "0123456789+/=") {
			return redactedMark
		}
		return run
	})
}

// A text input as the widget dump names it. The capture groups keep the indentation and the widget type.
var editableWidgetLine = regexp.MustCompile(`^(\s*)(EditableText|TextField|TextFormField|CupertinoTextField)\b`)

// obscuredMarker is what an agent writes on a dump line for a field whose text is hidden. The current agent writes no
// such marker, so the secret-field rule below is what applies today.
var obscuredMarker = regexp.MustCompile(`(?i)\bobscure(d|s|text)?\b`)

// secretFieldName matches a key that names a secret field (password, PIN, token and similar) as a whole word or word part.
var secretFieldName = regexp.MustCompile(`(?i)(^|[^a-z])(pass(word|code|wd)?|pin|secret|token|otp|cvv|api[_-]?key|credential)([^a-z]|$)`)

// redactTree prepares the device's widget tree for sending. An editable field that is obscured, or whose key names a
// secret, loses its key and is marked as obscured, and credential-looking text is redacted on every line. The dump holds
// widget types and keys, not the text typed into fields, so this never adds a value that was not there.
func redactTree(tree string) string {
	lines := strings.Split(tree, "\n")
	for i, line := range lines {
		if m := editableWidgetLine.FindStringSubmatch(line); m != nil && (obscuredMarker.MatchString(line) || secretFieldName.MatchString(line)) {
			line = m[1] + m[2] + "(key=[redacted], obscured)"
		}
		lines[i] = redactCredentials(line)
	}
	return strings.Join(lines, "\n")
}
