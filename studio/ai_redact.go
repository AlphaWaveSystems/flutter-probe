package main

import (
	"regexp"
	"strings"
)

// redactedMark replaces what must not leave the machine.
const redactedMark = "[redacted]"

// Credential-looking text. The families are the token shapes of the common providers, an Authorization header, a PEM private
// key, an assignment whose name says it is a secret, and a secret query parameter. Every piece of text that Studio sends to
// a provider passes through redactCredentials. Emails and file paths are not redacted: they are ordinary test text.
var (
	apiKeyPattern      = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`)
	githubTokenPattern = regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}`)
	githubPATPattern   = regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}`)
	awsKeyIDPattern    = regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)
	slackTokenPattern  = regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`)
	googleKeyPattern   = regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`)
	stripeKeyPattern   = regexp.MustCompile(`\b(?:sk|rk|pk)_(?:live|test)_[0-9A-Za-z]{16,}`)
	pemKeyPattern      = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	bearerPattern      = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`)
	basicAuthPattern   = regexp.MustCompile(`(?i)\bbasic\s+([A-Za-z0-9+/]{8,}={0,2})`)
	assignedSecret     = regexp.MustCompile("(?i)(\\b[A-Za-z0-9_]*(?:secret|token|passw(?:or)?d|passwd|pwd|api[_-]?key|private[_-]?key|access[_-]?key)[A-Za-z0-9_]*\\s*[=:]\\s*)[^\\s,;&\"'`]+")
	secretQueryParam   = regexp.MustCompile(`(?i)([?&](?:access_|api_|auth_)?(?:token|key|secret|password|passwd|sig|signature)=)[^&\s"']+`)
	longHexPattern     = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)
	longBase64Pattern  = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
)

// redactCredentials replaces credential-looking text in s with redactedMark. An assignment keeps its name, and a query
// parameter keeps its name, so the text still says what was there. A long base64 run is redacted only when it holds a digit
// or a base64 symbol, so a long identifier made of letters alone is kept.
func redactCredentials(s string) string {
	s = pemKeyPattern.ReplaceAllString(s, redactedMark)
	for _, re := range []*regexp.Regexp{apiKeyPattern, githubTokenPattern, githubPATPattern, awsKeyIDPattern, slackTokenPattern,
		googleKeyPattern, stripeKeyPattern, bearerPattern, longHexPattern} {
		s = re.ReplaceAllString(s, redactedMark)
	}
	s = basicAuthPattern.ReplaceAllStringFunc(s, func(m string) string {
		run := basicAuthPattern.FindStringSubmatch(m)[1]
		if !strings.ContainsAny(run, "0123456789+/=") {
			return m
		}
		return m[:len(m)-len(run)] + redactedMark
	})
	s = secretQueryParam.ReplaceAllString(s, "${1}"+redactedMark)
	s = assignedSecret.ReplaceAllString(s, "${1}"+redactedMark)
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
