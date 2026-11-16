package scriptgen

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if got := Step(ev{"action": "swipe", "direction": long}); got != "  swipe "+strings.Repeat("é", MaxNameRunes-1)+"…" {
		t.Errorf("a long direction was not cut: %q", got)
	}
	if got := Step(ev{"action": "swipe", "direction": "  left  "}); got != "  swipe left" {
		t.Errorf("a direction is trimmed: %q", got)
	}
}
