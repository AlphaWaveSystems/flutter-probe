package main

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// The line about the app's build, for every combination of what the agent may have sent. Nothing is shown for what was not sent: no placeholder,
// no guess.
func TestAppBuildText(t *testing.T) {
	for name, tc := range map[string]struct {
		res  probelink.HandshakeResult
		want string
	}{
		"nothing sent":            {probelink.HandshakeResult{}, ""},
		"an old agent":            {probelink.HandshakeResult{AgentVersion: "0.26.0", Capabilities: []string{"press_key"}}, ""},
		"everything":              {probelink.HandshakeResult{FlutterVersion: "3.44.8", FlutterChannel: "stable", DartVersion: "3.12.2", FrameworkRevision: "1a2b3c4d5e"}, "App built with Flutter 3.44.8 (stable), Dart 3.12.2 · framework revision 1a2b3c4d5e"},
		"no revision":             {probelink.HandshakeResult{FlutterVersion: "3.44.8", FlutterChannel: "stable", DartVersion: "3.12.2"}, "App built with Flutter 3.44.8 (stable), Dart 3.12.2"},
		"no channel":              {probelink.HandshakeResult{FlutterVersion: "3.44.8", DartVersion: "3.12.2"}, "App built with Flutter 3.44.8, Dart 3.12.2"},
		"no dart":                 {probelink.HandshakeResult{FlutterVersion: "3.44.8", FlutterChannel: "beta"}, "App built with Flutter 3.44.8 (beta)"},
		"only dart":               {probelink.HandshakeResult{DartVersion: "3.12.2"}, "App built with Dart 3.12.2"},
		"only a channel":          {probelink.HandshakeResult{FlutterChannel: "master"}, "App built with Flutter channel master"},
		"only a revision":         {probelink.HandshakeResult{FrameworkRevision: "abc1234"}, "Framework revision abc1234"},
		"a revision and dart":     {probelink.HandshakeResult{DartVersion: "3.12.2", FrameworkRevision: "abc1234"}, "App built with Dart 3.12.2 · framework revision abc1234"},
		"a version with a suffix": {probelink.HandshakeResult{FlutterVersion: "3.45.0-0.1.pre", FlutterChannel: "beta"}, "App built with Flutter 3.45.0-0.1.pre (beta)"},
	} {
		if got := appBuildText(tc.res); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

// The handshake result reaches the status: a connection whose agent sent the facts shows them, and one whose agent did not shows nothing.
func TestAppBuildReachesTheStatus(t *testing.T) {
	a := &App{conn: &connection{client: &runFake{}, appBuild: "App built with Flutter 3.44.8 (stable), Dart 3.12.2", deviceName: "Phone", platform: "ios"}}
	st := a.Status()
	if st.AppBuild != "App built with Flutter 3.44.8 (stable), Dart 3.12.2" {
		t.Errorf("status = %+v", st)
	}
	raw, _ := json.Marshal(st)
	if !strings.Contains(string(raw), `"appBuild"`) {
		t.Errorf("json = %s", raw)
	}
	b := &App{conn: &connection{client: &runFake{}, deviceName: "Phone", platform: "ios"}}
	raw, _ = json.Marshal(b.Status())
	if strings.Contains(string(raw), "appBuild") {
		t.Errorf("an agent that reported nothing leaves a field in the status: %s", raw)
	}
	if got := (&App{}).Status().AppBuild; got != "" {
		t.Errorf("disconnected: %q", got)
	}
}

// A WiFi connection to a fake agent that sends the facts carries them (the real handshake, the real parse and validation).
func TestAConnectionKeepsWhatTheHandshakeReported(t *testing.T) {
	agent := newFakeAgent(t)
	agent.pingExtra = map[string]any{"flutter_version": "3.44.8", "flutter_channel": "stable", "dart_version": "3.12.2", "framework_revision": "1a2b3c4d5e", "agent_version": "0.27.0"}
	a := NewApp()
	a.ctx = context.Background()
	t.Cleanup(func() { a.shutdown(context.Background()) })
	st, err := a.ConnectWiFi("127.0.0.1", agent.port, "abcdefgh1234")
	if err != nil {
		t.Fatal(err)
	}
	if want := "App built with Flutter 3.44.8 (stable), Dart 3.12.2 · framework revision 1a2b3c4d5e"; st.AppBuild != want || a.Status().AppBuild != want {
		t.Errorf("connect = %q, status = %q, want %q", st.AppBuild, a.Status().AppBuild, want)
	}
}

// Hostile values: the Go side drops anything that is not plain version text, so what the fake agent sends as markup, bidi or an over-long string
// never reaches the line, and the valid parts are shown alone. (Through the real handshake and its validation.)
func TestAHostileBuildReportShowsOnlyItsValidParts(t *testing.T) {
	agent := newFakeAgent(t)
	agent.pingExtra = map[string]any{
		"flutter_version":    "<img src=x onerror=alert(1)>",
		"flutter_channel":    "stable\u202e",
		"dart_version":       strings.Repeat("9", 65),
		"framework_revision": "ok1234",
	}
	a := NewApp()
	a.ctx = context.Background()
	t.Cleanup(func() { a.shutdown(context.Background()) })
	st, err := a.ConnectWiFi("127.0.0.1", agent.port, "abcdefgh1234")
	if err != nil {
		t.Fatal(err)
	}
	if st.AppBuild != "Framework revision ok1234" {
		t.Errorf("the line is %q: only the valid part may show", st.AppBuild)
	}
	// only a channel is valid: the line says so, as the parts the app reported
	agent.mu.Lock()
	agent.pingExtra = map[string]any{"flutter_version": "<b>3</b>", "flutter_channel": "stable"}
	agent.mu.Unlock()
	a.Disconnect()
	st, err = a.ConnectWiFi("127.0.0.1", agent.port, "abcdefgh1234")
	if err != nil {
		t.Fatal(err)
	}
	if st.AppBuild != "App built with Flutter channel stable" {
		t.Errorf("the line is %q", st.AppBuild)
	}
}

// Both ways of connecting (a device by id, and WiFi) keep what the handshake reported: every place that builds a connection with a client sets
// appBuild. The USB path needs a real device manager and a port forward, so this reads the source.
func TestEveryConnectionKeepsTheBuildReport(t *testing.T) {
	fset := token.NewFileSet()
	files, _ := filepath.Glob("*.go")
	found := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(src, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "connection" {
				return true
			}
			keys := map[string]bool{}
			for _, e := range lit.Elts {
				if kv, ok := e.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok {
						keys[k.Name] = true
					}
				}
			}
			if keys["client"] && keys["deviceID"] {
				found++
				if !keys["appBuild"] {
					t.Errorf("%s: a connection is built without appBuild (the handshake result is dropped)", fset.Position(lit.Pos()))
				}
			}
			return true
		})
	}
	if found != 2 {
		t.Errorf("found %d places that build a connection with a client, want 2 (USB and WiFi): update this test if a path was added", found)
	}
}
