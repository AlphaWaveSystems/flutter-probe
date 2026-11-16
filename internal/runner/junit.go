package runner

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// JUnitOptions tunes `--format junit`. The zero value gives the default output.
type JUnitOptions struct {
	// Attempts writes one <testcase> per attempt of a test that was re-run (retry_failed_tests): the failed
	// attempts as <failure> or <error>, the last one with its outcome, so a flaky test is visible to tools that
	// look for a failure and a pass of the same test. Off by default: the extra failed cases change the
	// counts a CI summary shows for a run that finally passed.
	Attempts bool
	// ClassSuffix is appended to every classname (for example "-ios") so the same test run on several devices
	// has distinct identities in tools that key on classname and name.
	ClassSuffix string
	// Label is written as the run_label property of every suite.
	Label string
	// Base is the directory paths are made relative to (default: the working directory).
	Base string
	// Artifacts writes the artifacts of each test (screenshots, the diff image of a failed `compare screenshot`, videos) as lines of the testcase's
	// <system-out>: "artifact: <path>", one per artifact, at most maxJUnitArtifacts per testcase, a path at most maxJUnitArtifactPath characters. A path inside
	// ArtifactBase is written relative to it; a path outside it is written as its file name only, so no absolute path of the machine that ran the tests
	// reaches a report that CI uploads. A testcase with no artifact gets no <system-out>. Off by default: an existing report stays byte-identical.
	Artifacts bool
	// ArtifactBase is the directory the artifact paths are made relative to: the directory of the report file when it is written to one, else Base.
	ArtifactBase string
}

// Limits of the artifact lines of one testcase.
const (
	maxJUnitArtifacts    = 50
	maxJUnitArtifactPath = 300
)

// SetJUnitOptions configures the JUnit writer.
func (r *Reporter) SetJUnitOptions(o JUnitOptions) { r.junit = o }

