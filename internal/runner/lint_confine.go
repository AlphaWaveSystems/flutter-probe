package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// lintConfinement is the workspace fence of a lint given LintWithRoot. Under it a lint reads a file only when the file, with its
// symlinks resolved, lies inside the workspace root. A file outside is not read, and the refusal names no file. A file inside that
// does not parse, or cannot be read, is named by its relative path and the parser's line, never by its text. A nil *lintConfinement
// is the CLI's behaviour: every method here allows the read as it is written and records nothing.
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

// errLink is what resolveLinks returns for a symlink that does not resolve: a dangling link, or one in a loop. Such a link is refused
// whatever it points to. Refusing it the same way whether its target exists or not means a lint reveals nothing about the disk.
var errLink = errors.New("symlink does not resolve")

// resolveLinks returns p with the symlinks in it resolved, as far as they exist. A component that does not exist is kept as it is
// written, so a missing file is judged by where it is written. A symlink that does not resolve is errLink.
func resolveLinks(p string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	}
	if info, err := os.Lstat(p); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errLink
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p, nil
	}
	resolvedParent, err := resolveLinks(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(p)), nil
}

// within reports whether path, both absolute and clean, lies inside root. A sibling whose name starts with root's name is outside.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// locate returns p with its symlinks resolved, and whether that lies inside the root. A symlink that does not resolve is outside.
func (c *lintConfinement) locate(p string) (string, bool) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p, false
	}
	resolved, err := resolveLinks(abs)
	if err != nil {
		return abs, false
	}
	return resolved, within(c.root, resolved)
}

// open decides whether a read of p may happen, and returns the path to read. The path returned is p with its symlinks resolved, so the
// read opens the file the check looked at. A path outside the workspace, or a symlink that does not resolve, is refused: the refusal is
// one warning at line with the fixed text refusal. Line is the line of the statement in the linted file that names p, or 0 when another
// file names it. A nil fence allows the read of p as it is written.
func (c *lintConfinement) open(p string, line int, refusal string) (string, bool) {
	if c == nil {
		return p, true
	}
	if resolved, inside := c.locate(p); inside {
		if lintAfterCheck != nil {
			lintAfterCheck(p)
		}
		return resolved, true
	}
	c.findings = append(c.findings, LintFinding{Line: line, Warning: true, Message: refusal})
	return "", false
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

// lintAfterCheck is a test seam. When set, it runs with each path the fence allows, after the check and before the read, so a test can
// swap a symlink in that window and show that the read uses the path the check resolved. It is nil in every other run.
var lintAfterCheck func(p string)

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

// unreadable records that a file the fence allowed could not be read, as an error named by its relative path. The read error is not
// shown: it names the path the read opened, which is the absolute one.
func (c *lintConfinement) unreadable(p string, line int) {
	c.findings = append(c.findings, LintFinding{Line: line, Message: fmt.Sprintf("%s cannot be read", c.name(p))})
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

// loadCSVSource reads the CSV of a `with examples from` source for a lint. Without a fence it is loadCSVExamples as it always was.
// With one, the file is read at the path the fence checked, and only when the fence allows it.
func loadCSVSource(conf *lintConfinement, path string, source string, line int) (*parser.ExamplesBlock, bool) {
	if conf == nil {
		csv, err := loadCSVExamples(filepath.Dir(path), source)
		return csv, err == nil
	}
	read, ok := conf.open(csvFile(path, source), line, refuseCSVSource)
	if !ok {
		return nil, false
	}
	csv, err := loadCSVExamples("", read)
	return csv, err == nil
}
