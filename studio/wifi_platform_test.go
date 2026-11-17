package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// A WiFi connection has no way to say what the device is (the agent does not advertise it): the status says nothing about the platform instead of
// "ios" for an Android phone. A connection made by Connect (a device Studio listed and classified) says what it is.
func TestAConnectionThatDoesNotKnowItsPlatformSaysNone(t *testing.T) {
	a := NewApp()
	a.conn = &connection{client: &runFake{}, deviceID: "192.0.2.7:48686", deviceName: "WiFi 192.0.2.7", platform: device.PlatformIOS, platformUnknown: true}
	if st := a.Status(); !st.Connected || st.Platform != "" {
		t.Errorf("a connection of unknown platform: %+v, want connected with no platform", st)
	}
	for _, p := range []device.Platform{device.PlatformAndroid, device.PlatformIOS} {
		a.conn = &connection{client: &runFake{}, deviceID: "x", deviceName: "Listed", platform: p}
		if st := a.Status(); st.Platform != string(p) {
			t.Errorf("a listed %s device reports %q", p, st.Platform)
		}
	}
	a.conn = &connection{client: &runFake{}, deviceID: "192.0.2.7:48686", platform: device.PlatformIOS, platformUnknown: true}
	if got := a.conn.platformName(); got != "" {
		t.Errorf("platformName = %q", got)
	}
}

// ConnectWiFi is the only place a connection of unknown platform is made, and it must say so: this reads its source for platformUnknown: true.
func TestConnectWiFiMarksThePlatformUnknown(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "app.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ConnectWiFi" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "platformUnknown" {
					if v, ok := kv.Value.(*ast.Ident); ok && v.Name == "true" {
						found = true
					}
				}
			}
			return true
		})
	}
	if !found {
		t.Error("ConnectWiFi no longer marks its connection platformUnknown: the status would call an Android phone iOS")
	}
}
