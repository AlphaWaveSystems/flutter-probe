package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A provider's error body is gated: the shapeless key, an address and a token in it are gone (the error text can echo what was sent).
func TestChatErrorTextIsGated(t *testing.T) {
	const key = "zorbleflux-quantum-seven"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "bad key "+key+" for jane.doe@example.test, token "+"ghp_"+strings.Repeat("a1B2", 9))
	}))
	t.Cleanup(srv.Close)
	s := aiProviderSettings{Provider: "local", Endpoint: srv.URL + "/v1", Model: "m", APIKey: key}
	_, err := completeChat(s, "sys", []claudeMessage{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("no error")
	}
	for _, bad := range []string{"quantum", "jane", "example.test", "ghp_a1B2"} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("%q is in the error %q", bad, err.Error())
		}
	}
}

// A reply that echoes the shapeless key shows it removed, and says that text was removed.
func TestChatReplyRemovesTheKey(t *testing.T) {
	const key = "zorbleflux-quantum-seven"
	f := newLocalFake(t, "your key is "+key+" ok")
	s := aiProviderSettings{Provider: "local", Endpoint: f.srv.URL + "/v1", Model: "m", APIKey: key}
	resp, err := completeChat(s, "sys", []claudeMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resp.Content, "quantum") || !strings.Contains(resp.Content, redactedMark) {
		t.Errorf("reply: %q", resp.Content)
	}
	if !strings.Contains(resp.Notice, "removed from this reply") {
		t.Errorf("no notice that text was removed: %q", resp.Notice)
	}
	// An unchanged reply carries no such notice.
	f2 := newLocalFake(t, "Try tap \"Sign in\".")
	s.Endpoint = f2.srv.URL + "/v1"
	resp, err = completeChat(s, "sys", []claudeMessage{{Role: "user", Content: "hi"}})
	if err != nil || resp.Notice != "" {
		t.Errorf("%v %q", err, resp.Notice)
	}
}

// Routes and URLs that look like home folders are kept; real home folders are not.
func TestRoutesAndURLsAreNotHomeFolders(t *testing.T) {
	for _, in := range []string{
		"tap on '/home/settings'", "open \"/home/dashboard\"", "see https://app.test/home/settings now", "GET http://localhost:8080/home/list",
		"https://app.test/Users/list", "route /home/settings is slow", "the /root directory", "`/root`",
		"tap '/home/settings/profile'", "open \"/home/orders/42\"", "go to \"/root/start\"",
	} {
		if out := chatField(in, ""); out != in {
			t.Errorf("%q changed: %q", in, out)
		}
	}
	for in, want := range map[string]string{
		"at /Users/alice/app/x.go": "at ~/app/x.go", "at /home/alice/app/x.go": "at ~/app/x.go", `C:\Users\alice\x`: `~\x`,
		"ls /var/root/Library": "ls ~/Library", "cd /root/work": "cd ~/work", "file:///Users/alice/p": "file://~/p",
	} {
		if out := chatField(in, ""); out != want {
			t.Errorf("%q -> %q, want %q", in, out, want)
		}
	}
	if me := currentUserName(); me != "" {
		if out := chatField("opened /home/"+me, ""); out != "opened ~" {
			t.Errorf("this account's own folder: %q", out)
		}
	}
}

// A joiner that is part of a word or an emoji is kept; one that hides a token, or sits at an edge or in a run, is dropped.
func TestJoinersInWordsAreKept(t *testing.T) {
	for _, in := range []string{"\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645", "\U0001F468\u200d\U0001F469\u200d\U0001F467", "\u0915\u094d\u200d\u0937"} {
		if out := chatField("say "+in, ""); out != "say "+in {
			t.Errorf("%q changed: %q", in, out)
		}
	}
	for _, in := range []string{"ghp_a1B2a1B2\u200da1B2a1B2a1B2a1B2a1B2a1B2a1B2", "a\u200c\u200db", "word\u200d ", "\u200c word"} {
		if out := chatField(in, ""); strings.ContainsAny(out, "\u200c\u200d") {
			t.Errorf("%q kept a joiner: %q", in, out)
		}
	}
}

