package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"runtime"
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

// The card search has no size limit and is linear: a card behind any number of digit groups is found (a cap that skipped long runs hid
// one behind 2,000 groups), and a megabyte of digit groups is searched in well under a second.
func TestCardSearchHasNoSizeLimit(t *testing.T) {
	const card = "4111111111111111"
	for _, n := range []int{10, 1999, 2000, 2001, 3000, 100000} {
		for _, group := range []string{"0 ", "000 ", "123 "} {
			body := strings.Repeat(group, n) + card
			s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body})
			if err != nil {
				continue // a refusal keeps it out as well
			}
			if got := stepOf(t, s).Body; strings.Contains(strings.ReplaceAll(got, " ", ""), card) || strings.HasSuffix(got, card) {
				t.Fatalf("%d groups of %q before a card: the card is in the step", n, group)
			}
		}
	}
	// A megabyte of digit groups is read in linear work. The check counts allocations, which do not depend on how busy the machine is (a
	// wall-clock limit failed under load): doubling the input at most doubles allocations and bytes, and a megabyte of one digit run allocates a handful.
	assertLinear(t, "digit groups", func(n int) func() { in := strings.Repeat("123 ", n); return func() { _ = cardRuns(in) } }, 1<<16)
	one := strings.Repeat("7", 1<<20)
	if got := measure(func() { _ = cardRuns(one) }); got.allocs > 64 { // a handful; the count moves with the runtime and with -race, the doubling ratio above is the real check
		t.Errorf("a megabyte of one digit run allocated %v times", got.allocs)
	}
}

// work is what one measurement of a piece of code costs: how many allocations, how many bytes, and how long it took.
type work struct {
	allocs float64
	bytes  float64
	took   time.Duration
}

// measure runs f once to warm it up (compiled patterns, caches) and once more under the counters of the runtime.
func measure(f func()) work {
	f()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	f()
	took := time.Since(start)
	runtime.ReadMemStats(&after)
	return work{allocs: float64(after.Mallocs - before.Mallocs), bytes: float64(after.TotalAlloc - before.TotalAlloc), took: took}
}

// looseBackstop is a wall-clock limit that only work that has gone wrong reaches (the linear work takes well under a second): it catches
// slowness that allocates nothing, which counting cannot see, and is too far from the real time to fail on a busy machine.
const looseBackstop = 30 * time.Second

// assertLinear runs work(n) and work(2n) and fails when the larger one allocates more than 2.5 times as many bytes or times as often as the
// smaller one, or takes longer than looseBackstop. Counting allocations does not depend on how busy the machine is (a tight wall-clock limit
// failed under load), but a count alone misses work that copies a growing slice on every step (one allocation per step, quadratic bytes), so the
// bytes are compared as well; the doubling ratio, not a fixed number, so a different input size or runtime does not move it.
func assertLinear(t *testing.T, what string, mk func(n int) func(), n int) {
	t.Helper()
	for _, msg := range linearFailures(what, mk, n) {
		t.Error(msg)
	}
}

// linearFailures is the check of assertLinear as a list of what is wrong, so a test can show that it fails.
func linearFailures(what string, mk func(n int) func(), n int) []string {
	var bad []string
	small, large := measure(mk(n)), measure(mk(2*n))
	if small.allocs < 1 || large.allocs > 2.5*small.allocs {
		bad = append(bad, fmt.Sprintf("%s: %v allocations for %d, %v for %d: not linear", what, small.allocs, n, large.allocs, 2*n))
	}
	if small.bytes < 1 || large.bytes > 2.5*small.bytes {
		bad = append(bad, fmt.Sprintf("%s: %.0f bytes allocated for %d, %.0f for %d: not linear", what, small.bytes, n, large.bytes, 2*n))
	}
	if large.took > looseBackstop {
		bad = append(bad, fmt.Sprintf("%s: %v for %d: far slower than any linear work", what, large.took, 2*n))
	}
	return bad
}

