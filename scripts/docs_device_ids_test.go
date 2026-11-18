package scripts

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The public documents (everything under docs/ and the website's documentation pages, and the README) must not carry a device
// identifier: a simulator UDID, or the serial of an emulator that someone started (`emulator-5600`). Results name the device
// (`Compat-Pixel-API35`). The default adb names that the docs use as examples (`emulator-5554` and `emulator-5556`, the first two
// emulators of a machine) are not a device of ours and are allowed. They also must not link to a private page of an AI assistant
// (claude.ai artifacts, chats, shares), which a reader cannot open.

var (
	udidInDocs     = regexp.MustCompile(`\b[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\b`)
	emulatorSerial = regexp.MustCompile(`\bemulator-([0-9]{4,5})\b`)
	privateLink    = regexp.MustCompile(`https?://claude\.ai/(?:code/)?(?:artifact|chat|share|project)s?/[^\s)>"']+`)
)

// placeholderUDID are the example UDIDs that the docs use where a real one would go (`probe device list` output, `--device` examples).
var placeholderUDID = map[string]bool{
	"A1B2C3D4-E5F6-7890-ABCD-EF1234567890": true,
	"00000000-0000-0000-0000-000000000000": true,
}

// publicDocRoots are the places the test reads, relative to the repository root.
var publicDocRoots = []string{"../docs", "../website/src/content/docs", "../README.md"}

// deviceIdentifiers returns the identifiers in text that a compat document must not carry.
func deviceIdentifiers(text string) []string {
	var out []string
	for _, u := range udidInDocs.FindAllString(text, -1) {
		if placeholderUDID[strings.ToUpper(u)] {
			continue
		}
		out = append(out, u)
	}
	for _, m := range emulatorSerial.FindAllStringSubmatch(text, -1) {
		if m[1] == "5554" || m[1] == "5556" {
			continue
		}
		out = append(out, m[0])
	}
	return out
}

// publicDocProblems returns "relative path: what" for every identifier or private link in the Markdown files under roots.
func publicDocProblems(roots []string) (problems []string, scanned int, err error) {
	for _, root := range roots {
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if ext := filepath.Ext(p); ext != ".md" && ext != ".mdx" {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			scanned++
			rel := filepath.ToSlash(strings.TrimPrefix(p, "../"))
			for _, id := range deviceIdentifiers(string(b)) {
				problems = append(problems, rel+": "+id)
			}
			for _, l := range privateLink.FindAllString(string(b), -1) {
				problems = append(problems, rel+": "+l)
			}
			return nil
		})
		if err != nil {
			return problems, scanned, err
		}
	}
	sort.Strings(problems)
	return problems, scanned, nil
}

func TestPublicDocsCarryNoDeviceIdentifiersOrPrivateLinks(t *testing.T) {
	problems, scanned, err := publicDocProblems(publicDocRoots)
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 60 {
		t.Fatalf("read only %d documents: the walk is not reaching the docs", scanned)
	}
	for _, p := range problems {
		t.Errorf("a public document carries a device identifier or a private link (name the device, drop the link): %s", p)
	}
}

// Plants one of each in a temporary tree, so the walk and both rules are proved to work even while the real tree is clean.
func TestPublicDocProblemsFindsPlantedOnes(t *testing.T) {
	root := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/a/serial.md", "ran on emulator-5600\n")
	write("docs/b/udid.md", "simulator 7CBE6910-6286-44FC-BC18-97A7EDF3A1DF\n")
	write("docs/c/link.md", "Full version: https://claude.ai/code/artifact/77e4a70f-260b-43ec-ba3f-522c1cf557ae\n")
	write("docs/d/chat.md", "see (https://claude.ai/chat/abc-123)\n")
	write("docs/e/fine.md", "adb -s emulator-5554 and Compat-iPhone and https://example.com/x\n")
	write("docs/f/not-markdown.txt", "emulator-5600\n")
	write("docs/node_modules/x/README.md", "emulator-5600\n")
	problems, scanned, err := publicDocProblems([]string{filepath.Join(root, "docs")})
	if err != nil {
		t.Fatal(err)
	}
	if scanned != 4+1 { // serial, udid, link, chat and fine
		t.Errorf("read %d documents, want 5", scanned)
	}
	want := map[string]bool{"a/serial.md": false, "b/udid.md": false, "c/link.md": false, "d/chat.md": false}
	for _, p := range problems {
		for k := range want {
			if strings.Contains(p, k) {
				want[k] = true
			}
		}
		if strings.Contains(p, "fine.md") || strings.Contains(p, "not-markdown") || strings.Contains(p, "node_modules") {
			t.Errorf("unexpected problem: %s", p)
		}
	}
	for k, found := range want {
		if !found {
			t.Errorf("the planted problem in %s was not found (got %v)", k, problems)
		}
	}
}

func TestDeviceIdentifiersCases(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"a simulator UDID", "on simulator `A57AEE67-3473-4C20-BE25-8847C2A9D6BE`", []string{"A57AEE67-3473-4C20-BE25-8847C2A9D6BE"}},
		{"the placeholder UDID of the docs", "--device A1B2C3D4-E5F6-7890-ABCD-EF1234567890", nil},
		{"the same placeholder in lower case", "a1b2c3d4-e5f6-7890-abcd-ef1234567890", nil},
		{"the all-zero UDID", "00000000-0000-0000-0000-000000000000", nil},
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