// A field that is cut says so, and the file's clip drops a token it cuts in two.
func TestACutIsMarkedAndTheFileClipDropsASplitToken(t *testing.T) {
	out := chatField(strings.Repeat("word ", chatFieldLimit/4), "")
	if !strings.Contains(out, "[truncated: ") || !strings.HasSuffix(out, "more characters not sent]") {
		t.Errorf("no marker: %q", out[len(out)-60:])
	}
	token := "ghp_" + strings.Repeat("a1B2", 9)
	file := strings.Repeat("x ", 20) + token
	for cut := 41; cut < 40+len(token); cut++ {
		if got := clipContext(file, cut); strings.Contains(got, "ghp_") {
			t.Errorf("clip at %d sends the start of a token: %q", cut, got)
		}
	}
	if got := clipContext("a b c d e f", 20); got != "a b c d e f" {
		t.Errorf("a text that fits changed: %q", got)
	}
}

// JSON-escaped hidden characters in an error are decoded so the drop sees them, and a very long conversation sends only the newest
// messages and says so in the preview.
func TestEscapedHiddenCharactersAndLongConversations(t *testing.T) {
	if got := decodeHiddenEscapes(`g\u200bh\u200bp_ and \u0041 and \u00e9`); got != "g\u200bh\u200bp_ and \\u0041 and \\u00e9" {
		t.Errorf("decode: %q", got)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"`+`g\u200bh\u200bp\u200b_`+strings.Repeat("a1B2", 9)+`"}`)
	}))
	t.Cleanup(srv.Close)
	_, err := completeChat(aiProviderSettings{Provider: "local", Endpoint: srv.URL + "/v1", Model: "m"}, "sys", []claudeMessage{{Role: "user", Content: "hi"}})
	if err == nil || strings.Contains(err.Error(), "ghp_") || strings.Contains(err.Error(), "a1B2a1B2") {
		t.Errorf("an escaped split token in an error: %v", err)
	}

	f := newLocalFake(t, "ok")
	dir := writeProbeYAML(t, localYAML(f.srv.URL))
	var msgs []ChatMessage
	for i := 0; i < 100; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, ChatMessage{Role: role, Content: fmt.Sprintf("message %d", i)})
	}
	msgs = append(msgs, ChatMessage{Role: "user", Content: "last"})
	preview, err := appIn(dir).PreviewAIChat(msgs, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview, "oldest messages are not sent") || strings.Contains(preview, "message 0\n") || !strings.Contains(preview, "[user] last") {
		t.Errorf("preview: %q", preview[len(preview)-300:])
	}
	if _, err := appIn(dir).Chat(msgs, ""); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Messages []struct{ Role, Content string } `json:"messages"`
	}
	if err := json.Unmarshal(f.body, &sent); err != nil {
		t.Fatal(err)
	}
	n := len(sent.Messages)
	if n > maxChatMessages+1 || sent.Messages[n-1].Content != "last" {
		t.Errorf("%d messages sent, last %q", n, sent.Messages[n-1].Content)
	}
	first := 0
	if sent.Messages[0].Role == "system" {
		first = 1
	}
	if sent.Messages[first].Role != "user" {
		t.Errorf("the first kept message is %q", sent.Messages[first].Role)
	}
}

// Full-width digits are read as digits, and the scrub is stable when a replacement puts two pieces side by side.
func TestFullWidthDigitsAndAdjacency(t *testing.T) {
	if out := chatField("card \uff14\uff11\uff11\uff11 \uff11\uff11\uff11\uff11 \uff11\uff11\uff11\uff11 \uff11\uff11\uff11\uff11", ""); strings.Contains(out, "4111") || strings.Contains(out, "\uff11") {
		t.Errorf("full-width card: %q", out)
	}
	for _, in := range []string{"a@b.co4111 1111 1111 1111", "/home//Users/x/y", "/Users//Users/x", "x@y.com/home/alice/q"} {
		once := chatField(in, "")
		if twice := chatField(once, ""); twice != once {
			t.Errorf("%q: %q then %q", in, once, twice)
		}
	}
}