// More values to replace than a step may name stops the save, and the placeholders are handed out in linear time.
func TestTooManyValuesStopTheSave(t *testing.T) {
	// 300 separate cards (a letter between them, so each is its own run) are more than a step may name.
	body := strings.Repeat("4111111111111111 x ", 300)
	if s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body}); err == nil {
		t.Errorf("300 cards were saved: %d lines", len(s.Lines))
	} else if !strings.Contains(err.Error(), "more than 200 values") {
		t.Errorf("%v", err)
	}
	// One run of 100,000 groups is one value, and fast.
	if s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: strings.Repeat("000 ", 100000) + "4111111111111111"}); err != nil || len(s.Fill) != 1 {
		t.Errorf("one run of digit groups: %v %v", err, s.Fill)
	}
	var ph placeholders
	for i := 0; i < maxPlaceholders; i++ {
		ph.next("same")
	}
	if ph.overflow || ph.order[0] != "PROBE_SAVED_SAME" || ph.order[1] != "PROBE_SAVED_SAME_2" || ph.order[maxPlaceholders-1] != "PROBE_SAVED_SAME_200" {
		t.Errorf("names: %v %v %v overflow %v", ph.order[0], ph.order[1], ph.order[maxPlaceholders-1], ph.overflow)
	}
	if got := ph.next("same"); !ph.overflow || got != "${PROBE_SAVED_MORE}" {
		t.Errorf("the 201st: %q overflow %v", got, ph.overflow)
	}
	// 199 values are fine.
	var parts []string
	for i := 0; i < 199; i++ {
		parts = append(parts, fmt.Sprintf("k%d=v%d", i, i))
	}
	if _, err := buildCallStep(SaveStepInput{Method: "GET", URL: "https://a.example.test/x?" + strings.Join(parts, "&")}); err != nil {
		t.Errorf("199 query values: %v", err)
	}
	// The placeholders are handed out in linear work: no wall clock, which failed under load. Counting allocations instead, a body with twice the
	// digit groups before one card allocates at most 2.5 times as much.
	assertLinear(t, "one run of digit groups", func(n int) func() {
		body := strings.Repeat("000 ", n) + "4111111111111111"
		return func() {
			_, _ = buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body})
		}
	}, 50000)
}

// A card number written with other digits around it is replaced with its whole run of digit groups, and a longer number that passes is not cut at a shorter prefix.
func TestCardsAreReplacedWholeNotInPart(t *testing.T) {
	luhnDigit := func(prefix string) string {
		for d := 0; d < 10; d++ {
			if luhn(prefix + string(rune('0'+d))) {
				return prefix + string(rune('0'+d))
			}
		}
		return ""
	}
	n19 := luhnDigit("411111111111111111") // 19 digits that pass
	grouped := n19[0:4] + " " + n19[4:8] + " " + n19[8:12] + " " + n19[12:16] + " " + n19[16:]
	for _, c := range []struct{ typed, want string }{
		{"6 4111 1111 1111 1111", "${PROBE_SAVED_CARD}"},
		{"4111 1111 1111 1111 12", "${PROBE_SAVED_CARD}"},
		{grouped, "${PROBE_SAVED_CARD}"},
		{"a " + n19 + " b", "a ${PROBE_SAVED_CARD} b"},
	} {
		s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: c.typed})
		if err != nil {
			t.Errorf("%q: %v", c.typed, err)
			continue
		}
		if got := stepOf(t, s).Body; got != c.want {
			t.Errorf("%q became %q, want %q", c.typed, got, c.want)
		}
	}
}

// Whitespace around an object key does not hide a credential key, and a name made from a key never carries what the key holds.
func TestKeysAndNamesDoNotCarryACredential(t *testing.T) {
	hex := "9f86d081884c7d659a2feaa0c55ad015"
	for _, key := range []string{" " + hex + " ", "\t" + hex, hex + "\n"} {
		body := `{"` + strings.ReplaceAll(key, "\n", `\n`) + `":1}`
		body = strings.ReplaceAll(body, "\t", `\t`)
		if s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body}); err == nil {
			t.Errorf("the key %q was saved: %v", key, s.Lines)
		}
	}
	// A key with a credential as a word is refused whole; a key with only a short credential-looking word is kept, and its name is neutral.
	if s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"Bearer ` + hex + `":"value1234abcd"}`}); err == nil {
		t.Errorf("a key holding a credential was saved: %v", s.Lines)
	}
	s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"user_id_ab12cd34ef":"value1234abcd","api_key_for_9f86d081":"other5678efgh"}`})
	if err != nil {
		t.Fatal(err)
	}
	all := strings.ToUpper(strings.Join(s.Lines, "\n"))
	if strings.Contains(strings.SplitN(all, "\n", 2)[0], "AB12CD34EF") || strings.Contains(strings.SplitN(all, "\n", 2)[0], "9F86D081") {
		t.Errorf("a credential from a key is in the comment or the names:\n%s", strings.Join(s.Lines, "\n"))
	}
	var ph placeholders
	for base, want := range map[string]string{"Bearer " + hex: "VALUE", "user_id_ab12cd34ef": "VALUE", "token": "TOKEN", "api key": "API_KEY", hex: "VALUE"} {
		if got := nameBase(base); got != want {
			t.Errorf("nameBase(%q) = %q, want %q", base, got, want)
		}
	}
	_ = ph
}

