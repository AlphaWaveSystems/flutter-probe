package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// crashFolderName is the folder of one attempt of one test: the test's name reduced to ASCII letters and digits (a name with none, such as a Japanese one, gives
// "test"), a dash and eight hex digits of a hash of the file, the full result name (the row included) and the attempt. The hash keeps apart tests whose names
// reduce to the same text, names that differ only in case (a case-insensitive disk would merge them), the rows of a data-driven test and the attempts of a retry; it
// also means the name is never one Windows reserves (CON, NUL, ...), which are only reserved without a suffix. It is never a path.
func crashFolderName(file, name string, attempt int) string {
	base := sanitizeName(name)
	if base == "" {
		base = "test"
	}
	sum := sha256.Sum256([]byte(file + "\x00" + name + "\x00" + strconv.Itoa(attempt)))
	return base + "-" + hex.EncodeToString(sum[:4])
}

// writeCrashFiles writes the evidence of one failed test attempt under dir/<folder>/ (see crashFolderName) and returns the paths it wrote: exit.txt (one line per exit entry of the app) and, when
// the crash buffer held anything, crash.txt (its lines). Everything written is cleaned line by line. An error leaves whatever was already written in place and is
// returned for the caller to print; the report then simply lists no files.
func writeCrashFiles(dir, file, name string, attempt int, c *CrashReport) ([]string, error) {
	if c == nil {
		return nil, nil
	}
	base, err := filepath.Abs(filepath.Join(dir, crashFolderName(file, name, attempt)))
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
func (r *Runner) writeCrashFilesFor(c *CrashReport, file, name string, attempt int) {
	if c == nil {
		return
	}
	dir := crashDefaultDir
	if r.cfg != nil && r.cfg.Reports != "" {
		dir = filepath.Join(r.cfg.Reports, "crash")
	}
	files, err := writeCrashFiles(dir, file, name, attempt, c)
	if err != nil {
		fmt.Printf("    \033[33m⚠\033[0m  crash evidence files: %v\n", err)
	}
	c.Files = files
}
