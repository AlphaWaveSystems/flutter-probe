package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The compatibility documents say which devices a result was measured on, by name (`Compat-Pixel-API35`). They must not carry a device
// identifier: a simulator UDID, or the serial of an emulator that someone started (`emulator-5600`). The default adb names that the
// docs use as examples (`emulator-5554` and `emulator-5556`, the first two emulators of a machine) are not a device of ours and are allowed.

const compatDocsDir = "../docs/compat"

var (
	udidInDocs     = regexp.MustCompile(`\b[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\b`)
	emulatorSerial = regexp.MustCompile(`\bemulator-([0-9]{4,5})\b`)
)

// deviceIdentifiers returns the identifiers in text that a compat document must not carry.
func deviceIdentifiers(text string) []string {
	var out []string
	out = append(out, udidInDocs.FindAllString(text, -1)...)
	for _, m := range emulatorSerial.FindAllStringSubmatch(text, -1) {
		if m[1] == "5554" || m[1] == "5556" {
			continue
		}
		out = append(out, m[0])
	}
	return out
}

func TestCompatDocsCarryNoDeviceIdentifiers(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(compatDocsDir, "*.md"))
	if err != nil || len(files) < 3 {
		t.Fatalf("found %d compat documents (err %v): the glob is not reaching docs/compat", len(files), err)
	}
	var problems []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range deviceIdentifiers(string(b)) {
			problems = append(problems, filepath.Base(f)+": "+id)
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Errorf("a compat document carries a device identifier (name the device instead): %s", p)
	}
}

func TestDeviceIdentifiersCases(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"a simulator UDID", "on simulator `A57AEE67-3473-4C20-BE25-8847C2A9D6BE`", []string{"A57AEE67-3473-4C20-BE25-8847C2A9D6BE"}},
		{"a lower-case UDID", "a57aee67-3473-4c20-be25-8847c2a9d6be", []string{"a57aee67-3473-4c20-be25-8847c2a9d6be"}},
		{"an emulator serial", "on emulator-5600 (AVD x)", []string{"emulator-5600"}},
		{"a serial in code", "`emulator-5590`", []string{"emulator-5590"}},
		{"the default adb names", "adb -s emulator-5554 and emulator-5556", nil},
		{"a device name", "`Compat-Pixel-API35` and `Compat-iPhone`", nil},
		{"a build id that is not a UDID", "build 3a4b-5c6d", nil},
		{"two identifiers", "emulator-5600 and emulator-5610", []string{"emulator-5600", "emulator-5610"}},
	} {
		got := deviceIdentifiers(tc.text)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