type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Errors   int          `xml:"errors,attr"`
	Skipped  int          `xml:"skipped,attr"`
	Time     string       `xml:"time,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name       string      `xml:"name,attr"`
	File       string      `xml:"file,attr"`
	Tests      int         `xml:"tests,attr"`
	Failures   int         `xml:"failures,attr"`
	Errors     int         `xml:"errors,attr"`
	Skipped    int         `xml:"skipped,attr"`
	Time       string      `xml:"time,attr"`
	Timestamp  string      `xml:"timestamp,attr,omitempty"`
	Properties *junitProps `xml:"properties,omitempty"`
	Cases      []junitCase `xml:"testcase"`

	seconds float64
	started time.Time
}

type junitCase struct {
	Name       string      `xml:"name,attr"`
	Classname  string      `xml:"classname,attr"`
	File       string      `xml:"file,attr"`
	Time       string      `xml:"time,attr"`
	Properties *junitProps `xml:"properties,omitempty"`
	Failure    *junitFail  `xml:"failure,omitempty"`
	Error      *junitFail  `xml:"error,omitempty"`
	Skipped    *junitSkip  `xml:"skipped,omitempty"`
	SystemOut  string      `xml:"system-out,omitempty"`
}

type junitProps struct {
	Props []junitProp `xml:"property"`
}

type junitProp struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type junitFail struct {
	Type    string `xml:"type,attr,omitempty"`
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

type junitSkip struct{}

// junitPath makes a path stable across machines: relative to base, with forward slashes. A path outside base
// keeps only its file name, so a home directory never ends up in the report.
func junitPath(base, p string) string {
	if p == "" {
		return p
	}
	if base == "" {
		base, _ = os.Getwd()
	}
	if filepath.IsAbs(p) {
		if rel, err := filepath.Rel(base, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			p = rel
		} else {
			p = filepath.Base(p)
		}
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// infraError marks an error of the test environment whatever its text, so that JUnit reports it as <error>.
type infraError struct{ err error }

func (e infraError) Error() string { return e.err.Error() }
func (e infraError) Unwrap() error { return e.err }

// isInfraError tells an error of the test environment (the agent connection, a setup hook) from an
// assertion that failed: JUnit calls the first an <error> and the second a <failure>.
func isInfraError(err error) bool {
	if err == nil {
		return false
	}
	var ie infraError
	return errors.As(err, &ie) || isConnectionError(err) || strings.HasPrefix(err.Error(), "before all:") || strings.HasPrefix(err.Error(), "before each:")
}

// ReportConnectFailure writes a JUnit report for a run that failed before any test ran because the agent could
// not be reached. It has one <error> testcase per planned test file, carrying the message of cause.
func (r *Reporter) ReportConnectFailure(files []string, cause error) error {
	results := make([]TestResult, 0, len(files))
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		results = append(results, TestResult{TestName: name, File: f, Row: -1, Error: infraError{cause}})
	}
	return r.writeJUnit(results)
}

func secs(d time.Duration) float64 { return d.Seconds() }

func fmtSecs(f float64) string { return fmt.Sprintf("%.3f", f) }

func (r *Reporter) writeJUnit(results []TestResult) error {
	o := r.junit
	suiteMap := map[string]*junitSuite{}
	var order []string
	for _, res := range results {
		path := junitPath(o.Base, res.File)
		s, ok := suiteMap[path]
		if !ok {
			s = &junitSuite{Name: path, File: path}
			suiteMap[path] = s
			order = append(order, path)
		}
		for _, c := range r.junitCases(res, path) {
			s.Cases = append(s.Cases, c.c)
			s.Tests++
			s.seconds += c.seconds
			switch {
			case c.c.Skipped != nil:
				s.Skipped++
			case c.c.Error != nil:
				s.Errors++
			case c.c.Failure != nil:
				s.Failures++
			}
		}
		if !res.StartedAt.IsZero() && (s.started.IsZero() || res.StartedAt.Before(s.started)) {
			s.started = res.StartedAt
		}
	}
	sort.Strings(order)

	var out junitSuites
	for _, name := range order {
		s := suiteMap[name]
		s.Time = fmtSecs(s.seconds)
		if !s.started.IsZero() {
			s.Timestamp = s.started.UTC().Format("2006-01-02T15:04:05")
		}
		s.Properties = r.junitSuiteProps()
		out.Tests += s.Tests
		out.Failures += s.Failures
		out.Errors += s.Errors
		out.Skipped += s.Skipped
		out.Suites = append(out.Suites, *s)
	}
	total := 0.0
	for _, s := range out.Suites {
		var f float64
		fmt.Sscanf(s.Time, "%f", &f)
		total += f
	}
	out.Time = fmtSecs(total)

	enc := xml.NewEncoder(r.out)
	enc.Indent("", "  ")
	if _, err := io.WriteString(r.out, xml.Header); err != nil {
		return err
	}
	return enc.Encode(out)
}

type timedCase struct {
	c       junitCase
	seconds float64
}

// junitCases returns the <testcase> elements of one result: one, or one per attempt with Attempts on.
func (r *Reporter) junitCases(res TestResult, path string) []timedCase {
	o := r.junit
	base := junitCase{Name: res.TestName, Classname: path + o.ClassSuffix, File: path}
	var props []junitProp
	if res.Row >= 0 && strings.Contains(res.TestName, "[row ") {
		props = append(props, junitProp{"row", fmt.Sprint(res.Row + 1)})
	}
	if res.DeviceName != "" {
		props = append(props, junitProp{"device", res.DeviceName})
	}
	if tags := ReportTags(res.Tags); len(tags) > 0 {
		props = append(props, junitProp{"tags", strings.Join(tags, ",")})
	}

	mk := func(passed, skipped bool, err error, d time.Duration, extra ...junitProp) timedCase {
		c := base
		c.Time = fmtSecs(secs(d))
		all := append(append([]junitProp{}, props...), extra...)
		if len(all) > 0 {
			c.Properties = &junitProps{Props: all}
		}
		switch {
		case skipped:
			c.Skipped = &junitSkip{}
		case !passed:
			msg := "test failed" // a failed result without an error must not crash the report
			if err != nil {
				msg = err.Error()
			}
			if isInfraError(err) {
				c.Error = &junitFail{Type: "error", Message: msg, Text: msg}
			} else {
				c.Failure = &junitFail{Message: msg, Text: msg}
			}
		}
		return timedCase{c: c, seconds: secs(d)}
	}

	if o.Attempts && len(res.AttemptLog) > 1 {
		var out []timedCase
		for i, a := range res.AttemptLog {
			passed, skipped := a.Passed, false
			if i == len(res.AttemptLog)-1 { // the last attempt carries the final outcome
				passed, skipped = res.Passed, res.Skipped
			}
			out = append(out, mk(passed, skipped, a.Error, a.Duration, junitProp{"attempt", fmt.Sprintf("%d/%d", i+1, len(res.AttemptLog))}))
		}
		// the artifacts of all attempts belong to the case that carries the final outcome
		out[len(out)-1].c.SystemOut = o.artifactLines(res.Artifacts)
		return out
	}
	var extra []junitProp
	if res.Attempts > 1 {
		extra = append(extra, junitProp{"attempts", fmt.Sprint(res.Attempts)})
	}
	one := mk(res.Passed, res.Skipped, res.Error, res.Duration, extra...)
	one.c.SystemOut = o.artifactLines(res.Artifacts)
	return []timedCase{one}
}

// artifactLines is the <system-out> text for the artifacts of one test, or "" when none is to be written (the option is off, or there are none).
func (o JUnitOptions) artifactLines(artifacts []string) string {
	if !o.Artifacts || len(artifacts) == 0 {
		return ""
	}
	base := o.ArtifactBase
	if base == "" {
		base = o.Base
	}
	var lines []string
	seen := map[string]bool{}
	listed, extra := 0, 0
	for _, a := range artifacts {
		p := artifactPath(base, a)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if listed == maxJUnitArtifacts {
			extra++
			continue
		}
		listed++
		lines = append(lines, "artifact: "+p)
	}
	if extra > 0 {
		lines = append(lines, fmt.Sprintf("artifacts: %d more not listed", extra))
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}

// artifactPath is the path of an artifact as a report shows it: relative to base when it is inside it, otherwise the file name alone (never an absolute
// path, and never a path that climbs out with ".."), masked for the secrets the run knows, made safe to write, and cut at maxJUnitArtifactPath characters.
func artifactPath(base, p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	q := junitPath(base, p)
	windowsAbs := len(q) > 2 && q[1] == ':' && (q[2] == '\\' || q[2] == '/') || strings.HasPrefix(q, `\\`)
	if q == ".." || strings.HasPrefix(q, "../") || strings.HasPrefix(q, "/") || windowsAbs {
		if i := strings.LastIndexAny(q, `/\`); i >= 0 {
			q = q[i+1:] // the file name alone, whichever separator the machine that wrote the path used
		}
	}
	q = reportText(q)
	if r := []rune(q); len(r) > maxJUnitArtifactPath {
		q = string(r[:maxJUnitArtifactPath])
	}
	return strings.TrimSpace(q)
}

func (r *Reporter) junitSuiteProps() *junitProps {
	var p []junitProp
	add := func(k, v string) {
		if v != "" {
			p = append(p, junitProp{k, v})
		}
	}
	if m := r.metadata; m != nil {
		add("probe_version", m.ProbeVersion)
		add("agent_version", m.AgentVersion)
		add("platform", m.Platform)
		add("os_version", m.OSVersion)
		add("app_id", m.AppID)
		add("app_version", m.AppVersion)
		add("device_name", m.DeviceName)
		add("device_class", m.DeviceClass)
		add("provider", m.Provider)
		add("locale", m.Locale)
	}
	add("run_label", r.junit.Label)
	for _, kv := range ReportProperties(r.properties) {
		p = append(p, junitProp(kv))
	}
	if len(p) == 0 {
		return nil
	}
	return &junitProps{Props: p}
}
