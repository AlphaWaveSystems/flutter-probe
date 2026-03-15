package runner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Format is a report output format.
type Format string

const (
	FormatTerminal Format = "terminal"
	FormatJUnit    Format = "junit"
	FormatJSON     Format = "json"
)

// Reporter writes test results to various output formats.
type Reporter struct {
	format  Format
	out     io.Writer
	verbose bool
}

// NewReporter creates a reporter writing to the given writer.
func NewReporter(format Format, out io.Writer, verbose bool) *Reporter {
	return &Reporter{format: format, out: out, verbose: verbose}
}

// NewFileReporter creates a reporter writing to a file.
func NewFileReporter(format Format, path string, verbose bool) (*Reporter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return NewReporter(format, f, verbose), nil
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
	Name      string   `json:"name"`
	File      string   `json:"file"`
	Passed    bool     `json:"passed"`
	Skipped   bool     `json:"skipped"`
	Duration  float64  `json:"duration_ms"`
	Error     string   `json:"error,omitempty"`
	Row       int      `json:"row,omitempty"`
	Artifacts []string `json:"artifacts,omitempty"`
}

type jsonReport struct {
	GeneratedAt string       `json:"generated_at"`
	TotalTests  int          `json:"total_tests"`
	Passed      int          `json:"passed"`
	Failed      int          `json:"failed"`
	Skipped     int          `json:"skipped"`
	Results     []jsonResult `json:"results"`
}

func (r *Reporter) writeJSON(results []TestResult) error {
	report := jsonReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		TotalTests:  len(results),
	}
	for _, res := range results {
		jr := jsonResult{
			Name:      res.TestName,
			File:      res.File,
			Passed:    res.Passed,
			Skipped:   res.Skipped,
			Duration:  float64(res.Duration.Milliseconds()),
			Row:       res.Row,
			Artifacts: res.Artifacts,
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
