package scriptgen

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

type ev = map[string]interface{}

func sel(kind, text string) ev { return ev{"kind": kind, "text": text} }

// One table for what both `probe record` and Studio's recorder write for a recorded event.
func TestStep(t *testing.T) {
	for _, c := range []struct {
		name   string
		params ev
		want   string
	}{
		{"tap by text", ev{"action": "tap", "selector": sel("text", "Sign In")}, `  tap "Sign In"`},
		{"tap by key", ev{"action": "tap", "selector": sel("id", "loginButton")}, `  tap #loginButton`},
		{"key already has a hash", ev{"action": "tap", "selector": sel("id", "#loginButton")}, `  tap #loginButton`},
		{"tap by type", ev{"action": "tap", "selector": sel("type", "Switch")}, `  tap the Switch`},
		{"tap with only text", ev{"action": "tap", "text": "OK"}, `  tap "OK"`},
		{"tap with only an id", ev{"action": "tap", "id": "ok"}, `  tap #ok`},
		{"tap without a selector", ev{"action": "tap"}, `  tap the element`},
		{"tap with a quote", ev{"action": "tap", "selector": sel("text", `Say "hi"`)}, `  tap "Say \"hi\""`},
		{"type into a field", ev{"action": "type", "text": "a@b.co", "selector": sel("id", "email")}, `  type "a@b.co" into the #email field`},
		{"type into a field by text", ev{"action": "type", "text": "x", "selector": sel("text", "Email")}, `  type "x" into the "Email" field`},
		{"type with no target", ev{"action": "type", "text": "hi"}, `  type "hi"`},
		{"type with a flat id", ev{"action": "type", "text": "hi", "id": "name"}, `  type "hi" into the #name field`},
		{"swipe", ev{"action": "swipe", "direction": "left"}, `  swipe left`},
		{"scroll", ev{"action": "scroll", "direction": "down"}, `  scroll down`},
		{"long press", ev{"action": "long_press", "selector": sel("text", "Item")}, `  long press on "Item"`},
		{"navigate", ev{"action": "navigate"}, `  go back`},
		{"see", ev{"action": "see", "selector": sel("text", "Done")}, `  see "Done"`},
		{"unknown action becomes a comment of its name", ev{"action": "pinch", "text": "x", "selector": sel("text", "Y")}, `  # pinch`},
		{"no action is skipped", ev{}, ``},
		{"an action that is not text is skipped", ev{"action": 7}, ``},
		// a selector without text is not a selector: the old CLI panicked here, and both wrote "#" for an id with no text
		{"a selector without a text key", ev{"action": "tap", "selector": ev{"kind": "id"}}, `  tap the element`},
		{"an id selector with empty text", ev{"action": "tap", "selector": sel("id", "")}, `  tap the element`},
		{"an empty selector falls back to flat text", ev{"action": "tap", "selector": sel("text", ""), "id": "ok"}, `  tap #ok`},
		{"an unknown selector kind falls back", ev{"action": "tap", "selector": sel("weird", "x"), "text": "Y"}, `  tap "Y"`},
		{"selector is not a map", ev{"action": "tap", "selector": "x"}, `  tap the element`},
	} {
		if got := Step(c.params); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Text from the screen can hold line breaks; a step stays on one line.
func TestTextStaysOnOneLine(t *testing.T) {
	for name, p := range map[string]ev{
		"selector text": {"action": "tap", "selector": sel("text", "  Line one\r\nline two \n")},
		"flat text":     {"action": "tap", "text": "a\nb"},
		"flat id":       {"action": "tap", "id": "a\nb"},
		"direction":     {"action": "swipe", "direction": "le\nft"},
		"unknown":       {"action": "pi\nnch"},
		"type":          {"action": "type", "text": "x", "selector": sel("text", "a\nb")},
	} {
		if got := Step(p); strings.ContainsAny(got, "\r\n") || got == "" {
			t.Errorf("%s: %q", name, got)
		}
	}
	if got := Step(ev{"action": "tap", "selector": sel("text", "  Line one\r\nline two \n")}); got != `  tap "Line one line two"` {
		t.Errorf("got %q", got)
	}
}

// The value of an obscured field is never written: not as a step, not in the comment, not as a selector, whatever else the event carries.
func TestObscuredFieldsNeverWriteTheirValue(t *testing.T) {
	const secret = "quorvex-secret-123"
	for name, p := range map[string]ev{
		"with a selector":                   {"action": "type", "text": "", "obscured": true, "selector": sel("text", "Password")},
		"the value sent with the flag":      {"action": "type", "text": secret, "obscured": true, "selector": sel("text", "Password")},
		"the value and no target at all":    {"action": "type", "text": secret, "obscured": true},
		"the value and only a flat id":      {"action": "type", "text": secret, "obscured": true, "id": "pw"},
		"the value in a selector-less type": {"action": "type", "text": secret, "obscured": true, "selector": ev{"kind": "id"}},
	} {
		got := Step(p)
		if strings.Contains(got, secret) || strings.Contains(got, `type "`) && !strings.Contains(got, "was not recorded") {
			t.Errorf("%s: %q", name, got)
		}
		if !strings.HasPrefix(got, "  # type into ") || !strings.Contains(got, "was not recorded") || !strings.Contains(got, `Add type "${VAR}" into`) {
			t.Errorf("%s: not the comment: %q", name, got)
		}
	}
	// a plain field still types
	if got := Step(ev{"action": "type", "text": "hello", "selector": sel("text", "Password")}); got != `  type "hello" into the "Password" field` {
		t.Errorf("a plain field: %q", got)
	}
}

// An unknown action's comment names the action and nothing else of the event.
func TestAnUnknownActionEchoesNoValues(t *testing.T) {
	got := Step(ev{"action": "drag", "text": "SECRET-1", "id": "SECRET-2", "direction": "SECRET-3", "selector": sel("text", "SECRET-4"), "obscured": true})
	if got != "  # drag" {
		t.Errorf("got %q", got)
	}
	if got := Step(ev{"action": "ev\nil\r# x"}); got != "  # ev il # x" || strings.ContainsAny(got, "\r\n") {
		t.Errorf("a sanitized name: %q", got)
	}
}

// The gap rule, once: more than two seconds gets a wait of whole seconds; two seconds or less, and a negative gap, none. The CLI (event
// times) and Studio (time between notifications) wrote the same rule before; both now call this.
func TestGapStep(t *testing.T) {
	for _, c := range []struct {
		gap  time.Duration
		want string
	}{
		{0, ""}, {100 * time.Millisecond, ""}, {2 * time.Second, ""}, {2*time.Second + time.Nanosecond, "  wait 2 seconds"},
		{2900 * time.Millisecond, "  wait 2 seconds"}, {3 * time.Second, "  wait 3 seconds"}, {61 * time.Second, "  wait 61 seconds"},
		{-5 * time.Second, ""}, {time.Duration(1<<63 - 1), "  wait 9223372036 seconds"},
	} {
		if got := GapStep(c.gap); got != c.want {
			t.Errorf("gap %v: got %q, want %q", c.gap, got, c.want)
		}
	}
}

// Whatever the event, a step is one line, and an obscured field's value is never in it. Fixed seed.
func TestRandomEventsNeverBreakTheRules(t *testing.T) {
	r := rand.New(rand.NewSource(20261116))
	words := []string{"", "Sign In", "  spaced  ", "two\nlines", "a\r\nb", `q"uote`, `back\slash`, "#hash", "ünï"}
	pick := func(xs ...string) string { return xs[r.Intn(len(xs))] }
	for i := 0; i < 5000; i++ {
		p := ev{}
		if r.Intn(10) != 0 {
			p["action"] = pick("tap", "type", "swipe", "scroll", "long_press", "navigate", "see", "pinch", "", "x\ny")
		}
		if r.Intn(2) == 0 {
			s := ev{"kind": pick("id", "text", "type", "other", "")}
			if r.Intn(5) != 0 {
				s["text"] = words[r.Intn(len(words))]
			}
			p["selector"] = s
		}
		if r.Intn(2) == 0 {
			p["text"] = words[r.Intn(len(words))]
		}
		if r.Intn(3) == 0 {
			p["id"] = words[r.Intn(len(words))]
		}
		if r.Intn(2) == 0 {
			p["direction"] = pick("left", "up", "", "x\ny")
		}
		obscured := r.Intn(3) == 0
		const secret = "S3CR3T-VALUE"
		if obscured {
			p["obscured"] = true
			p["text"] = secret
		}
		got := Step(p) // must not panic
		if strings.ContainsAny(got, "\r\n") {
			t.Fatalf("a step spans lines for %v: %q", p, got)
		}
		if obscured && strings.Contains(got, secret) {
			t.Fatalf("an obscured field's value was written for %v: %q", p, got)
		}
	}
}

// The recorded `type` selectors are Flutter widget class names. A name the catalog no longer has (a rename in a new Flutter) fails here with
// the name, which is the point: recorded steps like `tap the <Name>` would stop matching. It fails, with a message, when the catalog cannot be read: a test that skips would not notice.
func TestRecordedTypeNamesAreInTheWidgetCatalog(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "compat", "catalog.json"))
	if err != nil {
		t.Fatalf("docs/compat/catalog.json cannot be read (%v): this test notices a Flutter rename or removal of a widget the recorder writes steps for, so it needs the catalog", err)
	}
	var doc struct {
		Widgets []struct{ Name string } `json:"widgets"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, w := range doc.Widgets {
		have[w.Name] = true
	}
	// the type names the recorder's own tests and docs use
	for _, name := range []string{"Switch", "Checkbox", "TextField", "ElevatedButton", "Slider"} {
		if !have[name] {
			t.Errorf("%s is used as a recorded type selector (`tap the %s`) and is not in docs/compat/catalog.json: was it renamed or removed in the Flutter the catalog was made from?", name, name)
		}
	}
}

// Names the agent sends that are not screen text (an unknown action, a direction) get the engine's one-line cleaning: every kind of line break,
// control and invisible character becomes a space, and the name is cut at MaxNameRunes (the cut is marked with an ellipsis).
func TestNamesAreCleanedLikeEverywhereElseInTheEngine(t *testing.T) {
	for name, sep := range map[string]string{
		"LF": "\n", "CR": "\r", "CRLF": "\r\n", "NEL": "\u0085", "line separator": "\u2028", "paragraph separator": "\u2029", "vertical tab": "\v",
		"form feed": "\f", "NUL": "\x00", "an escape": "\x1b", "a C1 control": "\u009b", "a bidi override": "\u202e", "a zero-width space": "\u200b", "a BOM": "\ufeff",
	} {
		for _, p := range []ev{{"action": "pi" + sep + "nch"}, {"action": "swipe", "direction": "le" + sep + "ft"}, {"action": "scroll", "direction": "do" + sep + "wn"}} {
			got := Step(p)
			if strings.ContainsAny(got, "\r\n\v\f\x00\x1b\u0085\u2028\u2029\u202e\u200b\ufeff\u009b") {
				t.Errorf("%s: %q still holds a separator or a control character", name, got)
			}
			if !strings.HasPrefix(got, "  ") || strings.Count(got, "\n") != 0 {
				t.Errorf("%s: %q", name, got)
			}
		}
	}
	if got := Step(ev{"action": "pi\u2028nch"}); got != "  # pi nch" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("é", 200)
	got := Step(ev{"action": long})
	if want := "  # " + strings.Repeat("é", MaxNameRunes-1) + "…"; got != want {
		t.Errorf("a long action was not cut to %d runes: %q", MaxNameRunes, got)
	}
	// A direction is up, down, left or right (trimmed, any case); anything else is a comment that does not echo it, so a hostile word cannot add steps to the line.
	wantNote := "  # swipe was not recorded: its direction is not up, down, left or right. Add the step by hand."
	if got := Step(ev{"action": "swipe", "direction": long}); got != wantNote {
		t.Errorf("a direction that is not one: %q", got)
	}
	if got := Step(ev{"action": "swipe", "direction": "  Left  "}); got != "  swipe left" {
		t.Errorf("a direction is trimmed and lower-cased: %q", got)
	}
	for _, bad := range []string{"up steps: 1 tap \"x\"", "up\nsteps: 1", "diagonal", "", "le ft", "up up", "x", "forward", "upward"} {
		if got := Step(ev{"action": "scroll", "direction": bad}); got != "  # scroll was not recorded: its direction is not up, down, left or right. Add the step by hand." {
			t.Errorf("direction %q: %q", bad, got)
		}
	}
}

// Text that Go's %q escaped as \u00a0, \x00 or \r was read by the lexer as "u00a0", "x00" and "r": a label with a no-break space was recorded as
// `tap "Signu00a0in"` and never matched. The recorded text now survives the real parser, character for character.
func TestQuoteSurvivesTheRealParser(t *testing.T) {
	for name, text := range map[string]string{
		"a no-break space": "Sign\u00a0in", "a soft hyphen": "co\u00adop", "a zero-width space": "a\u200bb", "a bidi override": "a\u202eb", "a BOM": "\ufeffx",
		"an emoji": "Pay 💳", "a combining mark": "e\u0301", "a quote": `Say "hi"`, "a backslash": `C:\path\x`, "a backslash before n": `a\nb`, "a tab": "a\tb",
		"a trailing backslash": `end\`, "a quote after a backslash": `a\"b`, "a unicode escape lookalike": `\u00a0`, "Cyrillic and CJK": "Привет 世界",
	} {
		line := Step(ev{"action": "tap", "selector": sel("text", text)})
		prog, err := parser.ParseFile("test \"t\"\n" + line + "\n")
		if err != nil {
			t.Errorf("%s: %q does not parse: %v", name, line, err)
			continue
		}
		act, ok := prog.Tests[0].Body[0].(parser.ActionStep)
		if !ok || act.Sel == nil || act.Sel.Text != Sanitize(text) {
			t.Errorf("%s: %q parsed to %+v, want the text %q", name, line, act.Sel, Sanitize(text))
		}
		typed := Step(ev{"action": "type", "text": text, "selector": sel("id", "field")})
		prog, err = parser.ParseFile("test \"t\"\n" + typed + "\n")
		if err != nil {
			t.Errorf("%s: %q does not parse: %v", name, typed, err)
			continue
		}
		act, ok = prog.Tests[0].Body[0].(parser.ActionStep)
		if !ok || act.Text != text {
			t.Errorf("%s: %q typed %q, want %q", name, typed, act.Text, text)
		}
	}
}

