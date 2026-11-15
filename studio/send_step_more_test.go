package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// runSaved runs the saved call line with the runner's own executor against a fake server on this machine and returns what the server
// received: the request line, the query and the body, exactly as they came over the wire.
func runSaved(t *testing.T, s SavedStep, host string) (query, body string) {
	t.Helper()
	line := strings.ReplaceAll(s.Lines[len(s.Lines)-1], "https://api.example.test", host)
	prog, err := parser.ParseFile("test \"x\"\n  " + line + "\n")
	if err != nil {
		t.Fatalf("%v\n%s", err, line)
	}
	e := runner.NewExecutor(nil, nil, nil, 30*time.Second, false)
	if err := e.RunStep(context.Background(), prog.Tests[0].Body[0]); err != nil {
		t.Fatalf("the runner: %v", err)
	}
	return gotQuery(), gotBody()
}

var (
	seenMu    sync.Mutex
	seenQuery string
	seenBody  string
)

func gotQuery() string { seenMu.Lock(); defer seenMu.Unlock(); return seenQuery }
func gotBody() string  { seenMu.Lock(); defer seenMu.Unlock(); return seenBody }

func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seenMu.Lock()
		seenQuery, seenBody = r.URL.RawQuery, string(b)
		seenMu.Unlock()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// What the runner fills in when a step runs is never in a saved step as typed: the save stops, naming the form and nothing typed.
