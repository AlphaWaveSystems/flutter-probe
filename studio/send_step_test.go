package main

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func stepOf(t *testing.T, s SavedStep) parser.HTTPCallStep {
	t.Helper()
	prog, err := parser.ParseFile("test \"x\"\n  " + strings.Join(s.Lines, "\n  ") + "\n")
	if err != nil {
		t.Fatalf("the saved lines do not parse: %v\n%s", err, strings.Join(s.Lines, "\n"))
	}
	for _, st := range prog.Tests[0].Body {
		if c, ok := st.(parser.HTTPCallStep); ok {
			return c
		}
	}
	t.Fatal("no call step")
	return parser.HTTPCallStep{}
}

func TestSaveStepWritesTheCallGrammarAndNothingElse(t *testing.T) {
	// No placeholders: the call line alone.
	s, err := buildCallStep(SaveStepInput{Method: "get", URL: "https://api.example.test/v1/health"})
	if err != nil || len(s.Lines) != 1 || s.Lines[0] != `call GET "https://api.example.test/v1/health"` || len(s.Fill) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
	// A body, a query, a path: query values and credential-shaped values become ${NAME}, the step says which to set, and it parses back.
	s, err = buildCallStep(SaveStepInput{Method: "POST", URL: "https://api.example.test/v1/items/abc?page=2&token=tok-valueXYZ9#frag", Body: `{"name":"Jane","password":"pw-short","n":{"apiKey":12345678,"ok":true},"list":["a","longvalue1234",null],"count":3}`})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Lines) != 2 || !strings.HasPrefix(s.Lines[0], "# set before running probe test: ") || !strings.Contains(s.Lines[0], "PROBE_SAVED_PAGE") || !strings.Contains(s.Lines[0], "PROBE_SAVED_TOKEN") || !strings.Contains(s.Lines[0], "PROBE_SAVED_PASSWORD") {
		t.Fatalf("%+v", s)
	}
	c := stepOf(t, s)
	want := `{"name":"Jane","password":"${PROBE_SAVED_PASSWORD}","n":{"apiKey":"${PROBE_SAVED_APIKEY}","ok":true},"list":["a","${PROBE_SAVED_LIST}",null],"count":3}`
	if c.Method != "POST" || c.URL != "https://api.example.test/v1/items/abc?page=${PROBE_SAVED_PAGE}&token=${PROBE_SAVED_TOKEN}" || c.Body != want {
		t.Fatalf("url %q\nbody %q", c.URL, c.Body)
	}
	if strings.Join(s.Fill, ",") != "PROBE_SAVED_PAGE,PROBE_SAVED_TOKEN,PROBE_SAVED_PASSWORD,PROBE_SAVED_APIKEY,PROBE_SAVED_LIST" {
		t.Errorf("fill %v", s.Fill)
	}
	// PUT and DELETE are the other methods with a body.
	for _, m := range []string{"PUT", "DELETE"} {
		if s, err := buildCallStep(SaveStepInput{Method: m, URL: "https://a.example.test/x", Body: `{"a":1}`}); err != nil || stepOf(t, s).Method != m {
			t.Errorf("%s: %v", m, err)
		}
	}
}

// What the call step cannot say is not saved, and the reason is a sentence with nothing typed in it.
func TestSaveStepStopsWhereTheGrammarEnds(t *testing.T) {
	typed := "tok-valueXYZ9"
	for name, in := range map[string]SaveStepInput{
		"PATCH":                       {Method: "PATCH", URL: "https://a.example.test/x", Body: "{}"},
		"HEAD":                        {Method: "HEAD", URL: "https://a.example.test/x"},
		"OPTIONS":                     {Method: "OPTIONS", URL: "https://a.example.test/x"},
		"no method":                   {URL: "https://a.example.test/x"},
		"GET with a body":             {Method: "GET", URL: "https://a.example.test/x", Body: "x"},
		"a control char":              {Method: "POST", URL: "https://a.example.test/x", Body: "a\x01b"},
		"a lone CR":                   {Method: "POST", URL: "https://a.example.test/x", Body: "a\rb"},
		"user info":                   {Method: "GET", URL: "https://user:" + typed + "@a.example.test/x"},
		"a file URL":                  {Method: "GET", URL: "file:///etc/passwd"},
		"not a URL":                   {Method: "GET", URL: "nope"},
		"a secret-looking query name": {Method: "GET", URL: "https://a.example.test/x?" + typed + "=1"},
	} {
		s, err := buildCallStep(in)
		if err == nil || len(s.Lines) != 0 {
			t.Errorf("%s: saved %+v", name, s)
			continue
		}
		if strings.Contains(err.Error(), typed) || strings.Contains(err.Error(), "passwd") {
			t.Errorf("%s: the error repeats what was typed: %v", name, err)
		}
	}
}

