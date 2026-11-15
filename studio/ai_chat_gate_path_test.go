package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// splitBy puts a character between every two characters of s, so a token written with one in it is whole only after it is dropped.
func splitBy(s, with string) string {
	r := []rune(s)
	var b strings.Builder
	for i, c := range r {
		if i > 0 && i%7 == 0 {
			b.WriteString(with)
		}
		b.WriteRune(c)
	}
	return b.String()
}

// The chat takes the same path as Explain for every field: a token, an address or a home folder split by a hidden character or a
// no-break space is whole again before the gate reads it, in the request and in the preview.
func TestChatFieldsDropHiddenCharactersBeforeTheGate(t *testing.T) {
	token := "ghp_" + strings.Repeat("a1B2", 9)
	jwt := "eyJhbGciOiJIUzI1NiJ9" + "." + "eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4ifQ" + "." + "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV"
	email := "jane.doe@example.test"
	for _, with := range []string{"\u200b", "\u2060", "\u00ad", "\ufeff", "\u200d"} {
		for name, secret := range map[string]string{"token": token, "jwt": jwt, "email": email} {
			in := "see " + splitBy(secret, with) + " here"
			out := chatField(in, "")
			for _, part := range []string{token[:12], "eyJzdWIiOiIx", "jane.doe", "doe@example"} {
				if strings.Contains(out, part) {
					t.Errorf("%s split by %U: %q is still in %q", name, []rune(with)[0], part, out)
				}
			}
		}
	}
	// A no-break space is a plain space, so a spaced layout is read as one.
	if out := chatField("a\u00a0b\u3000c", ""); out != "a b c" {
		t.Errorf("spaces: %q", out)
	}
	// Line breaks are kept, a tab is a space.
	if out := chatField("one\ntwo\tthree", ""); out != "one\ntwo three" {
		t.Errorf("layout: %q", out)
	}
}

// The exact key is removed even when it is split, and even though it has no known shape.
func TestChatFieldRemovesTheKeyWhenSplit(t *testing.T) {
	const key = "zorbleflux-quantum-seven"
	for _, text := range []string{"my key is " + key, "my key is " + splitBy(key, "\u200b"), "my key is " + strings.ToUpper(key[:0]) + key + "."} {
		if out := chatField(text, key); strings.Contains(out, "quantum") || strings.Contains(out, "zorble") {
			t.Errorf("%q -> %q", text, out)
		}
	}
	// A key with a hidden character in it still matches.
	if out := chatField("k "+key, "zorble\u200bflux-quantum-seven"); strings.Contains(out, "quantum") {
		t.Errorf("a stored key with a hidden character: %q", out)
	}
}

// Nothing that fits is changed, a long field is bounded, and the path is stable: a text that went through it goes through it again
// unchanged, as the suggested fix needs (its request is built once and sent through here).
func TestChatFieldLeavesOrdinaryTextAndIsStable(t *testing.T) {
	text := "test \"login\"\n  tap \"Sign in\"\n  see \"Welcome\"\n"
	if out := chatField(text, "k-123"); out != text {
		t.Errorf("ordinary text changed: %q", out)
	}
	mixed := "mail jane.doe@example.test path /Users/jane/x token " + "ghp_" + strings.Repeat("a1B2", 9)
	once := chatField(mixed, "")
	if twice := chatField(once, ""); twice != once {
		t.Errorf("not stable: %q then %q", once, twice)
	}
	// The field is bounded before the gate reads it: the cut leaves the room that a token cut in two is dropped from.
	if out := chatField(strings.Repeat("a ", chatFieldLimit), ""); len([]rune(out)) > chatFieldLimit-explainTailCut+60 || !strings.Contains(out, "[truncated: ") {
		t.Errorf("a field of %d characters was not bounded before the gate and marked", len([]rune(out)))
	}
	// A scrub can make a field longer ("a@b.co" is six characters, "[email]" is seven), so the cap holds after the gate too.
	if out := chatField(strings.Repeat("a@b.co ", chatFieldLimit/7), ""); len([]rune(out)) > chatFieldLimit {
		t.Errorf("a scrubbed field of %d characters was not capped", len([]rune(out)))
	}
}

// End to end: a split token in the open file, the conversation and the key reaches neither the request nor the preview, and the
// preview carries what the request carries.
func TestChatRequestAndPreviewAreGatedTheSameWay(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "ok")
	dir := writeProbeYAML(t, localYAML(f.srv.URL))
	token := "ghp_" + strings.Repeat("a1B2", 9)
	msgs := []ChatMessage{{Role: "user", Content: "why? my token is " + splitBy(token, "\u200b") + " and I am jane.doe\u200b@example.test"}}
	file := "test \"x\"\n  type \"" + splitBy(token, "\u2060") + "\" into \"Field\"\n  # /Users/jane/dev/app\n"
	preview, err := appIn(dir).PreviewAIChat(msgs, file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appIn(dir).Chat(msgs, file); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(f.body, &sent); err != nil {
		t.Fatal(err)
	}
	all := string(f.body) + "\n" + preview
	for _, leak := range []string{token[:12], "a1B2a1B2", "jane.doe", "doe@"} {
		if strings.Contains(all, leak) {
			t.Errorf("%q is in the request or the preview", leak)
		}
	}
	if !strings.Contains(string(f.body), "[redacted]") || !strings.Contains(string(f.body), "[email]") {
		t.Errorf("the request shows no marker:\n%s", f.body)
	}
	for _, m := range sent.Messages {
		if !strings.Contains(preview, m.Content) {
			t.Errorf("the preview lacks text the request sends:\n%q", m.Content)
		}
	}
}

// A card number split by hidden characters or no-break spaces is whole again before the gate reads it.
func TestChatFieldRedactsASplitCard(t *testing.T) {
	for _, in := range []string{"card 4111\u200b1111\u200b1111\u200b1111 end", "card 4111\u00a01111\u00a01111\u00a01111 end", "card 41\u200d11 1111 1111 1111 end", "card 4111\u20601111-1111-1111 end"} {
		if out := chatField(in, ""); strings.Contains(out, "4111") || strings.Contains(out, "1111") {
			t.Errorf("%q -> %q", in, out)
		}
	}
}

// The reply is cleaned: an email, a home folder and a token the provider echoes back are not shown, hidden characters in it do not
// hide them, and ordinary text is shown as it is.
func TestChatReplyIsCleaned(t *testing.T) {
	isolateProvider(t, "")
	token := "ghp_" + strings.Repeat("a1B2", 9)
	f := newLocalFake(t, "Mail jane.doe@example.test, file /Users/jane/dev/app, token "+splitBy(token, "\u200b")+".\n\tUse `tap \"Sign in\"`.")
	dir := writeProbeYAML(t, localYAML(f.srv.URL))
	resp, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"jane.doe", "example.test", token[:7], "a1B2a1B2", "\u200b"} {
		if strings.Contains(resp.Content, leak) {
			t.Errorf("%q is in the reply %q", leak, resp.Content)
		}
	}
	if !strings.Contains(resp.Content, "[email]") || !strings.Contains(resp.Content, "\n") || !strings.Contains(resp.Content, "tap \"Sign in\"") {
		t.Errorf("the reply is not what was expected: %q", resp.Content)
	}
}