// What a script line cannot hold is refused, not written wrong: a selector text with it falls back to the next lookup, and typed text becomes a
// comment that does not carry the text.
func TestWhatAScriptCannotHoldIsRefused(t *testing.T) {
	for name, text := range map[string]string{
		"NUL": "a\x00b", "an escape": "a\x1bb", "DEL": "a\x7fb", "a C1 control": "a\u009bb", "NEL": "a\u0085b", "a line separator": "a\u2028b",
		"a paragraph separator": "a\u2029b", "invalid UTF-8": "a\xffb", "a bell": "a\ab", "a vertical tab": "a\vb", "a form feed": "a\fb",
	} {
		if q, ok := Quote(text); ok {
			t.Errorf("%s: Quote accepted %q as %q", name, text, q)
		}
		typed := Step(ev{"action": "type", "text": text, "selector": sel("id", "field")})
		if !strings.HasPrefix(typed, "  # type into #field was not recorded: the text has a character a script cannot hold") || strings.Contains(typed, "a") && strings.Contains(typed, text) {
			t.Errorf("%s: typed text became %q", name, typed)
		}
		if got := Step(ev{"action": "type", "text": text}); !strings.HasPrefix(got, "  # type was not recorded: the text has a character a script cannot hold") {
			t.Errorf("%s: with no target: %q", name, got)
		}
		// a selector text that cannot be held falls back to the flat id, then to the element
		if got := Step(ev{"action": "tap", "selector": sel("text", text), "id": "ok"}); got != "  tap #ok" {
			t.Errorf("%s: %q", name, got)
		}
		if got := Step(ev{"action": "tap", "selector": sel("text", text)}); got != "  tap the element" {
			t.Errorf("%s: %q", name, got)
		}
	}
}

