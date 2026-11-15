package runner_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

func TestParsePropertiesRefusalsNameTheRuleNotTheValue(t *testing.T) {
	secret := "hunter2-Zq8"
	bad := [][]string{
		{"noequals" + secret},
		{"=" + secret},
		{"bad key=" + secret},
		{"a/b=" + secret},
		{"ké=" + secret},
		{strings.Repeat("k", runner.MaxPropertyKeyBytes+1) + "=" + secret},
		{"run_label=" + secret},
		{"tags=" + secret},
		{"k=" + strings.Repeat("v", runner.MaxPropertyValueBytes+1) + secret},
		{"k=" + secret, "k=" + secret},
	}
	for _, in := range bad {
		_, err := runner.ParseProperties(in)
		if err == nil {
			t.Errorf("%.40q was accepted", in)
			continue
		}
		if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "bad key") {
			t.Errorf("the refusal repeats the entry: %v", err)
		}
	}
	many := make([]string, runner.MaxProperties+1)
	for i := range many {
		many[i] = "k" + strings.Repeat("x", i) + "=v"
	}
	if _, err := runner.ParseProperties(many); err == nil {
		t.Error("33 properties were accepted")
	}
}

func TestParsePropertiesAcceptsTheBounds(t *testing.T) {
	in := []string{
		"a.b-c_D9=" + strings.Repeat("v", runner.MaxPropertyValueBytes),
		strings.Repeat("k", runner.MaxPropertyKeyBytes) + "=",
		"eq=a=b=c", // only the first = separates
	}
	got, err := runner.ParseProperties(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["eq"] != "a=b=c" || len(got["a.b-c_D9"]) != runner.MaxPropertyValueBytes {
		t.Fatalf("got %d entries, eq=%q", len(got), got["eq"])
	}
	thirtyTwo := make([]string, runner.MaxProperties)
	for i := range thirtyTwo {
		thirtyTwo[i] = "k" + strings.Repeat("x", i) + "=v"
	}
	if _, err := runner.ParseProperties(thirtyTwo); err != nil {
		t.Fatalf("32 properties: %v", err)
	}
}

// Values the XML 1.0 format cannot carry, and markup, must come out as a file that parses and says what was meant.
var hostileTexts = []string{
	"a]]>b", "<img src=x onerror=alert(1)>", `"quoted" & 'single' <tag>`, "line1\nline2\ttab\rcr",
	"nul\x00bell\x07vt\x0Bff\x0Cesc\x1B", "bad\xff\xfeutf8", "nonchars\uFFFE\uFFFF\U0001FFFE\uFDD0end", "sep\u2028x\u2029y", "del\x7fc1\u0085end",
	"</script><script>alert(1)</script>", "]]>]]&gt;&#0;&#x0;", "emoji 💥 ünï",
}

func renderJUnitWith(t *testing.T, results []runner.TestResult, props map[string]string) ([]byte, runner.JUnitOptions) {
	t.Helper()
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJUnit, &buf, false)
	o := runner.JUnitOptions{Base: "/work"}
	rep.SetJUnitOptions(o)
	rep.SetProperties(props)
	if err := rep.Report(results); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), o
}

type xmlProp struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}
type xmlSuites struct {
	Suites []struct {
		Props []xmlProp `xml:"properties>property"`
		Cases []struct {
			Name  string    `xml:"name,attr"`
			Props []xmlProp `xml:"properties>property"`
		} `xml:"testcase"`
	} `xml:"testsuite"`
}

func TestJUnitTagsAndPropertiesAreWellFormedXML(t *testing.T) {
	for _, h := range hostileTexts {
		results := []runner.TestResult{{TestName: "t", File: "tests/a.probe", Passed: true, Row: -1, Tags: []string{"ok", h}}}
		out, _ := renderJUnitWith(t, results, map[string]string{"build": h})
		var parsed xmlSuites
		if err := xml.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("%q: output is not well-formed XML: %v\n%s", h, err, out)
		}
		if len(parsed.Suites) != 1 || len(parsed.Suites[0].Cases) != 1 {
			t.Fatalf("%q: structure changed: %+v", h, parsed)
		}
		var tags, build string
		for _, p := range parsed.Suites[0].Cases[0].Props {
			if p.Name == "tags" {
				tags = p.Value
			}
		}
		for _, p := range parsed.Suites[0].Props {
			if p.Name == "build" {
				build = p.Value
			}
		}
		if !strings.HasPrefix(tags, "ok,") || build == "" {
			t.Errorf("%q: tags=%q build=%q", h, tags, build)
		}
		// nothing the format forbids survives, and markup is data, never structure
		for _, bad := range []string{"\x00", "\x07", "\x0B", "\x0C", "\x1B", "\uFFFE", "\uFFFF", "\u2028"} {
			if strings.Contains(string(out), bad) || strings.Contains(tags, bad) || strings.Contains(build, bad) {
				t.Errorf("%q: output still holds %q", h, bad)
			}
		}
		if strings.Contains(string(out), "<img") || strings.Contains(string(out), "<script") {
			t.Errorf("%q: markup was written as markup", h)
		}
	}
}