// The characters a step string cannot hold raw are written the way the lexer reads them, and the step says exactly the body that was typed.
func TestSaveStepKeepsTheBodyExactly(t *testing.T) {
	body := "line one \"quoted\" \\ backslash\n\tindented ünïcode 日本\r\nlast"
	s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	want := "line one \"quoted\" \\ backslash\n\tindented ünïcode 日本\nlast" // CRLF is one line break
	if got := stepOf(t, s).Body; got != want {
		t.Errorf("body %q, want %q", got, want)
	}
	if strings.Contains(s.Lines[len(s.Lines)-1], "\n") {
		t.Error("a raw line break in the step")
	}
}

// A typed credential is written nowhere in the step, whatever position it is in and however it is hidden or split.
func TestSaveStepNeverWritesATypedSecret(t *testing.T) {
	const (
		tok   = "hunter2-Xk9pq"
		other = "plainvalueqq1234"
		qv    = "urlquery4321abcd"
		seg   = "reset4567abcdef"
		mail  = "jane.doe@example.com"
	)
	soft := func(s string) string { return strings.Join(strings.Split(s, ""), "­") }
	glyph := func(s string) string { return strings.Join(strings.Split(s, ""), "") }
	cases := map[string]SaveStepInput{
		"query value":               {Method: "GET", URL: "https://a.example.test/x?q=" + qv + "&id=7"},
		"query under a credential":  {Method: "GET", URL: "https://a.example.test/x?api_key=" + tok},
		"path segment":              {Method: "GET", URL: "https://a.example.test/reset/" + seg + "/go"},
		"bare query component":      {Method: "GET", URL: "https://a.example.test/x?" + qv},
		"percent-encoded query":     {Method: "GET", URL: "https://a.example.test/x?q=weird%20value%2Fwith" + tok},
		"JSON value under a name":   {Method: "POST", URL: "https://a.example.test/x", Body: `{"password":"` + tok + `"}`},
		"JSON nested and in a list": {Method: "POST", URL: "https://a.example.test/x", Body: `{"a":{"b":[{"secret":"` + tok + `"},"` + other + `"]}}`},
		"JSON pretty printed":       {Method: "POST", URL: "https://a.example.test/x", Body: "{\n  \"token\": \"" + tok + "\",\n  \"email\": \"" + mail + "\"\n}"},
		"JSON number under a name":  {Method: "POST", URL: "https://a.example.test/x", Body: `{"pin":4821}`},
		"form":                      {Method: "POST", URL: "https://a.example.test/x", Body: "user=jane&password=" + tok + "&note=" + other},
		"text with an assignment":   {Method: "POST", URL: "https://a.example.test/x", Body: "please use password: " + tok + " now"},
		"text with a bare token":    {Method: "POST", URL: "https://a.example.test/x", Body: "token is " + other + " ok"},
		"text with an email":        {Method: "POST", URL: "https://a.example.test/x", Body: "mail " + mail + " today"},
		"split by soft hyphens":     {Method: "POST", URL: "https://a.example.test/x", Body: `{"note":"` + soft(other) + `"}`},
		"split by glyphless":        {Method: "POST", URL: "https://a.example.test/x", Body: "x " + glyph(other) + " y"},
		"split in a form":           {Method: "POST", URL: "https://a.example.test/x", Body: "password=" + soft(tok)},
		"split in a path":           {Method: "GET", URL: "https://a.example.test/" + soft(seg)},
		"upper case":                {Method: "POST", URL: "https://a.example.test/x", Body: "x " + strings.ToUpper(other) + " y"},
	}
	for name, in := range cases {
		s, err := buildCallStep(in)
		if err != nil {
			t.Logf("%s: not saved: %v", name, err)
			continue // not saving is also a way to keep it out
		}
		all := strings.ToLower(strings.Join(s.Lines, "\n"))
		for _, secret := range []string{tok, other, qv, seg, mail, "4821", "weird value", "jane.doe", "plainvalue", "hunter2"} {
			if strings.Contains(all, strings.ToLower(secret)) {
				t.Errorf("%s: %q is in the step:\n%s", name, secret, strings.Join(s.Lines, "\n"))
			}
		}
		for _, r := range all {
			if r == '­' || r == '' {
				t.Errorf("%s: a hidden character is in the step", name)
				break
			}
		}
		// And what is saved reads back as a call.
		if c := stepOf(t, s); c.Method != strings.ToUpper(in.Method) {
			t.Errorf("%s: %+v", name, c)
		}
	}
	// Some of these must actually be saved, or the loop above proves nothing.
	saved := 0
	for _, in := range cases {
		if _, err := buildCallStep(in); err == nil {
			saved++
		}
	}
	if saved != len(cases)-4 { // the ones with an invisible character are refused, which keeps them out as well
		t.Errorf("only %d of %d were saved", saved, len(cases))
	}
}