// The oracle: for 20,000 random texts built from the awkward characters, a text step either parses to exactly the text (selector: after Sanitize;
// typed: as it is) or is a comment/fallback that says nothing of the text. Fixed seed.
func TestQuoteOracleOverRandomTexts(t *testing.T) {
	r := rand.New(rand.NewSource(20261116))
	alphabet := []string{"a", "Z", "7", " ", "\"", "\\", "\t", "\n", "\r", "\u00a0", "\u00ad", "\u200b", "\u202e", "\ufeff", "💳", "é", "\u0301", "世", "\x00", "\x1b", "\x7f", "\u0085", "\u2028", "\u2029", "\xff", "$", "{", "#", "'"}
	held, refused := 0, 0
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		for n := r.Intn(8); n >= 0; n-- {
			b.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		text := b.String()
		// typed text
		line := Step(ev{"action": "type", "text": text, "selector": sel("id", "f")})
		if q, ok := Quote(text); ok {
			held++
			prog, err := parser.ParseFile("test \"t\"\n" + line + "\n")
			if err != nil {
				t.Fatalf("typed %q: %q does not parse: %v", text, line, err)
			}
			if act, ok := prog.Tests[0].Body[0].(parser.ActionStep); !ok || act.Text != text {
				t.Fatalf("typed %q: %q parsed to %q (quote %s)", text, line, act.Text, q)
			}
		} else {
			refused++
			if !strings.HasPrefix(line, "  # type into #f was not recorded") {
				t.Fatalf("typed %q is not representable but the line is %q", text, line)
			}
		}
		// selector text
		clean := Sanitize(text)
		sline := Step(ev{"action": "tap", "selector": sel("text", text)})
		if q, ok := Quote(clean); ok && clean != "" {
			prog, err := parser.ParseFile("test \"t\"\n" + sline + "\n")
			if err != nil {
				t.Fatalf("selector %q: %q does not parse: %v", text, sline, err)
			}
			if act, ok := prog.Tests[0].Body[0].(parser.ActionStep); !ok || act.Sel == nil || act.Sel.Text != clean {
				t.Fatalf("selector %q: %q parsed to %+v (quote %s)", text, sline, act.Sel, q)
			}
		} else if clean == "" || !ok {
			if sline != "  tap the element" {
				t.Fatalf("selector %q cannot be held: the step is %q", text, sline)
			}
		}
		// `see` takes the same selector: its parsed text is the text too
		seeLine := Step(ev{"action": "see", "selector": sel("text", text)})
		if _, ok := Quote(clean); ok && clean != "" {
			prog, err := parser.ParseFile("test \"t\"\n" + seeLine + "\n")
			if err != nil {
				t.Fatalf("see %q: %q does not parse: %v", text, seeLine, err)
			}
			if as, ok := prog.Tests[0].Body[0].(parser.AssertStep); !ok || as.Sel.Text != clean {
				t.Fatalf("see %q: %q parsed to %+v", text, seeLine, prog.Tests[0].Body[0])
			}
		} else if seeLine != "  see the element" {
			t.Fatalf("see %q cannot be held: the step is %q", text, seeLine)
		}
	}
	if held < 5000 || refused < 2000 {
		t.Errorf("the corpus is lopsided: %d held, %d refused", held, refused)
	}
	_ = fmt.Sprint
}

