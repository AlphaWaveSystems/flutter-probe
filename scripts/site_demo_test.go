package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The sample dashboard (/demo/) is built from website/demo-templates by website/scripts/build-demo.mjs: every date moves so that the newest day is the
// build day, and the pages carry the site header with an Apply button. These tests run the real generator and check what a visitor sees.
var (
	demoDate     = regexp.MustCompile(`(?:^|[^\d-])(\d{4}-\d{2}-\d{2})(?:\D|$)`)
	demoPages    = []string{"index.html", "7-days.html", "90-days.html"}
	demoAnchorRe = regexp.MustCompile(`export const ANCHOR = '(\d{4}-\d{2}-\d{2})'`)
)

func buildDemo(t *testing.T, date string) string {
	t.Helper()
	out := t.TempDir()
	cmd := exec.Command(nodeOrSkip(t), "scripts/build-demo.mjs", "--date", date, "--out", out)
	cmd.Dir = "../website"
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-demo failed: %v\n%s", err, b)
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func distinctDates(s string) []string {
	seen := map[string]bool{}
	for _, m := range demoDate.FindAllStringSubmatch(s, -1) {
		seen[m[1]] = true
	}
	var out []string
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// The templates' newest day is the anchor the generator shifts from: if they disagree, "the newest day is the build day" is false.
func TestDemoTemplatesNewestDayIsTheAnchor(t *testing.T) {
	m := demoAnchorRe.FindStringSubmatch(readFile(t, "../website/scripts/build-demo.mjs"))
	if m == nil {
		t.Fatal("build-demo.mjs has no `export const ANCHOR = 'YYYY-MM-DD'`")
	}
	for _, p := range demoPages {
		d := distinctDates(readFile(t, filepath.Join("../website/demo-templates", p)))
		if len(d) == 0 || d[len(d)-1] != m[1] {
			t.Errorf("%s: the newest date in the template is %v, the anchor is %s", p, d, m[1])
		}
	}
}

func TestDemoBuildMovesTheNewestDayToTheBuildDay(t *testing.T) {
	const date = "2027-03-05"
	out := buildDemo(t, date)
	for _, p := range demoPages {
		tpl := distinctDates(readFile(t, filepath.Join("../website/demo-templates", p)))
		got := readFile(t, filepath.Join(out, p))
		gd := distinctDates(got)
		if len(gd) != len(tpl) || gd[len(gd)-1] != date {
			t.Errorf("%s: %d dates in the template, %d in the page, newest %v: want the same count and %s as the newest", p, len(tpl), len(gd), gd[len(gd)-1:], date)
		}
		if !strings.Contains(got, "Computed "+date+"T12:00:00Z") {
			t.Errorf("%s: the footer does not say Computed %sT12:00:00Z", p, date)
		}
		if strings.Contains(got, "{{") {
			t.Errorf("%s: a placeholder is left in the page", p)
		}
	}
}

// Every page has the site header (the way back to the site) and an Apply button to the design-partner form of sellable.json, and the notice that the data is
// made up comes before the header, so it is the first thing on the page.
func TestDemoPagesHaveTheSiteHeaderAnApplyButtonAndTheSampleNotice(t *testing.T) {
	var sell struct {
		Apply struct{ Form string } `json:"apply"`
	}
	if err := json.Unmarshal([]byte(readFile(t, "../website/sellable.json")), &sell); err != nil {
		t.Fatal(err)
	}
	out := buildDemo(t, "2027-03-05")
	for _, p := range demoPages {
		got := readFile(t, filepath.Join(out, p))
		notice := strings.Index(got, "Sample data: a made-up fixture app, not a real team.")
		header := strings.Index(got, `<header class="site">`)
		if notice < 0 || header < 0 || notice > header {
			t.Errorf("%s: the sample-data notice (at %d) must come before the site header (at %d)", p, notice, header)
		}
		for _, href := range []string{`href="/"`, `href="/probe-cloud/"`, `href="/pricing/"`, `href="/getting-started/installation/"`} {
			if !strings.Contains(got, href) {
				t.Errorf("%s: the site header has no link %s", p, href)
			}
		}
		if !strings.Contains(got, `<a class="apply" href="`+sell.Apply.Form+`">Apply as a design partner</a>`) {
			t.Errorf("%s: no Apply button to %s", p, sell.Apply.Form)
		}
	}
}

// Commit hashes look like commit hashes (ten hex characters, not one repeated), and a file name breaks at its slash, not in the middle of a word.
func TestDemoPagesHaveRealisticHashesAndFileNamesThatDoNotBreakMidWord(t *testing.T) {
	out := buildDemo(t, "2027-03-05")
	anyHex := regexp.MustCompile(`<code>([0-9a-f]{6,})</code>`)
	hash := regexp.MustCompile(`<code>[0-9a-f]{10}</code>`)
	path := regexp.MustCompile(`<code>tests/([^<]*)</code>`)
	for _, p := range demoPages {
		got := readFile(t, filepath.Join(out, p))
		for _, m := range anyHex.FindAllStringSubmatch(got, -1) {
			if strings.Count(m[1], m[1][:1]) == len(m[1]) {
				t.Errorf("%s: %s is a commit hash of one repeated character", p, m[1])
			}
		}
		if p != "90-days.html" && !hash.MatchString(got) {
			t.Errorf("%s: no 10-character commit hash in the pull request table", p)
		}
		if !strings.Contains(got, "td.t code{word-break:normal;overflow-wrap:normal}") {
			t.Errorf("%s: a file name in a test cell inherits word-break:break-word, which lets the column shrink until the name breaks inside a word (seen at 400 px): td.t code needs word-break:normal and overflow-wrap:normal", p)
		}
		if !strings.Contains(got, "td.d{white-space:nowrap}") || regexp.MustCompile(`<td>\d{4}-\d{2}-\d{2}</td>`).MatchString(got) {
			t.Errorf("%s: a date cell must have class d (white-space:nowrap), so a date does not wrap onto two lines at 400 px", p)
		}
		if strings.Contains(got, "word-break:break-all") {
			t.Errorf("%s: word-break:break-all breaks a file name in the middle of a word on a narrow screen", p)
		}
		for _, m := range path.FindAllString(got, -1) {
			if !strings.Contains(m, "tests/<wbr>") {
				t.Errorf("%s: %s has no <wbr> after its slash, so it can only break inside a word", p, m)
			}
		}
	}
}

// The pages are static and say so in their policy (default-src 'none'): no script, no external resource; the only link out of the site is the apply form.
func TestDemoPagesReferenceNothingExternal(t *testing.T) {
	out := buildDemo(t, "2027-03-05")
	var sell struct {
		Apply struct{ Form string } `json:"apply"`
	}
	if err := json.Unmarshal([]byte(readFile(t, "../website/sellable.json")), &sell); err != nil {
		t.Fatal(err)
	}
	ref := regexp.MustCompile(`(?:href|src|action)="(https?:[^"]*)"`)
	for _, p := range demoPages {
		got := readFile(t, filepath.Join(out, p))
		if strings.Contains(got, "<script") {
			t.Errorf("%s: has a script", p)
		}
		for _, m := range ref.FindAllStringSubmatch(got, -1) {
			if m[1] != sell.Apply.Form {
				t.Errorf("%s: references %s, outside the site", p, m[1])
			}
		}
	}
}

// The pages are generated on every build and are not committed: the build and the dev server must run the generator.
func TestWebsiteBuildAndDevRunTheDemoGenerator(t *testing.T) {
	var pkg struct{ Scripts map[string]string }
	if err := json.Unmarshal([]byte(readFile(t, "../website/package.json")), &pkg); err != nil {
		t.Fatal(err)
	}
	build := pkg.Scripts["build"]
	if i, j := strings.Index(build, "scripts/build-demo.mjs"), strings.Index(build, "astro build"); i < 0 || j < 0 || i > j {
		t.Errorf("the build must run scripts/build-demo.mjs before astro build, is %q", build)
	}
	if dev := pkg.Scripts["dev"]; !strings.Contains(dev, "scripts/build-demo.mjs") {
		t.Errorf("dev must run scripts/build-demo.mjs first, is %q", dev)
	}
	if !strings.Contains(readFile(t, "../.gitignore"), "website/public/demo/") {
		t.Error(".gitignore does not ignore the generated website/public/demo/")
	}
}

// The apply link goes into the pages as given: a URL that holds `$&`, `$1` or `$$` is not read as a replacement pattern.
func TestDemoGeneratorKeepsDollarSignsInTheApplyLink(t *testing.T) {
	url := "https://example.org/new?template=x.yml&a=$&&b=$1&c=$$&d=$`&e=$'"
	// The URL goes through the environment: it holds a backtick, which a JavaScript template literal could not carry.
	script := `import { render } from './scripts/build-demo.mjs'; process.stdout.write(render('<a href="{{APPLY_FORM}}">x</a> {{APPLY_FORM}}', '2026-10-11', process.env.APPLY_URL))`
	cmd := exec.Command(nodeOrSkip(t), "--input-type=module", "-e", script)
	cmd.Dir = "../website"
	cmd.Env = append(os.Environ(), "APPLY_URL="+url)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("render failed: %v\n%s", err, b)
	}
	want := `<a href="` + url + `">x</a> ` + url
	if string(b) != want {
		t.Errorf("render produced %q, want %q", b, want)
	}
}

// A folder under the templates whose name ends in .html is not a page: the generator reads files only.
func TestDemoGeneratorReadsFilesOnlyFromTheTemplates(t *testing.T) {
	templates, out := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(templates, "a.html"), []byte(`<p>{{APPLY_FORM}} 2026-10-11</p>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(templates, "folder.html"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(nodeOrSkip(t), "scripts/build-demo.mjs", "--date", "2027-03-05", "--out", out, "--templates", templates)
	cmd.Dir = "../website"
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-demo failed on a folder named .html: %v\n%s", err, b)
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 1 || entries[0].Name() != "a.html" {
		t.Errorf("output = %v (%v), want only a.html", entries, err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "a.html"))
	if !strings.Contains(string(got), "2027-03-05") {
		t.Errorf("the date of the template did not move: %s", got)
	}
}
