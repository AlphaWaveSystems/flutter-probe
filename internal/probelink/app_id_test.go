package probelink

import (
	"encoding/json"
	"strings"
	"testing"
)

// An agent that sends the id of its app: both clients carry it, and what was there is unchanged.
func TestHandshakeCarriesTheAppID(t *testing.T) {
	for _, wc := range wireClients {
		h := handshakeOf(t, wc, keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1), extra: map[string]any{
			"app_id": "com.example.app", "flutter_version": "3.50.0",
		}})
		if h.AppID != "com.example.app" || h.FlutterVersion != "3.50.0" || h.AgentVersion != "0.27.0" {
			t.Errorf("%s: %+v", wc.name, h)
		}
	}
}

// An agent from before the field, or an app whose id it could not read: no error, no id, everything else as before.
func TestHandshakeWithoutAnAppIDIsUnchanged(t *testing.T) {
	for _, wc := range wireClients {
		h := handshakeOf(t, wc, keyAgent{version: "0.26.0", capabilities: []string{CapabilityPressKey}, actions: make(chan json.RawMessage, 1)})
		if h.AppID != "" || h.AgentVersion != "0.26.0" || len(h.Capabilities) != 1 {
			t.Errorf("%s: %+v", wc.name, h)
		}
		// a field of the wrong type is an app that got it wrong, not a failed connect
		for _, wrong := range []any{nil, 42, true, []string{"com.example.app"}, map[string]any{"id": "com.example.app"}, ""} {
			h = handshakeOf(t, wc, keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1), extra: map[string]any{"app_id": wrong, "dart_version": "3.14.0"}})
			if h.AppID != "" || h.DartVersion != "3.14.0" {
				t.Errorf("%s: app_id %#v gave %+v", wc.name, wrong, h)
			}
		}
	}
}

// The text comes from the app, which is untrusted, and is printed in a warning: anything that is not a plain application id is dropped, never repaired, and the
// rest of the handshake still counts.
func TestHandshakeDropsAnAppIDThatIsNotPlain(t *testing.T) {
	hostile := []string{
		"com.example.app\n", "com.example.app\r\n", "\ncom.example.app", "com.example.app ", " com.example.app", "com.example. app",
		"com.example.app; rm -rf /", "com.example.app\x1b[31m", "com.example.app‮", "com.exa​mple.app", "com.example.app ",
		"com.exämple.app", "<script>alert(1)</script>", "com.example.app:remote", "com.example.app\x00", "com.example.app/../x", "com.example.app%0a",
		"single", ".com.example", "com..example", "com.example.", "-com.example", "com.-example", "com.example.app${IFS}", "com.example.app`id`",
		strings.Repeat("a", 128) + "." + strings.Repeat("b", 128), // 257 characters
	}
	for _, wc := range wireClients {
		for _, bad := range hostile {
			h := handshakeOf(t, wc, keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1), extra: map[string]any{"app_id": bad, "flutter_version": "3.50.0"}})
			if h.AppID != "" {
				t.Errorf("%s: %q was accepted as %q", wc.name, bad, h.AppID)
			}
			if h.FlutterVersion != "3.50.0" || h.AgentVersion != "0.27.0" {
				t.Errorf("%s: %q spoiled the rest of the handshake: %+v", wc.name, bad, h)
			}
		}
	}
}

func TestPlainAppID(t *testing.T) {
	for _, ok := range []string{
		"com.example.app", "a.b", "com.example.app_2", "com.my-co.app", "dev.probe.portspikea", "com.alphawavesystems.probe_fixture_b", "Com.Example.App",
		"com.example.2fa", "io.flutter.plugins.a1", strings.Repeat("a", 126) + "." + strings.Repeat("b", 128), // 255 characters
	} {
		if got := plainAppID(ok); got != ok {
			t.Errorf("plainAppID(%q) = %q, want it accepted", ok, got)
		}
	}
	if got := plainAppID(strings.Repeat("a", 126) + "." + strings.Repeat("b", 129)); got != "" { // 256 characters
		t.Errorf("a 256 character id was accepted")
	}
	for _, bad := range []string{"", "x", "a.", ".a", "a..b", "a.b.", "a b.c", "a.b c", "a.b\n", "a.b;c", "a.b:c", "a.b/c", "-a.b", "a.-b", "_a.b", "a._b"} {
		if got := plainAppID(bad); got != "" {
			t.Errorf("plainAppID(%q) = %q, want it dropped", bad, got)
		}
	}
}

func TestAppIDMismatchWarning(t *testing.T) {
	h := func(id string) HandshakeResult { return HandshakeResult{AppID: id} }
	for _, tc := range []struct {
		name       string
		got, want  string
		wantNotice bool
	}{
		{"the same app", "com.example.app", "com.example.app", false},
		{"another app", "com.other.app", "com.example.app", true},
		{"a suffixed build is another id", "com.example.app.debug", "com.example.app", true},
		{"a longer id that starts the same", "com.example.application", "com.example.app", true},
		{"the agent does not say", "", "com.example.app", false},
		{"the project does not say", "com.example.app", "", false},
		{"neither says", "", "", false},
	} {
		w := AppIDMismatchWarning(h(tc.got), tc.want)
		if (w != "") != tc.wantNotice {
			t.Errorf("%s: warning = %q, want a warning: %v", tc.name, w, tc.wantNotice)
		}
		if tc.wantNotice && (!strings.Contains(w, tc.got) || !strings.Contains(w, tc.want) || !strings.Contains(w, "project.app")) {
			t.Errorf("%s: the warning must name both ids and project.app: %q", tc.name, w)
		}
	}
}
