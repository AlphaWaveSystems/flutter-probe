package runner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Format is a report output format.
type Format string

const (
	FormatTerminal Format = "terminal"
	FormatJUnit    Format = "junit"
	FormatJSON     Format = "json"
)

// RunMetadata contains device, app, and environment info for report traceability.
type RunMetadata struct {
	DeviceName   string `json:"device_name"`
	DeviceID     string `json:"device_id"`
	Platform     string `json:"platform"`      // "ios" or "android"
	OSVersion    string `json:"os_version"`     // "iOS 18.6" or "Android 14"
	Provider     string `json:"provider"`       // "local", "browserstack", "firebase", etc.
	AppID        string `json:"app_id"`         // bundle ID / package name
	AppVersion   string `json:"app_version"`    // e.g. "1.2.16"
	ProbeVersion string `json:"probe_version"`  // FlutterProbe CLI version
	ConfigFile   string `json:"config_file"`    // which probe.yaml was used
}

// Reporter writes test results to various output formats.
type Reporter struct {
	format    Format
	out       io.Writer
	verbose   bool
	outputDir string // directory of the output file, used to relativize artifact paths
	metadata  *RunMetadata
	streaming bool // when true, StreamResult emits one JSON line per result as tests complete
}

// NewReporter creates a reporter writing to the given writer.
func NewReporter(format Format, out io.Writer, verbose bool) *Reporter {
	return &Reporter{format: format, out: out, verbose: verbose}
}

// NewFileReporter creates a reporter writing to a file.
func NewFileReporter(format Format, path string, verbose bool) (*Reporter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	absDir, _ := filepath.Abs(filepath.Dir(path))
	return &Reporter{format: format, out: f, verbose: verbose, outputDir: absDir}, nil
}

// SetMetadata attaches run metadata that will be included in JSON reports.
func (r *Reporter) SetMetadata(m RunMetadata) {
	r.metadata = &m
}

// SetStreaming enables newline-delimited JSON event emission during a run.
// When true, callers can invoke StreamResult after each test completes to
// emit a single JSON line on the reporter's output. The final Report() call
// still writes the full report. Streaming is JSON-format only; calling this
// with a non-JSON reporter is a no-op.
func (r *Reporter) SetStreaming(on bool) {
	r.streaming = on && r.format == FormatJSON
}

// StreamResult writes one newline-delimited JSON event for a completed test.
// No-op unless streaming is enabled. Safe to call from a goroutine that owns
// the result; the underlying writer is expected to be thread-safe (os.Stdout
// and os.Stderr are; *os.File generally is for line-sized writes on POSIX).
func (r *Reporter) StreamResult(res TestResult) {
	if !r.streaming {
		return
	}
	jr := jsonResult{
		Name:       res.TestName,
		File:       res.File,
		Passed:     res.Passed,
		Skipped:    res.Skipped,
		Duration:   float64(res.Duration.Milliseconds()),
		Row:        res.Row,
		DeviceID:   res.DeviceID,
		DeviceName: res.DeviceName,
	}
	for _, art := range res.Artifacts {
		if r.outputDir != "" && filepath.IsAbs(art) {
			if rel, err := filepath.Rel(r.outputDir, art); err == nil {
				jr.Artifacts = append(jr.Artifacts, rel)
				continue
			}
		}
		jr.Artifacts = append(jr.Artifacts, art)
	}
	if res.Error != nil {
		jr.Error = res.Error.Error()
	}
	evt := struct {
		Type   string     `json:"type"`
		Result jsonResult `json:"result"`
	}{Type: "test_result", Result: jr}
	// Manual encoding to ensure single-line output (no SetIndent).
	b, err := json.Marshal(evt)
	if err != nil {
		return
	}
	_, _ = r.out.Write(append(b, '\n'))
}

// Report writes results to the output.
func (r *Reporter) Report(results []TestResult) error {
	switch r.format {
	case FormatJUnit:
		return r.writeJUnit(results)
	case FormatJSON:
		return r.writeJSON(results)
	default:
		r.writeTerminal(results)
		return nil
	}
}

// ---- Terminal ----

func (r *Reporter) writeTerminal(results []TestResult) {
	passed, failed, skipped := 0, 0, 0
	total := time.Duration(0)

	for _, res := range results {
		total += res.Duration
		switch {
		case res.Skipped:
			skipped++
			if r.verbose {
				fmt.Fprintf(r.out, "  ⟳  %s\n", res.TestName)
			}
		case res.Passed:
			passed++
			fmt.Fprintf(r.out, "  \033[32m✓\033[0m  %s \033[2m(%s)\033[0m\n", res.TestName, res.Duration.Round(time.Millisecond))
		default:
			failed++
			fmt.Fprintf(r.out, "  \033[31m✗\033[0m  %s \033[2m(%s)\033[0m\n", res.TestName, res.Duration.Round(time.Millisecond))
			fmt.Fprintf(r.out, "       \033[31m%s\033[0m\n", res.Error)
		}
	}

	fmt.Fprintln(r.out)
	line := fmt.Sprintf("  %d passed", passed)
	if failed > 0 {
		line += fmt.Sprintf(", \033[31m%d failed\033[0m", failed)
	}
	if skipped > 0 {
		line += fmt.Sprintf(", %d skipped", skipped)
	}
	line += fmt.Sprintf("  (%s)", total.Round(time.Millisecond))
	fmt.Fprintln(r.out, line)
	fmt.Fprintln(r.out)
}

