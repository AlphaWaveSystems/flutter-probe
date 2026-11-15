package runner_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// Every name the reports write themselves is refused as a --property key, in any case.
func TestEveryReservedKeyIsRefusedInAnyCase(t *testing.T) {
	reserved := []string{"probe_version", "agent_version", "platform", "os_version", "app_id", "app_version", "device_name", "device_class",
		"provider", "locale", "run_label", "tags", "row", "device", "attempt", "attempts"}
	for _, k := range reserved {
		for _, form := range []string{k, strings.ToUpper(k), strings.ToUpper(k[:1]) + k[1:]} {
			if _, err := runner.ParseProperties([]string{form + "=x"}); err == nil {
				t.Errorf("%s was accepted", form)
			}
			if got := runner.ReportProperties(map[string]string{form: "x"}); len(got) != 0 {
				t.Errorf("%s was written: %+v", form, got)
			}
		}
	}
	// a name that only starts with a reserved word is fine
	if _, err := runner.ParseProperties([]string{"platform_x=1", "tags2=1", "devices=1"}); err != nil {
		t.Errorf("near-miss names: %v", err)
	}
}

func TestPropertyAndTagBoundsAreExact(t *testing.T) {
	// the count: 32 are accepted, 33 are not (each with a distinct key)
	mk := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("k%d=v", i)
		}
		return out
	}
	if _, err := runner.ParseProperties(mk(32)); err != nil {
		t.Errorf("32 properties: %v", err)
	}
	if _, err := runner.ParseProperties(mk(33)); err == nil {
		t.Error("33 properties were accepted")
	}
	// the key: 64 bytes are accepted, 65 are not
	if _, err := runner.ParseProperties([]string{strings.Repeat("k", 64) + "=v"}); err != nil {
		t.Errorf("64-byte key: %v", err)
	}
	if _, err := runner.ParseProperties([]string{strings.Repeat("k", 65) + "=v"}); err == nil {
		t.Error("a 65-byte key was accepted")
	}
	// the value: 1024 bytes are accepted, 1025 are not
	if _, err := runner.ParseProperties([]string{"k=" + strings.Repeat("v", 1024)}); err != nil {
		t.Errorf("1024-byte value: %v", err)
	}
	if _, err := runner.ParseProperties([]string{"k=" + strings.Repeat("v", 1025)}); err == nil {
		t.Error("a 1025-byte value was accepted")
	}
	// tags: 32 are all written, a 33rd is left out; a 64-character tag is whole, a 65-character one is cut to 64
	var tags []string
	for i := 0; i < 33; i++ {
		tags = append(tags, fmt.Sprintf("t%02d", i))
	}
	if got := runner.ReportTags(tags[:32]); len(got) != 32 || got[31] != "t31" {
		t.Errorf("32 tags: %v", got)
	}
	if got := runner.ReportTags(tags); len(got) != 32 || got[31] != "t31" {
		t.Errorf("33 tags: %d written, last %q", len(got), got[len(got)-1])
	}
	if got := runner.ReportTags([]string{strings.Repeat("a", 64)}); got[0] != strings.Repeat("a", 64) {
		t.Errorf("a 64-character tag was changed: %q", got[0])
	}
	if got := runner.ReportTags([]string{strings.Repeat("a", 65)}); got[0] != strings.Repeat("a", 64) {
		t.Errorf("a 65-character tag was not cut to 64: %d characters", len([]rune(got[0])))
	}
}

// A hand-made results file can hold more than the limits allow; the writer still stops at the cap, and writes in key order.
func TestReportPropertiesStopAtTheCapAndAreSortedByKey(t *testing.T) {
	m := map[string]string{}
	for i := 0; i < 40; i++ {
		m[fmt.Sprintf("key%02d", i)] = "v"
	}
	got := runner.ReportProperties(m)
	if len(got) != runner.MaxProperties {
		t.Fatalf("%d properties written, want %d", len(got), runner.MaxProperties)
	}
	for i, p := range got {
		if want := fmt.Sprintf("key%02d", i); p.Name != want {
			t.Fatalf("property %d is %s, want %s", i, p.Name, want)
		}
	}
	// every writer keeps that order: JUnit after run_label, JSON as an object (encoded in key order)
	props := map[string]string{"zeta": "1", "alpha": "2", "mid": "3"}
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJUnit, &buf, false)
	rep.SetJUnitOptions(runner.JUnitOptions{Label: "lbl"})
	rep.SetProperties(props)
	if err := rep.Report([]runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1}}); err != nil {
		t.Fatal(err)
	}
	x := buf.String()
	order := []string{`name="run_label"`, `name="alpha"`, `name="mid"`, `name="zeta"`}
	last := -1
	for _, o := range order {
		i := strings.Index(x, o)
		if i < 0 || i < last {
			t.Fatalf("%s is out of order or missing in:\n%s", o, x)
		}
		last = i
	}
	buf.Reset()
	rep = runner.NewReporter(runner.FormatJSON, &buf, false)
	rep.SetProperties(props)
	if err := rep.Report([]runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1}}); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties map[string]string `json:"properties"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil || len(doc.Properties) != 3 {
		t.Fatalf("json properties: %v %v", doc.Properties, err)
	}
	if i, j := strings.Index(buf.String(), `"alpha"`), strings.Index(buf.String(), `"zeta"`); i < 0 || j < i {
		t.Errorf("json properties are not in key order:\n%s", buf.String())
	}
}