// The four escapes are written as escapes, so a recorded file never holds an invisible raw tab or a split line.
func TestQuoteEscapesExactlyTheFour(t *testing.T) {
	for in, want := range map[string]string{"a\tb": `"a\tb"`, "a\nb": `"a\nb"`, `a"b`: `"a\"b"`, `a\b`: `"a\\b"`, "plain": `"plain"`, "": `""`, "a b": "\"a b\""} {
		if got, ok := Quote(in); !ok || got != want {
			t.Errorf("Quote(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

// A recorded label or typed text that has a placeholder is written as it is (no escape exists), and a comment under the step says the runner
// expands it. Comments carry no text, and text without the shapes gets no note.
func TestAPlaceholderInRecordedTextIsFlagged(t *testing.T) {
	for name, p := range map[string]ev{
		"an env placeholder in a label": {"action": "tap", "selector": sel("text", "${HOME} items")},
		"a variable in a label":         {"action": "tap", "selector": sel("text", "Hello <name>")},
		"a random variable":             {"action": "see", "selector": sel("text", "<random.email>")},
		"typed text":                    {"action": "type", "text": "user-<random.name>", "selector": sel("id", "f")},
		"a flat text":                   {"action": "long_press", "text": "${X}"},
	} {
		got := Lines(p)
		if len(got) != 2 || got[1] != PlaceholderNote || strings.HasPrefix(got[0], "  #") {
			t.Errorf("%s: %q", name, got)
		}
		if strings.Contains(got[1], "HOME") || strings.Contains(got[1], "name") {
			t.Errorf("%s: the note carries text: %q", name, got[1])
		}
	}
	for name, p := range map[string]ev{
		"plain text":           {"action": "tap", "selector": sel("text", "Sign in")},
		"a less-than sign":     {"action": "tap", "selector": sel("text", "a < b")},
		"a number in brackets": {"action": "tap", "selector": sel("text", "1<2>3")},
		"an unknown action":    {"action": "pinch"},
		"an obscured field":    {"action": "type", "obscured": true, "selector": sel("id", "pw")},
		"a swipe":              {"action": "swipe", "direction": "left"},
		"a refused typed text": {"action": "type", "text": "${X}\x00"},
	} {
		if got := Lines(p); len(got) != 1 {
			t.Errorf("%s: %q", name, got)
		}
	}
	if got := Lines(ev{}); got != nil {
		t.Errorf("no action: %q", got)
	}
}

// An id or a widget type is written raw, so one that holds a control character or a line separator is not written: the lookup falls back.
func TestIdsAndTypesThatCannotBeHeldFallBack(t *testing.T) {
	for name, text := range map[string]string{"NUL": "a\x00b", "a line separator": "a b", "an escape": "a\x1bb", "NEL": "a\u0085b", "invalid UTF-8": "a\xffb", "a lone CR": "a\rb"} {
		for _, kind := range []string{"id", "type"} {
			if got := Step(ev{"action": "tap", "selector": sel(kind, text)}); got != "  tap the element" {
				t.Errorf("%s as a %s selector: %q", name, kind, got)
			}
			if got := Step(ev{"action": "tap", "selector": sel(kind, text), "id": "ok"}); got != "  tap #ok" {
				t.Errorf("%s as a %s selector with a flat id: %q", name, kind, got)
			}
		}
		if got := Step(ev{"action": "tap", "id": text}); got != "  tap the element" {
			t.Errorf("%s as a flat id: %q", name, got)
		}
	}
	if got := Step(ev{"action": "tap", "selector": sel("type", "Switch")}); got != "  tap the Switch" {
		t.Errorf("an ordinary type: %q", got)
	}
}

// What the screen text keeps and loses: a line break becomes a space, spaces and tabs around the text are trimmed, and nothing else changes (a
// no-break space at an edge is kept); a lone carriage return is not deleted, it makes the text unholdable.
func TestSanitizeKeepsEverythingButLineBreaksAndEdgeSpaces(t *testing.T) {
	for in, want := range map[string]string{
		"  OK \t": "OK", "\n  x \r": "x", "a\nb": "a b", "a\r\nb": "a b", " OK ": " OK ", "a\u00adb": "a\u00adb", "a\rb": "a\rb", "": "",
	} {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Step(ev{"action": "tap", "selector": sel("text", " OK ")}); got != "  tap \" OK \"" {
		t.Errorf("an edge no-break space was lost: %q", got)
	}
	if got := Step(ev{"action": "tap", "selector": sel("text", "a\rb")}); got != "  tap the element" {
		t.Errorf("a lone CR was deleted, not refused: %q", got)
	}
}

// A flat text that is nothing but spaces is no lookup: never `tap ""`.
func TestBlankFlatTextIsNoLookup(t *testing.T) {
	for _, text := range []string{"", "   ", " \t ", "\n"} {
		if got := Step(ev{"action": "tap", "text": text}); got != "  tap the element" {
			t.Errorf("text %q: %q", text, got)
		}
		if got := Step(ev{"action": "tap", "text": text, "id": "ok"}); got != "  tap #ok" {
			t.Errorf("text %q with an id: %q", text, got)
		}
		if got := Step(ev{"action": "tap", "selector": sel("text", text)}); got != "  tap the element" {
			t.Errorf("selector text %q: %q", text, got)
		}
	}
}

// Whatever makes a step unrepresentable, the comment that replaces it never carries the text.
func TestARefusalNeverCarriesTheText(t *testing.T) {
	const word = "DISTINCTIVEWORD"
	for _, bad := range []string{"\x00", "\x1b", "\x7f", "\u0085", " ", " ", "\r", "\xff", "\a", "\v", "\f"} {
		for _, p := range []ev{
			{"action": "type", "text": word + bad + word},
			{"action": "type", "text": word + bad + word, "selector": sel("text", "Field")},
			{"action": "type", "text": word + bad + word, "id": "f"},
		} {
			got := Step(p)
			if !strings.HasPrefix(got, "  # type") || strings.Contains(got, "DISTINCTIVE") || strings.Contains(got, bad) {
				t.Errorf("typed %q: %q", bad, got)
			}
		}
		for _, kind := range []string{"text", "id", "type"} {
			if got := Step(ev{"action": "tap", "selector": sel(kind, word+bad+word)}); strings.Contains(got, "DISTINCTIVE") || strings.Contains(got, bad) {
				t.Errorf("a %s selector with %q: %q", kind, bad, got)
			}
		}
	}
}

// An id or a widget type is written as one word. One with a space, a quote or a backslash would end the word or open a string, and the line would
// read as more than the one step (found by FuzzStep); it falls back to the next lookup like every lookup a line cannot hold.
func TestAnIdOrTypeThatIsNotOneWordFallsBack(t *testing.T) {
	for _, bad := range []string{"has space", "has\ttab", `has"quote`, "has'quote", `back\slash`, "a\u00a0b", "x y z", "login-button", "a.b", "A!0", "1st", "##x", ""} {
		if got := Step(ev{"action": "tap", "selector": sel("id", bad)}); got != "  tap the element" {
			t.Errorf("id %q: %q", bad, got)
		}
		if got := Step(ev{"action": "tap", "id": bad}); got != "  tap the element" {
			t.Errorf("flat id %q: %q", bad, got)
		}
		if got := Step(ev{"action": "tap", "selector": sel("id", bad), "id": "ok"}); got != "  tap #ok" {
			t.Errorf("id %q with a flat id: %q", bad, got)
		}
	}
	for _, bad := range []string{`Foo" bar`, "Foo Bar", "Foo<int>", "Foo.Bar", "", "Foo\u00a0", "0", "2nd", "9Lives", "f$000", "$x", "field", "button", "the"} {
		if got := Step(ev{"action": "tap", "selector": sel("type", bad)}); got != "  tap the element" {
			t.Errorf("type %q: %q", bad, got)
		}
	}
	for _, good := range []string{"Switch", "_PrivateWidget", "Widget1", "Ünï"} {
		if got := Step(ev{"action": "tap", "selector": sel("type", good)}); got != "  tap the "+good {
			t.Errorf("type %q: %q", good, got)
		}
	}
	if got := Step(ev{"action": "tap", "selector": sel("id", "login_button2")}); got != "  tap #login_button2" {
		t.Errorf("an ordinary id: %q", got)
	}
}

// A type event whose lookup is a widget type is written without it: `type "x" into the TextField` does not read as one step, so the text is typed into the
// focused field (found by FuzzStep).
func TestATypeIntoAWidgetTypeIsTypedIntoTheFocusedField(t *testing.T) {
	got := Step(ev{"action": "type", "text": "hi", "selector": sel("type", "TextField")})
	if got != `  type "hi"` {
		t.Errorf("%q", got)
	}
	prog, err := parser.ParseFile("test \"t\"\n" + got + "\n")
	if err != nil || len(prog.Tests[0].Body) != 1 {
		t.Errorf("the line does not read back as one step: %v", err)
	}
}

// A widget type or an id that is a word of the grammar in another case still makes more than one step, so the line is read back with the real parser and
// is not written when it does not read as one step (found by FuzzStep: a type named SAve).
func TestALineThatDoesNotReadBackAsOneStepIsNotWritten(t *testing.T) {
	got := Step(ev{"action": "long_press", "selector": sel("type", "SAve")})
	if got != "  # long_press was not recorded: it could not be written as one step. Add the step by hand." {
		t.Errorf("%q", got)
	}
	if got := Step(ev{"action": "tap", "selector": sel("type", "Switch")}); got != "  tap the Switch" {
		t.Errorf("an ordinary type: %q", got)
	}
}
