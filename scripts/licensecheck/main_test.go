package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// check runs the tool on a mode, a file and more arguments and returns its exit code and output.
func check(t *testing.T, mode, file string, more ...string) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(append([]string{mode, file}, more...), &out, &errb)
	return code, out.String() + errb.String()
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func npmJSON(licence string) string {
	return `{"fake@1.0.0": {"licenses": ` + licence + `}, "ok@1.0.0": {"licenses": "MIT"}}`
}

// The self-test of the check: the cases measured on the old one (exit codes 1, 0, 0, 0, 0, 0), now through the whole tool.
func TestExitCodesOfTheCasesTheOldCheckMissed(t *testing.T) {
	for _, tc := range []struct {
		licence string
		want    int
	}{
		{`"GPL-3.0"`, 1}, {`"GPL-3.0-or-later"`, 1}, {`"GPL-2.0-only"`, 1}, {`"AGPL-3.0-only"`, 1}, {`"(GPL-3.0 AND MIT)"`, 1},
		{`"(MIT OR GPL-3.0)"`, 0}, {`"MIT"`, 0}, {`"LGPL-3.0-or-later"`, 0}, {`"UNKNOWN"`, 0},
		{`["MIT", "GPL-3.0"]`, 0}, {`["GPL-2.0", "GPL-3.0"]`, 1},
	} {
		code, out := check(t, "npm", writeTemp(t, "lc.json", npmJSON(tc.licence)))
		if code != tc.want {
			t.Errorf("licence %s: exit %d, want %d\n%s", tc.licence, code, tc.want, out)
		}
	}
}

// An allowlist silences a note and never a strong copyleft licence.
func TestAllowlistNeverSilencesAStrongLicence(t *testing.T) {
	allow := writeTemp(t, "allow.txt", "fake@* | reviewed, but this reason must not matter\n")
	for _, tc := range []struct {
		licence string
		want    int
		note    bool
	}{
		{`"GPL-3.0-or-later"`, 1, true},
		{`"(GPL-3.0 AND MIT)"`, 1, true},
		{`"MIT/GPL-3.0"`, 1, true},
		{`"LicenseRef-GPL-3.0"`, 1, true},
		{`"LGPL-3.0-or-later"`, 0, false},
		{`"UNKNOWN"`, 0, false},
	} {
		code, out := check(t, "npm", writeTemp(t, "lc.json", npmJSON(tc.licence)), "--allow", allow)
		if code != tc.want || strings.Contains(out, "fake@1.0.0") != tc.note {
			t.Errorf("licence %s with fake@* allowed: exit %d, mentioned=%v, want exit %d, mentioned=%v\n%s", tc.licence, code, strings.Contains(out, "fake@1.0.0"), tc.want, tc.note, out)
		}
	}
}

func TestAllowlistMatchesNameAndVersion(t *testing.T) {
	js := writeTemp(t, "lc.json", `{"@scope/pkg@1.2.3": {"licenses": "UNKNOWN"}}`)
	for allow, wantNote := range map[string]bool{
		"@scope/pkg@1.2.3 | exact":    false,
		"@scope/pkg@* | any version":  false,
		"@scope/pkg@9.9.9 | other":    true,
		"@scope/other@* | other name": true,
	} {
		code, out := check(t, "npm", js, "--allow", writeTemp(t, "a.txt", allow+"\n"))
		if code != 0 || strings.Contains(out, "NOTE") != wantNote {
			t.Errorf("allowlist %q: exit %d, note=%v, want note=%v\n%s", allow, code, strings.Contains(out, "NOTE"), wantNote, out)
		}
	}
}