// Saving reads the typed request only: it holds no state, makes no request and does not touch the allowed hosts.
func TestSaveStepTouchesNothing(t *testing.T) {
	a := &App{}
	if _, err := a.BuildCallStep(SaveStepInput{Method: "GET", URL: "https://never-looked-up.example.test/x"}); err != nil {
		t.Fatal(err)
	}
	if len(a.send.allowed) != 0 || len(a.send.shown) != 0 || len(a.send.recent) != 0 || len(a.send.lookups) != 0 {
		t.Errorf("the send state changed: %d allowed, %d shown", len(a.send.allowed), len(a.send.shown))
	}
}

// The two checks that stand behind the code that builds the step are tested alone, because that code keeps them from ever firing.
func TestSaveStepChecksStandOnTheirOwn(t *testing.T) {
	if !leftover(`call GET "https://a.example.test/x?q=urlquery4321abcd"`, "https://a.example.test/x?q=urlquery4321abcd", "") ||
		!leftover(`call POST "https://a.example.test/x" with body "{\"password\":\"hunter2-Xk9pq\"}"`, "https://a.example.test/x", `{"password":"hunter2-Xk9pq"}`) ||
		leftover(`call GET "https://a.example.test/x?q=${Q}"`, "https://a.example.test/x?q=urlquery4321abcd", "") {
		t.Error("leftover")
	}
	for name, c := range map[string]struct{ line, method, url, body string }{
		"a method the grammar has no word for": {`call PATCH "https://a.example.test/x"`, "PATCH", "https://a.example.test/x", ""},
		"another body":                         {`call POST "https://a.example.test/x" with body "b"`, "POST", "https://a.example.test/x", "a"},
		"another address":                      {`call GET "https://a.example.test/y"`, "GET", "https://a.example.test/x", ""},
		"another method":                       {`call GET "https://a.example.test/x"`, "POST", "https://a.example.test/x", ""},
		"not a line of the grammar":            {`call "x`, "GET", "x", ""},
	} {
		if err := verifyCall(c.line, c.method, c.url, c.body); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := verifyCall(`call POST "https://a.example.test/x" with body "a\tb\n\"c\""`, "POST", "https://a.example.test/x", "a\tb\n\"c\""); err != nil {
		t.Errorf("a good line: %v", err)
	}
	// Methods the call step has no word for are refused with the reason, not by the read-back.
	for _, m := range []string{"PATCH", "HEAD", "OPTIONS"} {
		_, err := buildCallStep(SaveStepInput{Method: m, URL: "https://a.example.test/x"})
		if err == nil || !strings.Contains(err.Error(), "GET, POST, PUT and DELETE") {
			t.Errorf("%s: %v", m, err)
		}
	}
	// A short value under a credential name in free text is replaced too, and one name used twice gets two placeholders.
	s, err := buildCallStep(SaveStepInput{Method: "POST", URL: "https://a.example.test/x?a=1&a=2", Body: "the password: abc ok"})
	if err != nil {
		t.Fatal(err)
	}
	c := stepOf(t, s)
	if c.Body != "the password: ${PROBE_SAVED_PASSWORD}" || c.URL != "https://a.example.test/x?a=${PROBE_SAVED_A}&a=${PROBE_SAVED_A_2}" {
		t.Errorf("%q %q", c.URL, c.Body)
	}
}
