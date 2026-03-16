// Package ui provides terminal output helpers with ANSI colors and spinners.
package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ANSI color codes.
const (
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Bold   = "\033[1m"
	Dim    = "\033[2m"
	Gray   = "\033[90m"
	Reset  = "\033[0m"
)

var noColor bool

// SetNoColor disables all ANSI color output.
func SetNoColor(disabled bool) {
	noColor = disabled
}

// C applies an ANSI color code to text (exported for use by other packages).
func C(code, text string) string {
	if noColor {
		return text
	}
	return code + text + Reset
}

// c is an internal alias for C.
func c(code, text string) string {
	return C(code, text)
}

// IsTerminal returns true if stdout is a TTY.
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Spinner shows an animated braille spinner on a TTY.
type Spinner struct {
	mu      sync.Mutex
	msg     string
	running bool
	done    chan struct{}
}

var braille = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner creates a spinner with the given message.
func NewSpinner(msg string) *Spinner {
	return &Spinner{msg: msg}
}

// Start begins the spinner animation.
func (s *Spinner) Start() {
	if !IsTerminal() || noColor {
		fmt.Printf("  %s\n", s.msg)
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.done = make(chan struct{})
	s.mu.Unlock()

	go func() {
		i := 0
		for {
			select {
			case <-s.done:
				fmt.Print("\r\033[K")
				return
			default:
				fmt.Printf("\r  %s %s", c(Dim, braille[i%len(braille)]), s.msg)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
}

// Update changes the spinner message.
func (s *Spinner) Update(msg string) {
	s.mu.Lock()
	s.msg = msg
	s.mu.Unlock()
}

// Stop halts the spinner.
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.done)
}

// PrintFileResult prints a per-file conversion result.
func PrintFileResult(name, output string, warningCount int, err error) {
	if err != nil {
		fmt.Printf("  %s  %s — %s\n", c(Red, "✗"), name, c(Red, err.Error()))
		return
	}
	suffix := ""
	if warningCount > 0 {
		suffix = fmt.Sprintf(" %s", c(Yellow, fmt.Sprintf("(%d warnings)", warningCount)))
	}
	fmt.Printf("  %s  %s → %s%s\n", c(Green, "✓"), name, output, suffix)
}

// PrintSummary prints a box-drawing summary table.
func PrintSummary(converted, warnings, errors, total int, outputDir string) {
	fmt.Println()
	fmt.Printf("  %s\n", c(Bold, "Conversion Summary"))
	fmt.Println("  " + strings.Repeat("─", 40))

	fmt.Printf("  Files processed:    %d\n", total)
	fmt.Printf("  Converted:          %s\n", c(Green, fmt.Sprintf("%d", converted)))
	if warnings > 0 {
		fmt.Printf("  Warnings:           %s\n", c(Yellow, fmt.Sprintf("%d", warnings)))
	}
	if errors > 0 {
		fmt.Printf("  Errors:             %s\n", c(Red, fmt.Sprintf("%d", errors)))
	}
	if outputDir != "" {
		fmt.Printf("  Output:             %s\n", outputDir)
	}
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()
}

// PrintDryRun prints ProbeScript to stdout with keyword highlighting.
func PrintDryRun(filename, code string) {
	fmt.Printf("\n%s %s\n", c(Bold, "───"), c(Bold, filename))
	for _, line := range strings.Split(code, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#"):
			fmt.Println(c(Dim, line))
		case strings.HasPrefix(trimmed, "test "),
			strings.HasPrefix(trimmed, "recipe "),
			strings.HasPrefix(trimmed, "before each"),
			strings.HasPrefix(trimmed, "after each"),
			strings.HasPrefix(trimmed, "on failure"),
			strings.HasPrefix(trimmed, "with examples:"),
			strings.HasPrefix(trimmed, "use "):
			fmt.Println(c(Bold, line))
		default:
			fmt.Println(line)
		}
	}
	fmt.Println()
}
