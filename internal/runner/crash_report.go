package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// The crash evidence of a failed test in the reports (docs/proposals/crash-anr-capture.md, slice S3). The collector already masked and capped the text; what is added
// here is the last cleaning before anything is written (control, bidi and hidden characters become spaces), the files in the output folder, and the object in the
// JSON report. A run that did not ask for crash evidence has no CrashReport, so nothing here runs and the reports are byte for byte what they were.

const (
	crashFieldRunes = 400 // one cleaned line of the files and of the report object
	crashDefaultDir = "reports/crash"
)

// jsonCrash is the optional "crash" object of a test result in the JSON report. Every string is one line of plain text.
type jsonCrash struct {
	Kind        string   `json:"kind"`
	Gone        bool     `json:"gone"`
	PID         int      `json:"pid,omitempty"`
	Process     string   `json:"process,omitempty"`
	ExitTime    string   `json:"exit_time,omitempty"` // the device's wall clock as it printed it, no zone
	Summary     string   `json:"summary,omitempty"`
	Description string   `json:"exit_description,omitempty"`
	Truncated   bool     `json:"truncated,omitempty"`
	Files       []string `json:"files,omitempty"` // relative to the report when they are inside its folder
}

// crashJSON is the report object of c, or nil when there is no evidence. outputDir makes file paths relative, as the report does for artifacts.
func crashJSON(c *CrashReport, outputDir string) *jsonCrash {
	if c == nil {
		return nil
	}
	j := &jsonCrash{
		Kind:      device.CleanOneLine(string(c.Kind), 40),
		Gone:      c.Gone,
		Summary:   device.CleanOneLine(c.Evidence.Summary, crashFieldRunes),
		Truncated: c.Evidence.Truncated,
	}
	if x := c.Evidence.Exit; x != nil {
		j.PID = x.PID
		j.Process = device.CleanOneLine(x.Process, crashFieldRunes)
		j.ExitTime = x.WallClock.Format("2006-01-02 15:04:05.000")
		j.Description = device.CleanOneLine(x.Description, crashFieldRunes)
	}
	for _, f := range c.Files {
		if outputDir != "" && filepath.IsAbs(f) {
			if rel, err := filepath.Rel(outputDir, f); err == nil && !strings.HasPrefix(rel, "..") {
				j.Files = append(j.Files, rel)
				continue
			}
		}
		j.Files = append(j.Files, f)
	}
	return j
}

// crashFileName is a folder name made of the test's name: letters and digits only, never empty, never a path.
func crashFileName(testName string) string {
	if s := sanitizeName(testName); s != "" {
		return s
	}
	return "test"
}

// writeCrashFiles writes the evidence of one failed test under dir/<test>/ and returns the paths it wrote: exit.txt (one line per exit entry of the app) and, when
// the crash buffer held anything, crash.txt (its lines). Everything written is cleaned line by line. An error leaves whatever was already written in place and is
// returned for the caller to print; the report then simply lists no files.
func writeCrashFiles(dir, testName string, c *CrashReport) ([]string, error) {
	if c == nil {
		return nil, nil
	}
	base, err := filepath.Abs(filepath.Join(dir, crashFileName(testName)))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	var written []string
	var exits strings.Builder
	for _, x := range c.Evidence.Exits {
		fmt.Fprintf(&exits, "%s pid=%d kind=%s status=%d process=%s description=%s\n", x.WallClock.Format("2006-01-02 15:04:05.000"), x.PID,
			device.CleanOneLine(string(x.Kind), 40), x.Status, device.CleanOneLine(x.Process, crashFieldRunes), device.CleanOneLine(x.Description, crashFieldRunes))
	}
	if exits.Len() > 0 {
		p := filepath.Join(base, "exit.txt")
		if err := os.WriteFile(p, []byte(exits.String()), 0o644); err != nil {
			return written, err
		}
		written = append(written, p)
	}
	if len(c.Evidence.CrashLines) > 0 {
		var b strings.Builder
		if c.Evidence.Summary != "" {
			b.WriteString("# " + device.CleanOneLine(c.Evidence.Summary, crashFieldRunes) + "\n")
		}
		for _, l := range c.Evidence.CrashLines {
			b.WriteString(device.CleanOneLine(l, crashFieldRunes) + "\n")
		}
		if c.Evidence.Truncated {
			b.WriteString("# (cut: more text was left out)\n")
		}
		p := filepath.Join(base, "crash.txt")
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			return written, err
		}
		written = append(written, p)
	}
	return written, nil
}

// writeCrashFilesFor writes the files of a failed test's crash evidence under <reports folder>/crash and records them on the report. A failure to write is
// printed as a warning and the test result is not changed by it.
func (r *Runner) writeCrashFilesFor(c *CrashReport, testName string) {
	if c == nil {
		return
	}
	dir := crashDefaultDir
	if r.cfg != nil && r.cfg.Reports != "" {
		dir = filepath.Join(r.cfg.Reports, "crash")
	}
	files, err := writeCrashFiles(dir, testName, c)
	if err != nil {
		fmt.Printf("    \033[33m⚠\033[0m  crash evidence files: %v\n", err)
	}
	c.Files = files
}
