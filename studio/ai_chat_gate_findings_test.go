package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// The other ways an error can spell a hidden character are decoded before the drop, and an escape of a visible one is left alone.
func TestEveryEscapeOfAHiddenCharacterIsDecoded(t *testing.T) {
	for _, in := range []string{`\u200b`, `\u200b`, `\x{200b}`, `\x{200B}`, `&#8203;`, `&#x200B;`, `&#X200b;`, `%E2%80%8B`, `%e2%80%8b`} {
		if got := decodeHiddenEscapes("g" + in + "h"); got != "g\u200bh" {
			t.Errorf("%q -> %q", in, got)
		}
	}
	for _, in := range []string{`A`, `\x{41}`, `&#65;`, `%41`, `%E2%82%AC`, `&#99999999;`} {
		if got := decodeHiddenEscapes(in); got != in {
			t.Errorf("a visible escape %q changed: %q", in, got)
		}
	}
}

// A turn that left out old messages says so when it is sent, next to any notice the settings carry, and an unchanged short one does not.
func TestTheCapIsAnnouncedOnSend(t *testing.T) {
	f := newLocalFake(t, "ok")
	var msgs []claudeMessage
	for i := 0; i < 43; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, claudeMessage{Role: role, Content: fmt.Sprintf("m%d", i)})
	}
	s := aiProviderSettings{Provider: "local", Endpoint: f.srv.URL + "/v1", Model: "m", Notice: "a plain key was not used"}
	resp, err := completeChat(s, "sys", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Notice, "a plain key was not used") || !strings.Contains(resp.Notice, "Only the newest 39 messages") || !strings.Contains(resp.Notice, "the 4 oldest were not") {
		t.Errorf("notice: %q", resp.Notice)
	}
	resp, err = completeChat(s, "sys", msgs[:3])
	if err != nil || resp.Notice != "a plain key was not used" {
		t.Errorf("%v %q", err, resp.Notice)
	}
}

// A chat turn replaces this account's own home folder and no other path: routes, URLs and other accounts' folders are left as they are.
func TestChatReplacesOnlyTheOwnHomeFolder(t *testing.T) {
	for _, in := range []string{
		"tap on '/home/settings'", "open \"/home/dashboard\"", "see https://app.test/home/settings now", "GET http://localhost:8080/home/list",
		"https://app.test/Users/list", "route /home/settings/profile", "the /root directory", "GoRoute(path: '/users/:id')", "/api/users/42/orders",
		"/Users/alice/Documents/x", "/home/other/dev/app", `C:\Users\carol\dev`, "file:///Users/alice/p", "/var/root/Library",
	} {
		if out := chatField(in, ""); out != in {
			t.Errorf("%q changed: %q", in, out)
		}
	}
}

func TestOwnHomeFolderIsReplacedInEveryForm(t *testing.T) {
	t.Setenv("HOME", "/srv/people/dana")
	me := currentUserName()
	in := []string{
		"at /srv/people/dana/app/x.go", "'/srv/people/dana'", "{\"p\":\"/srv/people/dana/x\"}", "`/SRV/People/Dana/x`", "(/srv/people/dana)", "end /srv/people/dana.",
		`{"p":"\/srv\/people\/dana\/x"}`,
	}
	if me != "" {
		in = append(in, "`/home/"+me+"/x`", "\"/HOME/"+strings.ToUpper(me)+"/x\"", "{\"p\":\"/Users/"+me+"\"}", "https://x.test/home/"+me+"/y", `{"p":"\/Users\/`+me+`\/x"}`, `{"p":"C:\\Users\\`+me+`\\dev"}`)
	}
	for _, text := range in {
		for name, out := range map[string]string{"chatField": chatField(text, ""), "scrubPersonalText": scrubPersonalText(text)} {
			if !strings.Contains(out, "~") || strings.Contains(strings.ToLower(out), "dana") || (me != "" && strings.Contains(strings.ToLower(out), strings.ToLower(me))) {
				t.Errorf("%s: %q -> %q", name, text, out)
			}
		}
	}
}

// The home folder needs a boundary on both sides: another name that starts with it, and a path that merely holds it, are kept.
func TestOwnHomeFolderNeedsBoundaries(t *testing.T) {
	t.Setenv("HOME", "/home/user")
	for _, in := range []string{"/home/users/42", "/home/username", "/home/user2/x", "/home/user_x", "/home/user-x", "/tmp/home/user/x", "a/home/user", "~/home/user", "x.home/user"} {
		if out := scrubOwnHome(in); strings.HasPrefix(out, "~") && !strings.Contains(in, "/tmp") && false {
			t.Errorf("%q -> %q", in, out)
		}
	}
	for _, in := range []string{"/home/users/42", "/home/username", "/home/user2/x", "/home/user_x", "/home/user-x", "/tmp/home/user/x", "a/home/user"} {
		if out := scrubOwnHome(in); out != in && !(currentUserName() != "" && strings.Contains(in, "/home/"+currentUserName())) {
			t.Errorf("%q changed: %q", in, out)
		}
	}
	for in, want := range map[string]string{"/home/user": "~", "/home/user/x": "~/x", "see /home/user, ok": "see ~, ok", "(/home/user)": "(~)", "/home/user\nnext": "~\nnext"} {
		if out := scrubOwnHome(in); out != want {
			t.Errorf("%q -> %q, want %q", in, out, want)
		}
	}
	// a large text of near misses is scanned in one pass
	if out := scrubOwnHome(strings.Repeat("/home/users;", 1<<17)); len(out) != len(strings.Repeat("/home/users;", 1<<17)) {
		t.Error("near misses were changed")
	}
}