func TestBadInputIsExitTwoNeverAPass(t *testing.T) {
	good := writeTemp(t, "lc.json", npmJSON(`"MIT"`))
	for name, args := range map[string][]string{
		"no arguments":             {},
		"unknown mode":             {"pip", good},
		"missing file":             {"npm", filepath.Join(t.TempDir(), "absent.json")},
		"not json":                 {"npm", writeTemp(t, "x.json", "<html>")},
		"empty object":             {"npm", writeTemp(t, "e.json", "{}")},
		"go without --own":         {"go", writeTemp(t, "g.csv", "a/b,u,MIT\n")},
		"go with only own rows":    {"go", writeTemp(t, "o.csv", "own/x,Unknown,Unknown\n"), "--own", "own/"},
		"allowlist without reason": {"npm", good, "--allow", writeTemp(t, "a.txt", "fake@1.0.0\n")},
		"allowlist empty reason":   {"npm", good, "--allow", writeTemp(t, "a.txt", "fake@1.0.0 |  \n")},
		"allowlist without ver":    {"npm", good, "--allow", writeTemp(t, "a.txt", "fake | reason\n")},
		"unexpected argument":      {"npm", good, "--verbose"},
	} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 {
			t.Errorf("%s: exit %d, want 2\n%s%s", name, code, out.String(), errb.String())
		}
	}
}

// --- the real captures (testdata/README.md)

func TestRealCaptureOfTheExtension(t *testing.T) {
	file := "testdata/license-checker-vscode.json"
	code, out := check(t, "npm", file)
	if code != 0 || !strings.Contains(out, "@vscode/vsce-sign@2.1.1") || !strings.Contains(out, "flutterprobe@0.27.0") {
		t.Errorf("without the allowlist: exit %d, want 0 with notes for vsce-sign and the extension itself\n%s", code, out)
	}
	if strings.Contains(out, "Artistic-2.0") {
		t.Errorf("Artistic-2.0 is a permissive licence and must not be noted\n%s", out)
	}
	code, out = check(t, "npm", file, "--allow", "allowlist.txt")
	if code != 0 || strings.Contains(out, "NOTE") {
		t.Errorf("with the allowlist of this repository: exit %d, want 0 and no note\n%s", code, out)
	}
}

func TestRealCaptureOfTheWebsite(t *testing.T) {
	file := "testdata/license-checker-website.json"
	code, out := check(t, "npm", file)
	if code != 0 || !strings.Contains(out, "sharp-libvips") || !strings.Contains(out, "weak copyleft") {
		t.Errorf("without the allowlist: exit %d, want 0 with a note for sharp-libvips\n%s", code, out)
	}
	if code, out = check(t, "npm", file, "--allow", "allowlist.txt"); code != 0 || strings.Contains(out, "NOTE") {
		t.Errorf("with the allowlist: exit %d, want 0 and no note\n%s", code, out)
	}
}

func TestRealCapturesOfTheGoModules(t *testing.T) {
	for _, f := range []string{"testdata/go-licenses-studio.csv", "testdata/go-licenses-probe-convert.csv"} {
		code, out := check(t, "go", f, "--own", "github.com/alphawavesystems/")
		if code != 0 || !strings.Contains(out, "0 failed") {
			t.Errorf("%s: exit %d, want 0\n%s", f, code, out)
		}
		// with the wrong prefix, this repository's own packages (which go-licenses reports as Unknown) look like third-party packages with no licence
		if code, out = check(t, "go", f, "--own", "github.com/nobody/"); code != 1 || !strings.Contains(out, "no recognised licence file") {
			t.Errorf("%s with a wrong --own: exit %d, want 1\n%s", f, code, out)
		}
	}
}

func TestGoUnknownIsAFailureAndAGPLToo(t *testing.T) {
	csv := writeTemp(t, "g.csv", "own/x,Unknown,Unknown\nexample.com/a,https://x/LICENSE,MIT\nexample.com/b,Unknown,Unknown\n")
	if code, out := check(t, "go", csv, "--own", "own/"); code != 1 || !strings.Contains(out, "example.com/b") {
		t.Errorf("an unlicensed third-party package must fail: exit %d\n%s", code, out)
	}
	allow := writeTemp(t, "a.txt", "example.com/b@* | reviewed: public domain dedication in the README\n")
	if code, out := check(t, "go", csv, "--own", "own/", "--allow", allow); code != 0 {
		t.Errorf("a reviewed unlicensed package can be allowed: exit %d\n%s", code, out)
	}
	agpl := writeTemp(t, "g2.csv", "own/x,Unknown,Unknown\nexample.com/c,https://x/LICENSE,AGPL-3.0\n")
	if code, _ := check(t, "go", agpl, "--own", "own/", "--allow", writeTemp(t, "a2.txt", "example.com/c@* | no\n")); code != 1 {
		t.Errorf("AGPL-3.0 must fail even when listed: exit %d", code)
	}
}
