package convert

import (
	"fmt"
	"strings"
)

// ProbeWriter builds well-formatted .probe output.
type ProbeWriter struct {
	sb    strings.Builder
	depth int
}

// NewProbeWriter creates a new ProbeWriter.
func NewProbeWriter() *ProbeWriter {
	return &ProbeWriter{}
}

func (w *ProbeWriter) indent() string {
	return strings.Repeat("  ", w.depth)
}

// Comment writes a # comment line.
func (w *ProbeWriter) Comment(text string) {
	w.sb.WriteString(w.indent() + "# " + text + "\n")
}

// BlankLine writes an empty line.
func (w *ProbeWriter) BlankLine() {
	w.sb.WriteString("\n")
}

// Test writes a test declaration.
func (w *ProbeWriter) Test(name string) {
	w.sb.WriteString(fmt.Sprintf("test %q\n", name))
	w.depth = 1
}

// Recipe writes a recipe declaration with optional params.
func (w *ProbeWriter) Recipe(name string, params []string) {
	if len(params) > 0 {
		w.sb.WriteString(fmt.Sprintf("recipe %q (%s)\n", name, strings.Join(params, ", ")))
	} else {
		w.sb.WriteString(fmt.Sprintf("recipe %q\n", name))
	}
	w.depth = 1
}

// BeforeEach writes a before each block.
func (w *ProbeWriter) BeforeEach() {
	w.sb.WriteString("before each\n")
	w.depth = 1
}

// AfterEach writes an after each block.
func (w *ProbeWriter) AfterEach() {
	w.sb.WriteString("after each\n")
	w.depth = 1
}

// OnFailure writes an on failure block.
func (w *ProbeWriter) OnFailure() {
	w.sb.WriteString("on failure\n")
	w.depth = 1
}

// Use writes a use statement.
func (w *ProbeWriter) Use(path string) {
	w.sb.WriteString(fmt.Sprintf("use %q\n", path))
}

// Tags writes tag annotations.
func (w *ProbeWriter) Tags(tags []string) {
	if len(tags) == 0 {
		return
	}
	var formatted []string
	for _, t := range tags {
		if !strings.HasPrefix(t, "@") {
			t = "@" + t
		}
		formatted = append(formatted, t)
	}
	w.sb.WriteString(w.indent() + strings.Join(formatted, " ") + "\n")
}

// Step writes a ProbeScript step line.
func (w *ProbeWriter) Step(line string) {
	w.sb.WriteString(w.indent() + line + "\n")
}

// TODO writes an unconvertible line as a comment.
func (w *ProbeWriter) TODO(original string) {
	w.sb.WriteString(w.indent() + "# TODO: " + original + "\n")
}

// WithExamples writes examples block.
func (w *ProbeWriter) WithExamples(headers []string, rows [][]string) {
	w.sb.WriteString("\nwith examples:\n")
	w.sb.WriteString("  " + strings.Join(headers, "\t") + "\n")
	for _, row := range rows {
		var quoted []string
		for _, cell := range row {
			quoted = append(quoted, fmt.Sprintf("%q", cell))
		}
		w.sb.WriteString("  " + strings.Join(quoted, "\t") + "\n")
	}
}

// Indent increases the indentation level.
func (w *ProbeWriter) Indent() {
	w.depth++
}

// Dedent decreases the indentation level.
func (w *ProbeWriter) Dedent() {
	if w.depth > 0 {
		w.depth--
	}
}

// SetDepth sets the indentation level directly.
func (w *ProbeWriter) SetDepth(d int) {
	w.depth = d
}

// String returns the accumulated ProbeScript.
func (w *ProbeWriter) String() string {
	return w.sb.String()
}