// PrintTestStart prints a test start line (real-time streaming).
func (r *Reporter) PrintTestStart(name string) {
	if r.format == FormatTerminal {
		fmt.Fprintf(r.out, "  \033[2m▸\033[0m  %s\n", name)
	}
}

// PrintStep prints a single step execution (verbose mode).
func (r *Reporter) PrintStep(step string) {
	if r.format == FormatTerminal && r.verbose {
		fmt.Fprintf(r.out, "        \033[2m%s\033[0m\n", step)
	}
}

// ---- JUnit XML ----

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Time     string      `xml:"time,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string       `xml:"name,attr"`
	Classname string       `xml:"classname,attr"`
	Time      string       `xml:"time,attr"`
	Failure   *junitFail   `xml:"failure,omitempty"`
	Skipped   *junitSkip   `xml:"skipped,omitempty"`
}

type junitFail struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

type junitSkip struct{}

func (r *Reporter) writeJUnit(results []TestResult) error {
	// Group by file
	suiteMap := make(map[string]*junitSuite)
	for _, res := range results {
		s, ok := suiteMap[res.File]
		if !ok {
			s = &junitSuite{Name: res.File}
			suiteMap[res.File] = s
		}
		s.Tests++
		c := junitCase{
			Name:      res.TestName,
			Classname: res.File,
			Time:      fmt.Sprintf("%.3f", res.Duration.Seconds()),
		}
		if res.Skipped {
			s.Skipped++
			c.Skipped = &junitSkip{}
		} else if !res.Passed {
			s.Failures++
			c.Failure = &junitFail{
				Message: res.Error.Error(),
				Text:    res.Error.Error(),
			}
		}
		s.Cases = append(s.Cases, c)
	}

	var suites junitSuites
	for _, s := range suiteMap {
		suites.Suites = append(suites.Suites, *s)
	}

	enc := xml.NewEncoder(r.out)
	enc.Indent("", "  ")
	if _, err := io.WriteString(r.out, xml.Header); err != nil {
		return err
	}
	return enc.Encode(suites)
}

// ---- JSON ----

type jsonResult struct {
	Name       string   `json:"name"`
	File       string   `json:"file"`
	Passed     bool     `json:"passed"`
	Skipped    bool     `json:"skipped"`
	Duration   float64  `json:"duration_ms"`
	Error      string   `json:"error,omitempty"`
	Row        int      `json:"row,omitempty"`
	Artifacts  []string `json:"artifacts,omitempty"`
	DeviceID   string   `json:"device_id,omitempty"`
	DeviceName string   `json:"device_name,omitempty"`
}

type jsonReport struct {
	GeneratedAt string       `json:"generated_at"`
	Metadata    *RunMetadata `json:"metadata,omitempty"`
	TotalTests  int          `json:"total_tests"`
	Passed      int          `json:"passed"`
	Failed      int          `json:"failed"`
	Skipped     int          `json:"skipped"`
	Results     []jsonResult `json:"results"`
}

func (r *Reporter) writeJSON(results []TestResult) error {
	report := jsonReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Metadata:    r.metadata,
		TotalTests:  len(results),
	}
	for _, res := range results {
		jr := jsonResult{
			Name:       res.TestName,
			File:       res.File,
			Passed:     res.Passed,
			Skipped:    res.Skipped,
			Duration:   float64(res.Duration.Milliseconds()),
			Row:        res.Row,
			DeviceID:   res.DeviceID,
			DeviceName: res.DeviceName,
		}
		// Convert artifact paths to relative paths for portability (CI/CD)
		for _, art := range res.Artifacts {
			if r.outputDir != "" && filepath.IsAbs(art) {
				if rel, err := filepath.Rel(r.outputDir, art); err == nil {
					jr.Artifacts = append(jr.Artifacts, rel)
					continue
				}
			}
			jr.Artifacts = append(jr.Artifacts, art)
		}
		if res.Error != nil {
			jr.Error = res.Error.Error()
		}
		switch {
		case res.Skipped:
			report.Skipped++
		case res.Passed:
			report.Passed++
		default:
			report.Failed++
		}
		report.Results = append(report.Results, jr)
	}
	enc := json.NewEncoder(r.out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// ---- Summary ----

// Summary returns a one-line summary string.
func Summary(results []TestResult) string {
	passed, failed, skipped := 0, 0, 0
	for _, r := range results {
		switch {
		case r.Skipped:
			skipped++
		case r.Passed:
			passed++
		default:
			failed++
		}
	}
	parts := []string{fmt.Sprintf("%d passed", passed)}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	return strings.Join(parts, ", ")
}

// AllPassed returns true if every result passed.
func AllPassed(results []TestResult) bool {
	for _, r := range results {
		if !r.Passed && !r.Skipped {
			return false
		}
	}
	return true
}

// ---- Real-time step feedback ----

// isTTY reports whether os.Stdout is an interactive terminal. The result is
// computed once (via sync.OnceValue) and cached — no repeated syscalls.
// Returns false in CI pipes, when stdout is redirected to a file, etc.
var isTTY = sync.OnceValue(func() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
})

// printStepBeforeW writes the "→ desc" pre-step line to w.
// On a TTY the line will be overwritten by printStepAfterNW (when tickLines==0).
func printStepBeforeW(w io.Writer, _ bool, depth int, desc string) {
	if desc == "" {
		return
	}
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(w, "    %s\033[2m→\033[0m  %s\n", indent, desc)
}

// printStepAfterNW writes the ✓/✗ result line.
// On a TTY with no tick lines printed below the "→" line, it moves the cursor
// up one line and overwrites in place (clean single-line-per-step output).
// When tickLines > 0 the "→" line has scrolled above the tick lines, so we
// just append — attempting to erase N lines is more fragile than helpful.
// On non-TTY output is always appended.
func printStepAfterNW(w io.Writer, tty bool, depth int, desc string, elapsed time.Duration, err error, tickLines int) {
	if desc == "" {
		return
	}
	indent := strings.Repeat("  ", depth)
	status := "\033[32m✓\033[0m"
	if err != nil {
		status = "\033[31m✗\033[0m"
	}
	line := fmt.Sprintf("    %s%s %s \033[2m(%.1fs)\033[0m", indent, status, desc, elapsed.Seconds())
	if tty && tickLines == 0 {
		// Cursor up one line, carriage return, erase to EOL, then print result.
		fmt.Fprintf(w, "\033[1A\r\033[K%s\n", line)
	} else {
		fmt.Fprintln(w, line)
	}
}

// printStepTickW writes a ⏱ progress line showing elapsed time.
func printStepTickW(w io.Writer, depth int, desc string, elapsed time.Duration) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(w, "    %s\033[33m⏱\033[0m  %s... \033[2m(%ds)\033[0m\n",
		indent, desc, int(elapsed.Seconds()))
}

// printStepWarningW writes a one-time ⚠ warning when a step is near its timeout.
func printStepWarningW(w io.Writer, depth int, desc string, elapsed, timeout time.Duration) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(w, "    %s\033[33m⚠\033[0m  %s still running — %ds elapsed, %ds timeout\n",
		indent, desc, int(elapsed.Seconds()), int(timeout.Seconds()))
}

// printCurrentStepW writes a transient \r-overwriting status line in non-verbose
// mode. On non-TTY this is a no-op so CI output stays clean.
func printCurrentStepW(w io.Writer, tty bool, desc string) {
	if !tty || desc == "" {
		return
	}
	const maxW = 80
	label := "  \033[2m" + desc + "\033[0m"
	// Pad to maxW so previous (longer) status is fully erased.
	fmt.Fprintf(w, "\r%-*s", maxW, label)
}

// clearCurrentStepW erases the transient status line written by printCurrentStepW.
func clearCurrentStepW(w io.Writer, tty bool) {
	if !tty {
		return
	}
	fmt.Fprintf(w, "\r%*s\r", 80, "")
}

// Public wrappers — called from executor.go. Each delegates to the testable
// helper with os.Stdout and the cached isTTY result.

// PrintStepBefore prints the "→ desc" line before a step runs (verbose mode).
func PrintStepBefore(depth int, desc string) {
	printStepBeforeW(os.Stdout, isTTY(), depth, desc)
}

// PrintStepAfterN prints the ✓/✗ result line after a step finishes.
// tickLines is the number of ⏱/⚠ lines printed below the "→" line.
func PrintStepAfterN(depth int, desc string, elapsed time.Duration, err error, tickLines int) {
	printStepAfterNW(os.Stdout, isTTY(), depth, desc, elapsed, err, tickLines)
}

// PrintStepTick prints a ⏱ progress line during a long-running step.
func PrintStepTick(depth int, desc string, elapsed time.Duration) {
	printStepTickW(os.Stdout, depth, desc, elapsed)
}

// PrintStepWarning prints a one-time ⚠ warning when a step is near its timeout.
func PrintStepWarning(depth int, desc string, elapsed, timeout time.Duration) {
	printStepWarningW(os.Stdout, depth, desc, elapsed, timeout)
}

// PrintCurrentStep prints a transient \r-based status line (non-verbose, TTY only).
func PrintCurrentStep(desc string) {
	printCurrentStepW(os.Stdout, isTTY(), desc)
}

// ClearCurrentStep erases the transient status line (non-verbose, TTY only).
func ClearCurrentStep() {
	clearCurrentStepW(os.Stdout, isTTY())
}
