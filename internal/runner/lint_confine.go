package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// lintConfinement is the workspace fence of a lint given LintWithRoot. Under it a lint reads a file only when the file, with its
// symlinks resolved, lies inside the workspace root. A file outside is not read, and the report names it by its base name. A
// file inside that does not parse is named by its relative path and the parser's line, never by its text: the parser's message
// quotes the file. A nil *lintConfinement is the CLI's behaviour: every method here allows the read and records nothing.
type lintConfinement struct {
	root        string        // the root, with its symlinks resolved
	linted      string        // the absolute path of the linted file, to tell its own use statements from those of other files
	skipRecipes bool          // a recipe source (the folder, a recipe file or a use target) was refused: unresolved recipe calls are not reported
	findings    []LintFinding // what the fence kept out or could not read, in the order it met them
}

// newLintConfinement fences a lint to root. path is the file being linted.
func newLintConfinement(root, path string) (*lintConfinement, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace root: %w", err)
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("workspace root is not a directory")
	}
	linted, err := filepath.Abs(path)
	if err != nil {
		linted = path
	}
	return &lintConfinement{root: resolved, linted: linted}, nil
}

// locate returns p with its symlinks resolved, as far as they exist, and whether that lies inside the root. A path that does not
// exist is judged by where it is written; nothing is read from it either way.
func (c *lintConfinement) locate(p string) (string, bool) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p, false
	}
	resolved := resolveExisting(abs)
	rel, err := filepath.Rel(c.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return resolved, false
	}
	return resolved, true
}

// resolveExisting resolves the symlinks of the longest prefix of p that exists, and appends the rest as it is written.
func resolveExisting(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(resolveExisting(parent), filepath.Base(p))
}

// name is how a report names p: relative to the root when p lies inside it, and the base name when it does not, so a report never
// shows a directory outside the workspace.
func (c *lintConfinement) name(p string) string {
	if resolved, inside := c.locate(p); inside {
		if rel, err := filepath.Rel(c.root, resolved); err == nil {
			return rel
		}
	}
	return filepath.Base(p)
}

// allow reports whether a read of p may happen. A path outside the workspace is refused: the refusal is recorded as one warning
// with the fixed text refusal, at line. Line is the line of the statement in the linted file that names p, or 0 when another
// file names it. The text names nothing, so a report never shows a path outside the workspace.
func (c *lintConfinement) allow(p string, line int, refusal string) bool {
	if c == nil {
		return true
	}
	if _, inside := c.locate(p); inside {
		return true
	}
	c.findings = append(c.findings, LintFinding{Line: line, Warning: true, Message: refusal})
	return false
}

// Refusal texts. Each is one warning per refused path, and none names a file. The first three refuse a recipe source, and each says in
// the same line that it suppresses unknown-step errors (see recipesSkipped). The CSV source refusal suppresses nothing, so it does not.
const (
	refuseRecipesFolder = "the recipes folder is outside the workspace; recipes were not read, so calls to recipes were not checked"
	refuseRecipeFile    = "a recipe file is outside the workspace and was not read, so calls to recipes were not checked"
	refuseUseTarget     = "use target is outside the workspace and was not read, so calls to recipes were not checked"
	refuseCSVSource     = "CSV source is outside the workspace and was not read"
)

// recipesSkipped reports whether a recipe source was refused. A refused source may define any recipe the lint cannot see, so an
// unresolved recipe call is not reported as an unknown step: the one warning for the refused source explains it. The lint cannot
// tell a call that a refused source defines from one that nothing defines, so a lint with a refused source reports neither as an
// error. A lint with no refused source reports every unknown call, as it always has.
func (c *lintConfinement) recipesSkipped() bool {
	return c != nil && c.skipRecipes
}

// parseLinePattern finds the line the parser reports at the start of its messages ("line 2: ...").
var parseLinePattern = regexp.MustCompile(`line (\d+):`)

// broken records that the file at p, inside the workspace, does not parse. The error names the file and the parser's line, and none
// of the file's text: the parser's message quotes that text.
func (c *lintConfinement) broken(p string, err error, line int) {
	msg := fmt.Sprintf("%s does not parse", c.name(p))
	if m := parseLinePattern.FindStringSubmatch(err.Error()); m != nil {
		msg += fmt.Sprintf(": first error at its line %s", m[1])
	}
	c.findings = append(c.findings, LintFinding{Line: line, Message: msg})
}

// lineOf is line when from is the linted file, and 0 otherwise: a statement in another file has no line in the linted file.
func (c *lintConfinement) lineOf(from string, line int) int {
	if c == nil {
		return 0
	}
	if abs, err := filepath.Abs(from); err == nil && abs == c.linted {
		return line
	}
	return 0
}

// csvFile is the path loadCSVExamples opens for a `with examples from` source: the source itself when it is absolute, and
// otherwise the source relative to the folder of the linted file.
func csvFile(path, source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(filepath.Dir(path), source)
}

// readError is the error for a file the lint could not read. With a fence it names the file relatively, so the error does not show
// a path outside the workspace. Without one it is err.
func (r *Runner) readError(p string, err error) error {
	if r.confine == nil {
		return err
	}
	return fmt.Errorf("%s cannot be read", r.confine.name(p))
}
