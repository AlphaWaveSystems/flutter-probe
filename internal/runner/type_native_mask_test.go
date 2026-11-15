//go:build !windows

package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// standInDevice is an adb that shows one native element ("Search"), accepts taps and fails every `input text`, with the failure text real adb gives (the command line is in the error of the caller).
func standInDevice(t *testing.T) *DeviceContext {
	t.Helper()
	dir := t.TempDir()
	fake := filepath.Join(dir, "adb")
	script := `#!/bin/sh
case "$*" in
  *"uiautomator dump"*) echo "UI hierarchy dumped to: /sdcard/probe_uidump.xml" ;;
  *"cat /sdcard/probe_uidump.xml"*) printf '<hierarchy><node text="Search" resource-id="" class="x" package="p" content-desc="" bounds="[0,0][100,100]"/></hierarchy>' ;;
  *"input tap"*) exit 0 ;;
  *"input text"*) echo "error: device offline" >&2; exit 1 ;;
esac
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &DeviceContext{Manager: device.NewManagerWithPaths(device.ToolPaths{ADB: fake}), Serial: "emulator-5554", Platform: device.PlatformAndroid}
}

// A planted password typed with `type native` must not appear in the error of a failed run, nor in the step description that goes to the log and the report.
func TestTypeNativeFailureAndDescriptionDoNotCarryThePassword(t *testing.T) {
	const secret = "tn-secret-5151"
	sysdialog.NoteSecret(secret)

	dc := standInDevice(t)
	err := dc.TypeNative(context.Background(), "Search", secret)
	if err == nil {
		t.Fatal("expected the typing to fail")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("the error carries the password: %s", err)
	}
	if !strings.Contains(err.Error(), "device offline") {
		t.Errorf("the error lost its cause: %s", err)
	}

	e := &Executor{vars: map[string]string{"pw": secret}}
	desc := e.stepDescription(parser.ActionStep{Verb: parser.VerbTypeNative, Name: "Search", Text: "<pw>"})
	if strings.Contains(desc, secret) {
		t.Errorf("the step description carries the password: %s", desc)
	}
	if !strings.Contains(desc, "type native") || !strings.Contains(desc, "Search") {
		t.Errorf("the step description lost its meaning: %s", desc)
	}
}