// luhnNumber returns digits of the given length that pass the card number check, from a base pattern.
func luhnNumber(length int) string {
	base := strings.Repeat("4", 1) + strings.Repeat("2", length-2)
	for d := 0; d < 10; d++ {
		if n := base + string(rune('0'+d)); luhn(n) {
			return n
		}
	}
	return ""
}

func groupBy(n string, sizes ...int) string {
	var parts []string
	for _, k := range sizes {
		parts = append(parts, n[:k])
		n = n[k:]
	}
	return strings.Join(parts, " ")
}

// A card number in any layout is gone: solid, with spaces, with hyphens, with a one- or two-digit group at either end (a layout the
// first version of the search missed whole).
func TestACardIsGoneInEveryLayout(t *testing.T) {
	layouts := []struct {
		digits int
		sizes  []int
	}{
		{13, []int{4, 4, 4, 1}}, {14, []int{4, 4, 4, 2}}, {15, []int{4, 6, 5}}, {16, []int{4, 4, 4, 4}}, {17, []int{1, 4, 4, 4, 4}},
		{17, []int{2, 4, 4, 4, 3}}, {17, []int{4, 4, 4, 4, 1}}, {18, []int{4, 4, 4, 4, 2}}, {19, []int{6, 6, 6, 1}}, {19, []int{4, 4, 4, 4, 3}},
		{13, []int{13}}, {19, []int{19}}, {16, []int{1, 1, 1, 13}}, {16, []int{13, 1, 1, 1}},
	}
	for _, l := range layouts {
		n := luhnNumber(l.digits)
		if n == "" || len(n) != l.digits {
			t.Fatalf("no %d-digit number", l.digits)
		}
		spaced := groupBy(n, l.sizes...)
		for form, typed := range map[string]string{"solid": n, "spaced": spaced, "hyphenated": strings.ReplaceAll(spaced, " ", "-")} {
			for _, body := range []string{typed, "card " + typed + " ok", `{"n":"` + typed + `"}`, "x=" + typed + "&y=1"} {
				s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body})
				if err != nil {
					continue
				}
				got := stepOf(t, s).Body
				digits := strings.Map(func(r rune) rune {
					if r >= '0' && r <= '9' {
						return r
					}
					return -1
				}, got)
				if strings.Contains(digits, n) {
					t.Errorf("%d digits %v, %s, in %q: the card is in %q", l.digits, l.sizes, form, body, got)
				}
			}
		}
	}
}

// The window is 13 to 19 digits: a number of 12 or 20 digits that passes the check is not a card, one of 13 or 19 is, and a window that
// starts inside a group is never tried.
func TestTheCardWindowIs13To19Digits(t *testing.T) {
	for _, c := range []struct {
		digits int
		card   bool
	}{{12, false}, {13, true}, {19, true}, {20, false}, {21, false}} {
		n := luhnNumber(c.digits)
		if got := len(cardRuns(n)) > 0; got != c.card {
			t.Errorf("%d digits: card %v, want %v", c.digits, got, c.card)
		}
		if got := len(cardRuns(groupBy(n, 4, c.digits-4))) > 0; got != c.card && c.digits >= 13 {
			t.Errorf("%d digits as 4+%d: card %v, want %v", c.digits, c.digits-4, got, c.card)
		}
	}
	// A 20-digit run that holds a 19-digit window at a group boundary is a card; one that holds it only inside a group is not.
	n19 := luhnNumber(19)
	if len(cardRuns("7 "+n19)) == 0 {
		t.Error("a 19-digit card after a one-digit group was not found")
	}
	if len(cardRuns("7"+n19)) != 0 && !luhn("7"+n19) {
		t.Error("a window that starts inside a group was tried")
	}
}

// The first measured rate of false replacements, for the record: stretches of short numbers written with spaces.
func TestAListOfShortNumbersIsMostlyLeftAlone(t *testing.T) {
	for _, body := range []string{"1 2 3", "12 34 56 78", "100 200 300 400", "2024 11 15", "1 22 333 4444"} { // at most 12 digits: never a card
		if len(cardRuns(body)) != 0 {
			t.Errorf("%q was taken for a card", body)
		}
	}
}

