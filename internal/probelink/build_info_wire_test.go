package probelink

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// wireClient is one element of wireClients.
type wireClient = struct {
	name    string
	connect func(t *testing.T, agent keyAgent) (ProbeClient, func())
}

func handshakeOf(t *testing.T, wc wireClient, agent keyAgent) *HandshakeResult {
	t.Helper()
	client, done := wc.connect(t, agent)
	defer done()
	h, err := client.Handshake(context.Background(), "0.27.0")
	if err != nil {
		t.Fatalf("%s: Handshake: %v", wc.name, err)
	}
	return h
}

// An agent that sends the build info of the app: both clients carry it.
func TestHandshakeCarriesTheBuildInfo(t *testing.T) {
	for _, wc := range wireClients {
		agent := keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1), extra: map[string]any{
			"flutter_version": "3.50.0", "flutter_channel": "stable", "framework_revision": "ab29120", "dart_version": "3.14.0",
		}}
		h := handshakeOf(t, wc, agent)
		if h.FlutterVersion != "3.50.0" || h.FlutterChannel != "stable" || h.FrameworkRevision != "ab29120" || h.DartVersion != "3.14.0" {
			t.Errorf("%s: %+v", wc.name, h)
		}
		if h.AgentVersion != "0.27.0" {
			t.Errorf("%s: the agent version is unchanged: %+v", wc.name, h)
		}
	}
}

// An agent from before the fields (or a build whose tool set none): no error, empty fields, everything else as before.
func TestHandshakeWithoutBuildInfoIsUnchanged(t *testing.T) {
	for _, wc := range wireClients {
		h := handshakeOf(t, wc, keyAgent{version: "0.26.0", capabilities: []string{CapabilityPressKey}, actions: make(chan json.RawMessage, 1)})
		if h.FlutterVersion != "" || h.FlutterChannel != "" || h.FrameworkRevision != "" || h.DartVersion != "" {
			t.Errorf("%s: fields from nowhere: %+v", wc.name, h)
		}
		if h.AgentVersion != "0.26.0" || len(h.Capabilities) != 1 {
			t.Errorf("%s: %+v", wc.name, h)
		}
		// the field may be present and empty: still empty, not an error
		h = handshakeOf(t, wc, keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1), extra: map[string]any{"flutter_version": "", "dart_version": nil}})
		if h.FlutterVersion != "" || h.DartVersion != "" {
			t.Errorf("%s: empty fields: %+v", wc.name, h)
		}
	}
}

// The text comes from the app: anything that is not plain version text is dropped, field by field, and the rest still counts.
func TestHandshakeDropsBuildInfoThatIsNotPlainText(t *testing.T) {
	long := strings.Repeat("a", 65)
	hostile := []string{
		"3.50.0\n3.49.0", "3.50.0; rm -rf /", "<script>alert(1)</script>", "3.50.0\u202e", "3.5\u200b0.0", "\x1b[31m3.50.0", "3.50.0 ", " 3.50.0", "3.50.0\n", "3.50.0\r\n", "\n3.50.0", "v 3", "é", "3.50.0\x00", long,
	}
	for _, wc := range wireClients {
		for _, bad := range hostile {
			agent := keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1), extra: map[string]any{
				"flutter_version": bad, "flutter_channel": bad, "framework_revision": bad, "dart_version": "3.14.0",
			}}
			h := handshakeOf(t, wc, agent)
			if h.FlutterVersion != "" || h.FlutterChannel != "" || h.FrameworkRevision != "" {
				t.Errorf("%s: kept %q: %+v", wc.name, bad, h)
			}
			if h.DartVersion != "3.14.0" {
				t.Errorf("%s: a good field was dropped along with a bad one: %+v", wc.name, h)
			}
		}
		// the limit is 64: 64 is kept
		h := handshakeOf(t, wc, keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1), extra: map[string]any{"flutter_channel": strings.Repeat("a", 64)}})
		if len(h.FlutterChannel) != 64 {
			t.Errorf("%s: a 64 character value is plain: %q", wc.name, h.FlutterChannel)
		}
	}
}

// Fields of another type read as empty and never fail the connect: an app is not trusted to send the right JSON.
func TestHandshakeIgnoresBuildInfoOfTheWrongType(t *testing.T) {
	for _, wc := range wireClients {
		for name, v := range map[string]any{"number": 3.5, "object": map[string]any{"a": 1}, "list": []string{"3.50.0"}, "bool": true, "null": nil} {
			h := handshakeOf(t, wc, keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1), extra: map[string]any{
				"flutter_version": v, "flutter_channel": v, "framework_revision": v, "dart_version": "3.14.0",
			}})
			if h.FlutterVersion != "" || h.FlutterChannel != "" || h.FrameworkRevision != "" {
				t.Errorf("%s %s: kept a non-text value: %+v", wc.name, name, h)
			}
			if h.DartVersion != "3.14.0" || h.AgentVersion != "0.27.0" {
				t.Errorf("%s %s: a wrong field spoiled the others: %+v", wc.name, name, h)
			}
		}
	}
}

func TestPlainBuildText(t *testing.T) {
	for in, want := range map[string]string{
		"3.50.0": "3.50.0", "3.50.0-0.1.pre+7": "3.50.0-0.1.pre+7", "stable": "stable", "AB12cd": "AB12cd", "a": "a",
		"": "", "a b": "", "a_b": "", "a/b": "", "a:b": "", "é": "", "a\n": "", "3.50.0\n": "", "3.50.0\r\n": "", "\n3.50.0": "", "\t": "",
	} {
		if got := plainBuildText(in); got != want {
			t.Errorf("plainBuildText(%q) = %q, want %q", in, got, want)
		}
	}
}