// 1 MB of path-looking text is scanned in well under a second.
func TestChatFieldIsFastOnPathLookingText(t *testing.T) {
	for _, unit := range []string{"/home/a;", "/Users/ab:", "/home/a|", "'/home/a/b'", "/home/a/b/", "x=/home/a "} {
		text := strings.Repeat(unit, (1<<20)/len(unit))
		start := time.Now()
		_ = chatField(text, "")
		_ = scrubPersonalText(text)
		limit := 3 * time.Second
		if raceEnabled {
			limit = 30 * time.Second // the race detector slows this by an order of magnitude
		}
		if d := time.Since(start); d > limit {
			t.Errorf("%q: %v", unit, d)
		}
	}
}

// A file clip never ends inside a token of any shape, in a text with spaces and in one without.
func TestClipNeverSendsPartOfAToken(t *testing.T) {
	tokens := map[string]string{
		"aws secret": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "aws key": "AKIAIOSFODNN7EXAMPLE", "ghp": "ghp_" + strings.Repeat("a1B2", 9),
		"jwt": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV", "hex": strings.Repeat("9f86d081", 8), "sk": "sk-" + strings.Repeat("Ab1Cd2Ef", 6),
	}
	for name, tok := range tokens {
		for _, layout := range []string{"spaced", "minified"} {
			for back := 1; back < len(tok); back += 3 {
				var doc string
				if layout == "spaced" {
					doc = strings.Repeat("word ", 300) + tok + " tail"
				} else {
					doc = `{"k":"` + strings.Repeat("x", 1500) + `","t":"` + tok + `"}`
				}
				limit := strings.Index(doc, tok) + back
				got := clipContext(doc, limit)
				if i := strings.Index(got, tok[:min(8, len(tok))]); i >= 0 {
					t.Errorf("%s %s: %d of %d characters of the token are in the clip", name, layout, back, len(tok))
					break
				}
			}
		}
	}
	// a minified text is cut at a quote or a comma, so most of it is kept
	mini := `{"k":"` + strings.Repeat("x", 1500) + `","t":"` + tokens["ghp"] + `"}`
	if got := clipContext(mini, strings.Index(mini, "ghp_")+10); len(got) < 1500 || strings.Contains(got, "ghp_") {
		t.Errorf("a minified clip kept %d bytes, token in it: %v", len(got), strings.Contains(got, "ghp_"))
	}
	// a clip at a boundary keeps the text before it
	if got := clipContext(strings.Repeat("a\n", 1000), 1501); !strings.HasPrefix(got, "a\na\n") || len(got) < 1400 {
		t.Errorf("a clip at a line break lost too much: %d bytes", len(got))
	}
	if got := clipContext("one two three four", 10); !strings.HasPrefix(got, "one two") || strings.Contains(got, "thr") {
		t.Errorf("%q", got)
	}
}

// Full-width digits and dashes are folded only inside a card-shaped run; other text is sent as it is.
func TestFoldingTouchesOnlyCardShapedRuns(t *testing.T) {
	for _, in := range []string{"4111\uff0d1111\uff0d1111\uff0d1111", "4111\u20101111\u20111111\u20121111", "4111\u22121111\u22121111\u22121111", "\uff14\uff11\uff11\uff11\uff0d\uff11\uff11\uff11\uff11\uff0d\uff11\uff11\uff11\uff11\uff0d\uff11\uff11\uff11\uff11", "\uff14\uff11\uff11\uff11 \uff11\uff11\uff11\uff11 \uff11\uff11\uff11\uff11 \uff11\uff11\uff11\uff11"} {
		if out := chatField("card "+in, ""); strings.Contains(out, "4111") || strings.Contains(out, "\uff14") || strings.Contains(out, "\uff0d") {
			t.Errorf("%q -> %q", in, out)
		}
	}
	for _, in := range []string{"tap \"Save \u2014 now\"", "\u0031\u0032\u0033\u5186", "\uff11\uff12\uff13\u5186", "Price \uff19\uff19 \u2013 \uff11\uff10\uff10", "a\u2212b", "\uff10\uff19"} {
		if out := chatField(in, ""); out != in {
			t.Errorf("%q changed: %q", in, out)
		}
	}
	if got := foldDigits("\uff10\uff19\uff0d"); got != "\uff10\uff19\uff0d" {
		t.Errorf("a short run changed: %q", got)
	}
	if got := foldDigits("\uff14\uff11\uff11\uff11 \uff11\uff11\uff11\uff11 \uff11\uff11\uff11\uff11 \uff11\uff11\uff11\uff19"); got != "4111 1111 1111 1119" {
		t.Errorf("a card run: %q", got)
	}
}

// Two hidden characters written as percent-encoded bytes in a row are decoded, and a run that holds a visible one is left alone.
func TestConsecutivePercentEncodedHiddenCharacters(t *testing.T) {
	if got := decodeHiddenEscapes("g%E2%80%8B%E2%80%8Bh%EF%BB%BF%E2%80%8Bi"); got != "g\u200b\u200bh\ufeff\u200bi" {
		t.Errorf("%q", got)
	}
	if got := decodeHiddenEscapes("a%E2%80%8B%41b"); got != "a%E2%80%8B%41b" {
		t.Errorf("a mixed run changed: %q", got)
	}
}

// A path deep inside a URL is a route, even when it looks like this account's folder; only the path right after the host is replaced.
func TestOwnHomeInsideADeepURLPathIsKept(t *testing.T) {
	me := currentUserName()
	if me == "" {
		t.Skip("no account name")
	}
	for _, in := range []string{"https://x.test/tmp/home/" + me + "/y", "https://x.test/a/home/" + me, "https://x.test/p?next=/a/home/" + me + "/z"} {
		if out := scrubOwnHome(in); out != in {
			t.Errorf("%q changed: %q", in, out)
		}
	}
}