// An object key that is a card number (digits only) is replaced and not refused: a number is an id or a card, and the replacement keeps the
// object whole. One that holds a credential in a shape no ordinary key has, hex with a letter among 16 or more digits, a JWT or padded base64,
// is refused.
func TestCredentialKeysAreRefusedAndCardKeysReplaced(t *testing.T) {
	s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"4111111111111111":1,"ok":2}`})
	if err != nil || strings.Contains(stepOf(t, s).Body, "4111") || !strings.Contains(stepOf(t, s).Body, `"ok":2`) || !strings.Contains(stepOf(t, s).Body, "${PROBE_SAVED_") {
		t.Errorf("a card as a key must be replaced, not refused and not kept: %v %+v", err, s.Lines)
	}
	// A long id that is no card is kept.
	if s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"1234567890123456789":1}`}); err != nil || stepOf(t, s).Body != `{"1234567890123456789":1}` {
		t.Errorf("a numeric id as a key: %v", err)
	}
	hex := "9f86d081884c7d659a2feaa0c55ad015"
	for _, key := range []string{hex, "password=" + hex, "0x" + hex, "x_" + hex, hex + ".pem", " " + hex + " ", "token:" + hex, "a;b," + hex,
		"password=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxw", "k=dGVzdF9zZWNyZXRfMTIzNDU2Nzg5MA=="} {
		if s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + key + `":1}`}); err == nil {
			t.Errorf("the key %q was saved: %v", key, s.Lines)
		}
	}
	// What is not refused, on purpose: hex in short groups (a UUID-like id), 15 hex digits, and ordinary keys.
	for _, key := range []string{"9f86d081-884c7d65-9a2feaa0-c55ad015", "123e4567-e89b-12d3-a456-426614174000", "9f86d081884c7d6", "address1_line", "ABCDEF012345678"} {
		if s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + key + `":1}`}); err != nil || stepOf(t, s).Body != `{"`+key+`":1}` {
			t.Errorf("the key %q: %v", key, err)
		}
	}
	// The same boundary for a run inside a longer key (bounded by a character that is not a hex digit): 15 digits kept, 16 and 17 refused.
	for n, refused := range map[int]bool{15: false, 16: true, 17: true} {
		key := "x_" + strings.Repeat("b", n-1) + "2_y"
		_, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + key + `":1}`})
		if (err != nil) != refused {
			t.Errorf("a run of %d hex digits inside a key: refused %v, want %v", n, err != nil, refused)
		}
	}
	// The boundary of the hex run: 15 digits are kept, 16 and 17 are refused.
	for n, refused := range map[int]bool{15: false, 16: true, 17: true} {
		key := strings.Repeat("a", n-1) + "1"
		_, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + key + `":1}`})
		if (err != nil) != refused {
			t.Errorf("a key of %d hex digits: refused %v, want %v", n, err != nil, refused)
		}
	}
}

// A credential deep inside a long key is found: the whole key is read, and a key over maxKeyBytes is refused, never cut.
func TestSaveStepReadsTheWholeKey(t *testing.T) {
	const hex = "9f86d081884c7d659a2feaa0c55ad015"
	const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxw"
	for _, pad := range []int{0, 300, 380, 385, 400, 401, 900} {
		for _, tail := range []string{" " + hex, " " + jwt, "-" + hex} {
			key := strings.Repeat("k", pad) + tail
			_, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + key + `":1}`})
			if err == nil {
				t.Errorf("a key of %d bytes with a credential at its end was saved", len(key))
			}
		}
	}
	for _, size := range []int{1025, 5000} {
		_, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + strings.Repeat("k", size) + `":1}`})
		if err == nil || !strings.Contains(err.Error(), "too long") {
			t.Errorf("a key of %d bytes: %v", size, err)
		}
	}
	// A long key with no credential, up to the limit, is kept.
	key := strings.Repeat("k", maxKeyBytes)
	// (its value is replaced: a name over 256 characters is treated as a secret name)
	if s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + key + `":1}`}); err != nil || !strings.Contains(stepOf(t, s).Body, key) {
		t.Errorf("a plain key of %d bytes: %v", len(key), err)
	}
}

// Behaviours the reviews' mutants found: a hyphen next to hex does not hide it, and a JWT that is one word of a key is seen.
func TestSaveStepKeyShapesNextToText(t *testing.T) {
	for _, key := range []string{"ke" + "y-9f86d081884c7d659a2feaa0c55ad015", "9f86d081884c7d659a2feaa0c55ad015-key", "x eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxw", "name dGVzdF9zZWNyZXRfMTIzNDU2Nzg5MA=="} {
		if _, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: `{"` + key + `":1}`}); err == nil {
			t.Errorf("the key %q was saved", key)
		}
	}
}