func TestSaveStepRefusesWhatTheRunnerWouldFill(t *testing.T) {
	for name, in := range map[string]SaveStepInput{
		"an environment name in a JSON body":  {Method: "POST", URL: "https://api.example.test/x", Body: `{"note":"${REV_DEPLOY_TOKEN}"}`},
		"an environment name in a text body":  {Method: "POST", URL: "https://api.example.test/x", Body: "hello ${HOME} there"},
		"an environment name in a query":      {Method: "GET", URL: "https://api.example.test/x?q=${REV_DEPLOY_TOKEN}"},
		"an environment name as a query name": {Method: "GET", URL: "https://api.example.test/x?${REV_DEPLOY_TOKEN}=1"},
		"an environment name in a path":       {Method: "GET", URL: "https://api.example.test/${REV_DEPLOY_TOKEN}/x"},
		"a generator":                         {Method: "POST", URL: "https://api.example.test/x", Body: `{"email":"<random.email>"}`},
		"a generator with arguments":          {Method: "POST", URL: "https://api.example.test/x", Body: "<random.number(1,5)>"},
		"the last response":                   {Method: "POST", URL: "https://api.example.test/x", Body: `{"x":"<response.body>"}`},
		"the clipboard":                       {Method: "GET", URL: "https://api.example.test/x?q=<clipboard>"},
		"a variable":                          {Method: "POST", URL: "https://api.example.test/x", Body: "<name>"},
		"a variable in a form":                {Method: "POST", URL: "https://api.example.test/x", Body: "a=<name>"},
	} {
		s, err := buildCallStep(in)
		if err == nil || len(s.Lines) != 0 {
			t.Errorf("%s: saved %+v", name, s)
			continue
		}
		if !strings.Contains(err.Error(), "fills in") || strings.Contains(err.Error(), "REV_DEPLOY") || strings.Contains(err.Error(), "HOME") || strings.Contains(err.Error(), "random") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// What is saved reaches the server as it was typed, when the runner runs it: text that only looks like a placeholder is not changed, and
// the generated placeholders are filled from the environment and from nothing else.
func TestSavedStepsRunAsTyped(t *testing.T) {
	srv := fakeServer(t)
	t.Setenv("HOME", "zz-the-real-home")
	t.Setenv("USER", "zz-the-real-user")
	t.Setenv("PATH_X", "zz-path")
	t.Setenv("REV_DEPLOY_TOKEN", "zz-the-deploy-token")
	// Typed text that is not a placeholder of the runner is written and run unchanged.
	for _, body := range []string{`{"price":"$5","n":"${1}","t":"$ {X}","lt":"a < b > c","pct":"%24%7BHOME%7D"}`, "a $HOME b $5 c < d > e", "<a href=\"x\">y</a>", "< name >", "$"} {
		s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://api.example.test/x", Body: body})
		if err != nil {
			t.Errorf("%q: %v", body, err)
			continue
		}
		if _, got := runSaved(t, s, srv.URL); got != body && !strings.HasPrefix(body, "{") {
			t.Errorf("body typed %q, the server got %q", body, got)
		} else if strings.HasPrefix(body, "{") && !strings.Contains(got, `"price":"$5"`) {
			t.Errorf("json typed %q, the server got %q", body, got)
		}
		if strings.Contains(gotBody(), "zz-the-real") {
			t.Errorf("the environment reached the body: %q", gotBody())
		}
	}
	// Generated placeholders come from their own names: USER, PATH and HOME are the machine's and are never read.
	s, err := buildCallStep(SaveStepInput{Method: "GET", URL: "https://api.example.test/x?user=bob&path=/a&home=x&HOME=y"})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range s.Fill {
		if !strings.HasPrefix(n, "PROBE_SAVED_") {
			t.Errorf("a generated name without the prefix: %s", n)
		}
	}
	q, _ := runSaved(t, s, srv.URL)
	if strings.Contains(q, "zz-the-real") || !strings.Contains(q, "user=${PROBE_SAVED_USER}") {
		t.Errorf("the query the server got: %q", q)
	}
	// A set name is filled in, and only that one.
	t.Setenv("PROBE_SAVED_USER", "filled-in")
	if q, _ = runSaved(t, s, srv.URL); !strings.Contains(q, "user=filled-in") || !strings.Contains(q, "home=${PROBE_SAVED_HOME}") {
		t.Errorf("the query after setting one name: %q", q)
	}
}

// A name made from what was typed is letters, digits and underscores behind the prefix whatever was typed, and the comment line stays one line.
func TestPlaceholderNamesAreSafe(t *testing.T) {
	valid := regexp.MustCompile(`^PROBE_SAVED_[A-Z0-9_]+$`)
	for name, base := range map[string]string{
		"a line break": "a\nb", "CRLF": "a\r\nb", "U+2028": "a b", "NEL": "a\u0085b", "a space": "a b", "a quote": `a"b`, "a brace": "a}{b",
		"a placeholder": "${HOME}", "unicode": "ünïcödé日本", "empty": "", "only symbols": "---", "300 characters": strings.Repeat("a", 300), "digits first": "1abc",
		"a hash": "a#b", "a backslash": `a\b`, "a NUL": "a\x00b",
	} {
		var ph placeholders
		got := strings.TrimSuffix(strings.TrimPrefix(ph.next(base), "${"), "}")
		if !valid.MatchString(got) || len(got) > len("PROBE_SAVED_")+40 {
			t.Errorf("%s: %q", name, got)
		}
	}
	var ph placeholders
	if a, b := ph.next("x"), ph.next("x"); a == b || b != "${PROBE_SAVED_X_2}" {
		t.Errorf("%q %q", a, b)
	}
	// Through a build: names from JSON keys and query names with line breaks in them.
	s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://api.example.test/x?a%0Ab=1&c%E2%80%A8d=2", Body: "{\"pass\\nword\":\"x\",\"to\\u2028ken\":\"y\",\"api key\":\"z\",\"\":\"w\"}"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Lines) != 2 || strings.ContainsAny(s.Lines[0], "\n\r  \u0085") || !strings.HasPrefix(s.Lines[0], "# set before running probe test: PROBE_SAVED_") {
		t.Errorf("the comment line: %q", s.Lines)
	}
	for _, n := range s.Fill {
		if !valid.MatchString(n) {
			t.Errorf("name %q", n)
		}
	}
	// The comment names exactly what the step uses.
	for _, n := range s.Fill {
		if !strings.Contains(s.Lines[1], "${"+n+"}") {
			t.Errorf("%s is named in the comment and not in the step", n)
		}
	}
}

// Emails, card numbers, credential elements, Authorization lines and phrases are replaced, in the places the reviewer found them left.
func TestSaveStepClosesTheKnownGaps(t *testing.T) {
	const card = "4111 1111 1111 1111"
	cases := map[string]struct {
		in    SaveStepInput
		gone  []string
		keeps []string
	}{
		"an email in a path":             {SaveStepInput{Method: "GET", URL: "https://a.example.test/users/jane.doe@example.com/x"}, []string{"jane.doe", "example.com"}, []string{"/users/"}},
		"an email in a form value":       {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: "contact=jane.doe@example.com&n=1"}, []string{"jane.doe"}, []string{"n=1"}},
		"an email as a JSON key":         {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"jane.doe@example.com":true,"ok":1}`}, []string{"jane.doe"}, []string{`"ok":1`}},
		"a card in a JSON string":        {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"note":"pay with ` + card + ` now"}`}, []string{"4111"}, nil},
		"a card as a digits-only string": {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"n":"4111111111111111"}`}, []string{"4111"}, nil},
		"a card as a JSON number":        {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"n":4111111111111111}`}, []string{"4111"}, nil},
		"a card in a form":               {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: "x=4111-1111-1111-1111"}, []string{"4111"}, nil},
		"a card in text":                 {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: "my card is " + card + " ok"}, []string{"4111"}, []string{"my card is"}},
		"a card in a path":               {SaveStepInput{Method: "GET", URL: "https://a.example.test/cards/4111111111111111"}, []string{"4111"}, []string{"/cards/"}},
		"a number that is no card":       {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"n":1234567890123}`}, nil, []string{"1234567890123"}},
		"an XML element":                 {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `<login v="1"><user id="2">jane</user><password type="plain">my secret phrase</password></login>`}, []string{"my secret", "phrase"}, []string{`<user id="2">jane</user>`}},
		"an XML element with an attr":    {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `<password type="plain">hunter two</password>`}, []string{"hunter"}, []string{`<password type="plain">`}},
		"an Authorization line":          {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: "Authorization: Bearer abcdefgh"}, []string{"abcdefgh"}, []string{"Authorization: Bearer "}},
		"a Basic Authorization line":     {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: "authorization=Basic dXNlcjpwYXNz rest"}, []string{"dXNlcjpwYXNz"}, []string{"Basic"}},
		"a phrase after a name":          {SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: "note\npassword: my secret phrase here\nafter"}, []string{"my secret", "phrase", "here"}, []string{"after", "note"}},
	}
	for name, c := range cases {
		s, err := buildCallStep(c.in)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		step := stepOf(t, s)
		all := step.URL + "\n" + step.Body
		for _, g := range c.gone {
			if strings.Contains(all, g) {
				t.Errorf("%s: %q is still in %q", name, g, all)
			}
		}
		for _, k := range c.keeps {
			if !strings.Contains(all, k) {
				t.Errorf("%s: %q is gone from %q", name, k, all)
			}
		}
	}
}

// The checks on the finished line cannot be skipped: a line altered after it was built is stopped by each of them.
func TestTheFinalChecksAreAlwaysRun(t *testing.T) {
	defer func() { corruptLineForTest = nil }()
	in := SaveStepInput{Method: "GET", URL: "https://a.example.test/x?q=urlquery4321abcd"}
	for name, alter := range map[string]func(string) string{
		"a typed value put back":        func(l string) string { return strings.Replace(l, "${PROBE_SAVED_Q}", "urlquery4321abcd", 1) },
		"an email put back":             func(l string) string { return strings.Replace(l, "${PROBE_SAVED_Q}", "jane.doe@example.com", 1) },
		"a card number put back":        func(l string) string { return strings.Replace(l, "${PROBE_SAVED_Q}", "4111111111111111", 1) },
		"another environment name":      func(l string) string { return strings.Replace(l, "${PROBE_SAVED_Q}", "${HOME}", 1) },
		"a variable":                    func(l string) string { return strings.Replace(l, "${PROBE_SAVED_Q}", "<name>", 1) },
		"another method than was meant": func(l string) string { return strings.Replace(l, "call GET", "call POST", 1) },
		"another address":               func(l string) string { return strings.Replace(l, "a.example.test", "b.example.test", 1) },
	} {
		corruptLineForTest = alter
		if s, err := buildCallStep(in); err == nil {
			t.Errorf("%s: saved %v", name, s.Lines)
		}
	}
	corruptLineForTest = nil
	if _, err := buildCallStep(in); err != nil {
		t.Fatalf("unaltered: %v", err)
	}
	// The method box is not echoed.
	for _, m := range []string{"hunter2-Xk9pq", "x\ny", ""} {
		_, err := buildCallStep(SaveStepInput{Method: m, URL: "https://a.example.test/x"})
		if err == nil || strings.Contains(strings.ToLower(err.Error()), "hunter") || strings.Contains(err.Error(), "x\ny") {
			t.Errorf("method %q: %v", m, err)
		}
	}
}

// Each of the checks on the finished line stops what it is for, with the others given a line they accept.
func TestCheckSavedLineEachCheckAlone(t *testing.T) {
	line := func(u string) string { return `call GET "` + u + `"` }
	check := func(u, rawURL string, used ...string) error {
		ph := &placeholders{used: map[string]bool{}}
		for _, n := range used {
			ph.used[n] = true
		}
		return checkSavedLine(line(u), rawURL, "", "GET", u, "", ph)
	}
	if err := check("https://a.example.test/x?q=jane.doe@example.com", "https://a.example.test/x"); err == nil || !strings.Contains(err.Error(), "could not be hidden") {
		t.Errorf("an email left in the line: %v", err)
	}
	if err := check("https://a.example.test/x?q=4111111111111111", "https://a.example.test/x"); err == nil || !strings.Contains(err.Error(), "could not be hidden") {
		t.Errorf("a card number left in the line: %v", err)
	}
	if err := check("https://a.example.test/x?q=urlquery4321abcd", "https://a.example.test/x?q=urlquery4321abcd"); err == nil || !strings.Contains(err.Error(), "could not be hidden") {
		t.Errorf("a typed value left in the line: %v", err)
	}
	if err := check("https://a.example.test/x?q=${HOME}", "https://a.example.test/x"); err == nil || !strings.Contains(err.Error(), "not one it generated") {
		t.Errorf("an environment name that was not generated: %v", err)
	}
	if err := check("https://a.example.test/x?q=${PROBE_SAVED_Q}", "https://a.example.test/x", "PROBE_SAVED_Q"); err != nil {
		t.Errorf("a generated placeholder: %v", err)
	}
	if err := check("https://a.example.test/x?q=<name>", "https://a.example.test/x"); err == nil || !strings.Contains(err.Error(), "fills in") {
		t.Errorf("a variable left in the line: %v", err)
	}
}

// A card number is found next to other digits, in any run of digits, and an object key that is a credential in a shape no ordinary key
// has is refused (found by the fuzz test: "0 4111111111111111 0" and {"<hex>":true} were saved as typed).
func TestSaveStepFindsCardsNextToDigitsAndRefusesCredentialKeys(t *testing.T) {
	for _, body := range []string{"0 4111111111111111 0", "12 4111 1111 1111 1111 345", "x4111111111111111y", "99-4111-1111-1111-1111-99", "1234567890123 4111111111111111"} {
		s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body})
		if err != nil {
			t.Errorf("%q: %v", body, err)
			continue
		}
		if got := stepOf(t, s).Body; strings.Contains(strings.ReplaceAll(strings.ReplaceAll(got, " ", ""), "-", ""), "4111111111111111") {
			t.Errorf("%q: the card is in %q", body, got)
		}
	}
	// Digits that are not a card are kept.
	for _, body := range []string{"1234567890123", "0 1234567890123 0", "1111 1111 1111 1112"} {
		s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body})
		if err != nil || stepOf(t, s).Body != body {
			t.Errorf("%q: %v %+v", body, err, s)
		}
	}
	if got := len(cardRuns("a 4111111111111111 b 5500000000000004 c")); got != 2 {
		t.Errorf("two cards in one text: %d runs", got)
	}
	for _, key := range []string{"9f86d081884c7d659a2feaa0c55ad015", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxw", "dGVzdF9zZWNyZXRfMTIzNDU2Nzg5MA=="} {
		_, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + key + `":true}`})
		if err == nil || strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "object key") {
			t.Errorf("a key %q: %v", key[:12], err)
		}
	}
	// Ordinary keys are kept, however many letters and digits they have.
	for _, key := range []string{"address1_line", "shippingAddressLine1", "field_2024", "user-id-12345678", "ABCDEF0123456789"[:15], "a1b2c3d4"} {
		s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + key + `":1}`})
		if err != nil || stepOf(t, s).Body != `{"`+key+`":1}` {
			t.Errorf("the key %q: %v %+v", key, err, s.Lines)
		}
	}
}
