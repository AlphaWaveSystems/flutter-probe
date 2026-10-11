package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/locale"
)

// runLocaleMatrix runs the whole `probe test` invocation once per language of
// --locales, each in its own process with --locale <tag> (a fresh connection,
// app launch and report per language), and prints a per-language summary.
// A report file given with -o/--output gets the tag in its name (report.de.json).
func runLocaleMatrix(cmd *cobra.Command, raw string) error {
	tags, err := locale.ParseList(raw)
	if err != nil {
		return fmt.Errorf("--locales: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("--locales: %w", err)
	}
	base := stripFlag(os.Args[1:], "--locales", "")

	type row struct {
		tag     string
		ok      bool
		took    time.Duration
		details string
	}
	var rows []row
	for _, t := range tags {
		label := t.BCP47
		if t.System {
			label = "system"
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "\n\033[1m── locale %s ──\033[0m\n", label)
		args := withLocale(base, label)
		args = suffixOutput(args, label)
		start := time.Now()
		child := exec.Command(exe, args...)
		child.Stdout, child.Stderr, child.Stdin = os.Stdout, os.Stderr, os.Stdin
		runErr := child.Run()
		r := row{tag: label, ok: runErr == nil, took: time.Since(start)}
		if runErr != nil {
			r.details = runErr.Error()
		}
		rows = append(rows, r)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "\n  %-12s %-8s %s\n", "LOCALE", "RESULT", "TIME")
	failed := 0
	for _, r := range rows {
		res := "\033[32mpass\033[0m    "
		if !r.ok {
			res = "\033[31mfail\033[0m    "
			failed++
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "  %-12s %s %s\n", r.tag, res, r.took.Round(time.Second))
	}
	if failed > 0 {
		return fmt.Errorf("%w: %d of %d locales failed", errTestFailed, failed, len(rows))
	}
	return nil
}

// stripFlag removes "--name value" / "--name=value" from args.
func stripFlag(args []string, name, short string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == name || (short != "" && a == short):
			i++ // skip the value
		case strings.HasPrefix(a, name+"="):
		default:
			out = append(out, a)
		}
	}
	return out
}

// withLocale returns args with "--locale tag" replacing any earlier --locale.
func withLocale(args []string, tag string) []string {
	return append(stripFlag(args, "--locale", ""), "--locale", tag)
}

// suffixOutput inserts ".<tag>" before the extension of the -o/--output value.
func suffixOutput(args []string, tag string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out); i++ {
		switch {
		case (out[i] == "-o" || out[i] == "--output") && i+1 < len(out):
			out[i+1] = tagPath(out[i+1], tag)
		case strings.HasPrefix(out[i], "--output="):
			out[i] = "--output=" + tagPath(strings.TrimPrefix(out[i], "--output="), tag)
		}
	}
	return out
}

func tagPath(p, tag string) string {
	ext := filepath.Ext(p)
	return strings.TrimSuffix(p, ext) + "." + tag + ext
}