func TestJUnitWithoutTagsOrPropertiesHasNeitherElement(t *testing.T) {
	results := []runner.TestResult{{TestName: "t", File: "tests/a.probe", Passed: true, Row: -1}}
	out, _ := renderJUnitWith(t, results, nil)
	if strings.Contains(string(out), "tags") || strings.Contains(string(out), "<properties") {
		t.Fatalf("unexpected tags or properties:\n%s", out)
	}
	// an empty map and an all-blank tag list are the same as none
	out2, _ := renderJUnitWith(t, []runner.TestResult{{TestName: "t", File: "tests/a.probe", Passed: true, Row: -1, Tags: []string{"", "  "}}}, map[string]string{})
	if !bytes.Equal(out, out2) {
		t.Fatalf("empty tags/properties changed the output:\n%s", out2)
	}
}

func TestReportTagsLimitsAndMasking(t *testing.T) {
	tags := make([]string, 40)
	for i := range tags {
		tags[i] = "t" + string(rune('a'+i%26)) + strings.Repeat("x", i)
	}
	got := runner.ReportTags(tags)
	if len(got) != runner.MaxTags {
		t.Fatalf("%d tags reported", len(got))
	}
	long := runner.ReportTags([]string{strings.Repeat("é", 200)})
	if len([]rune(long[0])) != runner.MaxTagRunes {
		t.Fatalf("a long tag is %d characters", len([]rune(long[0])))
	}
	sysdialog.NoteSecret("tag-secret-Wq7z")
	if m := runner.ReportTags([]string{"release tag-secret-Wq7z"}); strings.Contains(m[0], "tag-secret-Wq7z") {
		t.Fatalf("a known secret in a tag was written: %q", m[0])
	}
}

func TestPropertyValuesAreMaskedWhenTheReportIsWritten(t *testing.T) {
	props, err := runner.ParseProperties([]string{"build=run-secret-Lk3p"})
	if err != nil {
		t.Fatal(err)
	}
	sysdialog.NoteSecret("run-secret-Lk3p") // the run learns the secret after the flags were read
	out, _ := renderJUnitWith(t, []runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1}}, props)
	if strings.Contains(string(out), "run-secret-Lk3p") {
		t.Fatalf("junit leaked the value:\n%s", out)
	}
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJSON, &buf, false)
	rep.SetProperties(props)
	if err := rep.Report([]runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1, Tags: []string{"x run-secret-Lk3p"}}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "run-secret-Lk3p") {
		t.Fatalf("json leaked the value:\n%s", buf.String())
	}
}

func TestJSONReportCarriesTagsAndPropertiesOnlyWhenPresent(t *testing.T) {
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJSON, &buf, false)
	if err := rep.Report([]runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "tags") || strings.Contains(buf.String(), "properties") {
		t.Fatalf("keys present without data:\n%s", buf.String())
	}
	buf.Reset()
	rep = runner.NewReporter(runner.FormatJSON, &buf, false)
	rep.SetProperties(map[string]string{"b": "2", "a": "1"})
	if err := rep.Report([]runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1, Tags: []string{"smoke", "<b>"}}}); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties map[string]string `json:"properties"`
		Results    []struct {
			Tags []string `json:"tags"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Properties["a"] != "1" || doc.Properties["b"] != "2" || len(doc.Results[0].Tags) != 2 || doc.Results[0].Tags[1] != "<b>" {
		t.Fatalf("%+v", doc)
	}
}

// Whatever the file or the flags carried, what is written is valid text with no control or non-character in it, so the JSON
// and HTML reports (which do not pass through an XML encoder) are as clean as the JUnit one.
func TestReportTextIsCleanForEveryWriter(t *testing.T) {
	for _, h := range hostileTexts {
		tags := runner.ReportTags([]string{"x" + h})
		props := runner.ReportProperties(map[string]string{"k": h})
		texts := append([]string{}, tags...)
		for _, p := range props {
			texts = append(texts, p.Value)
		}
		for _, s := range texts {
			if !utf8.ValidString(s) {
				t.Errorf("%q: invalid UTF-8 written: %q", h, s)
			}
			for _, r := range s {
				if unicode.IsControl(r) || r == 0x2028 || r == 0x2029 || r == 0xFFFE || r == 0xFFFF || r&0xFFFE == 0xFFFE || (r >= 0xFDD0 && r <= 0xFDEF) {
					t.Errorf("%q: %U written in %q", h, r, s)
				}
			}
		}
	}
	// a hand-made file can hold a value over the limit; the writers leave it out
	if got := runner.ReportProperties(map[string]string{"big": strings.Repeat("v", runner.MaxPropertyValueBytes+1), "ok": "1"}); len(got) != 1 || got[0].Name != "ok" {
		t.Errorf("oversize value written: %+v", got)
	}
}

// Bytes that are not UTF-8 are replaced by a space, like any other character the reports cannot carry.
func TestReportTextReplacesInvalidUTF8WithASpace(t *testing.T) {
	if got := runner.ReportTags([]string{"a\xffb"}); len(got) != 1 || got[0] != "a b" {
		t.Fatalf("got %q", got)
	}
}