// A card with other separators (a slash, an underscore, a dot, two spaces) is found, and so is one among them.
func TestSaveStepCardSeparators(t *testing.T) {
	for _, body := range []string{"4111/1111/1111/1111", "4111_1111_1111_1111", "4111.1111.1111.1111", "4111  1111  1111  1111", "x 4111-1111.1111_1111 y"} {
		s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body})
		if err != nil {
			continue
		}
		if got := stepOf(t, s).Body; strings.Contains(digitsOf(got), "4111") {
			t.Errorf("%q: the card is in %q", body, got)
		}
	}
	// Dotted numbers that are not a card are kept.
	for _, body := range []string{"10.0.0.1", "version 1.2.3", "2026/11/15"} {
		if s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body}); err != nil || stepOf(t, s).Body != body {
			t.Errorf("%q: %v", body, err)
		}
	}
}

// A card split by real slashes in the path is found: the segments are read as one text.
func TestSaveStepCardSplitAcrossPathSegments(t *testing.T) {
	for _, u := range []string{
		"https://x.example.test/a/4111/1111/1111/1111/b",
		"https://x.example.test/a/44%2060826/9770/7646/b",
		"https://x.example.test/4111/1111/1111/1111",
		"https://x.example.test/a/4111111111111111",
		"https://x.example.test/a/4111-1111-1111-1111/b",
	} {
		s, err := buildCallStep(SaveStepInput{Method: "GET", URL: u})
		if err != nil {
			continue
		}
		if got := stepOf(t, s).URL; strings.Contains(digitsOf(got), "4111") || strings.Contains(digitsOf(got), "9770") {
			t.Errorf("%q: the card is in %q", u, got)
		}
	}
	// Paths that are not a card are kept.
	for _, u := range []string{"https://x.example.test/v1/users/42/orders/7", "https://x.example.test/2026/11/15/post", "https://x.example.test/a/4111/1111/b"} {
		s, err := buildCallStep(SaveStepInput{Method: "GET", URL: u})
		if err != nil || stepOf(t, s).URL != u {
			t.Errorf("%q: %v %+v", u, err, s.Lines)
		}
	}
}

// zzSink keeps the work of the tests below on the heap: the compiler keeps an allocation that does not escape on the stack, where nothing counts it.
var zzSink [][]byte

// The linearity check fails on each kind of work that goes wrong (the mutants of the code under test, as data) and passes on work in proportion.
func TestTheLinearCheckSeesWorkThatGoesWrong(t *testing.T) {
	linear := func(n int) func() {
		return func() {
			zzSink = nil
			for i := 0; i < n; i++ {
				zzSink = append(zzSink, make([]byte, 16))
			}
		}
	}
	// more allocations than linear: a few more for each ones already made
	manyAllocs := func(n int) func() {
		return func() {
			zzSink = nil
			for i := 0; i < n; i++ {
				for j := 0; j < i/64+1; j++ {
					zzSink = append(zzSink, make([]byte, 1))
				}
			}
		}
	}
	// one allocation for each step, but each copies everything so far: the count is linear, the bytes are not
	copiesEveryStep := func(n int) func() {
		return func() {
			var acc []byte
			for i := 0; i < n; i++ {
				acc = append(append([]byte(nil), acc...), 'x')
				zzSink = [][]byte{acc}
			}
		}
	}
	// one extra pass that allocates a buffer as long as the input for each of n/64 outer steps
	extraPassPerOuter := func(n int) func() {
		return func() {
			for i := 0; i < n/64; i++ {
				zzSink = [][]byte{make([]byte, i*64+1)}
			}
		}
	}
	if msgs := linearFailures("linear", linear, 4000); len(msgs) != 0 {
		t.Errorf("linear work flagged: %v", msgs)
	}
	for name, f := range map[string]func(n int) func(){"more allocations": manyAllocs, "a copy on every step": copiesEveryStep, "an extra pass per outer step": extraPassPerOuter} {
		if msgs := linearFailures(name, f, 8000); len(msgs) == 0 {
			t.Errorf("%s: not seen", name)
		}
	}
	// the copy on every step has a linear allocation count: only the bytes see it
	if bad := linearFailures("copy", copiesEveryStep, 8000); len(bad) != 1 || !strings.Contains(bad[0], "bytes") {
		t.Errorf("a copy on every step must be caught by the bytes alone: %v", bad)
	}
}
