package runner

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// LintFinding is one problem LintReferences found in a file. An error is a reference a run cannot satisfy as written: a
// step or recipe call that resolves to nothing, or a selector that can never match. A warning is a placeholder that no
// example column, recipe parameter or stored value defines (a run leaves it in the text as written), or a use that names
// no file (a run only needs that file when a call uses one of its recipes).
type LintFinding struct {
	Line    int
	Warning bool
	Message string
}

// placeholderPattern matches a <name> placeholder in a step's text. Names are identifiers, with dots for names such as
// response.status. <random.TYPE> generators do not match it as a name, and are skipped by name below.
var placeholderPattern = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_.]*)>`)

// runtimeVars are placeholder names the executor sets itself while it runs a step.
var runtimeVars = []string{"clipboard", "response.status", "response.body"}

var (
	stepType      = reflect.TypeFor[parser.Step]()
	stepSliceType = reflect.TypeFor[[]parser.Step]()
	selectorType  = reflect.TypeFor[parser.Selector]()
)

// LintReferences checks path for references that a run could not resolve, without running any step. Recipe calls and
// use statements resolve the way --dry-run resolves them, with the recipes folder from cfg. A file that does not parse
// returns the parse error.
func LintReferences(cfg *config.Config, path string) ([]LintFinding, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return lintReferencesSource(cfg, path, string(src), nil)
}

// lintReferencesSource is LintReferences over the given text. The file at path is only used to resolve use targets and the
// CSV examples of `with examples from` relative to its folder. conf, when not nil, fences every read (see LintWithRoot).
func lintReferencesSource(cfg *config.Config, path, content string, conf *lintConfinement) ([]LintFinding, error) {
	prog, err := parser.ParseFile(content)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	r := New(cfg, nil, nil, RunOptions{DryRun: true, Files: []string{path}})
	r.confine = conf
	if err := r.loadRecipes(ctx); err != nil {
		return nil, err
	}
	r.baseRecipes = make(map[string]parser.RecipeDef, len(r.recipes))
	maps.Copy(r.baseRecipes, r.recipes)
	if err := r.loadFileRecipes(ctx, path, prog); err != nil {
		return nil, err
	}

	var findings []LintFinding
	for _, u := range prog.Uses {
		target := filepath.Join(filepath.Dir(path), u.Path)
		stat, shown := target, target
		if conf != nil {
			// A target outside the workspace was refused when loadUse reached it, and is not checked again here. A missing one is
			// named relative to the workspace, as the linted text writes it.
			resolved, inside := conf.locate(target)
			if !inside {
				continue
			}
			stat, shown = resolved, conf.name(target)
		}
		if _, err := os.Stat(stat); os.IsNotExist(err) {
			// A missing file is a warning: a run only fails when a call needs one of its recipes, and that call is
			// reported as an unknown step.
			findings = append(findings, LintFinding{Line: u.Line, Warning: true, Message: fmt.Sprintf("use %q: no file at %s", u.Path, shown)})
		}
	}

	// The file's own bodies are checked. The recipes it can call only add the names they define, because a recipe
	// parameter or a stored value from one of them stays set for the rest of the run.
	var own [][]parser.Step
	for _, t := range prog.Tests {
		own = append(own, t.Body)
	}
	for _, h := range prog.Hooks {
		own = append(own, h.Body)
	}
	for _, rec := range prog.Recipes {
		own = append(own, rec.Body)
	}
	for _, c := range prog.CompositeTests {
		own = append(own, c.Body)
	}

	defined := map[string]bool{}
	// Lines where a go back step starts. A call on such a line was read apart from the step (go back takes no arguments).
	goBackLines := map[int]bool{}
	for _, name := range runtimeVars {
		defined[name] = true
	}
	for _, t := range prog.Tests {
		if t.Examples == nil {
			continue
		}
		headers := t.Examples.Headers
		if t.Examples.Source != "" {
			if csv, ok := loadCSVSource(conf, path, t.Examples.Source, t.Examples.Line); ok {
				headers = csv.Headers
			}
		}
		for _, h := range headers {
			defined[h] = true
		}
	}
	for _, rec := range r.recipes {
		for _, p := range rec.Params {
			defined[p] = true
		}
		walkSteps(rec.Body, func(s parser.Step) { markStored(s, defined) })
	}
	for _, body := range own {
		walkSteps(body, func(s parser.Step) {
			markStored(s, defined)
			if a, ok := s.(parser.ActionStep); ok && a.Verb == parser.VerbGoBack {
				goBackLines[a.Line] = true
			}
		})
	}

	for _, body := range own {
		walkSteps(body, func(s parser.Step) {
			line := stepLine(s)
			if a, ok := s.(parser.ActionStep); ok && a.DirectionWord != "" {
				// A word that names a recipe is a recipe call, not a mistyped direction: no warning.
				if _, _, _, isRecipe := resolveCall(r.recipes, parser.RecipeCall{Name: a.DirectionWord}); !isRecipe {
					findings = append(findings, LintFinding{Line: line, Warning: true, Message: fmt.Sprintf("swipe: %q is not up, down, left or right, so the swipe runs down", a.DirectionWord)})
				}
			}
			if rc, ok := s.(parser.RecipeCall); ok {
				if _, _, _, found := resolveCall(r.recipes, rc); !found {
					msg := fmt.Sprintf("unknown step %q: not a built-in step and no recipe with that name is defined (check the use statements and the recipes folder)", rc.Name)
					if goBackLines[rc.Line] {
						msg = fmt.Sprintf("go back takes no arguments: the words after it on line %d are read as a separate step %q", rc.Line, rc.Name)
					} else if strings.HasPrefix(rc.Name, "use ") || rc.Name == "use" {
						// The parser reads use only at the top of a file; inside a test it is an ordinary call.
						msg = "a use statement must come before the first test; inside a test it is not read"
					}
					// With a recipe source refused (the folder, a recipe file or a use target), a call it might define is not
					// an error: the one warning for the refused source says why recipes were not read.
					if !conf.recipesSkipped() {
						findings = append(findings, LintFinding{Line: rc.Line, Message: msg})
					}
					if word, ok := pressKeyTypo(rc.Name); ok {
						findings = append(findings, LintFinding{Line: rc.Line, Warning: true, Message: fmt.Sprintf("unknown key '%s'; keys: %s; or define a recipe named '%s'", word, strings.Join(parser.PressKeys(), ", "), rc.Name)})
					}
				}
			}
			texts, sels := scanStep(s)
			for _, sel := range sels {
				if problem := selectorProblem(sel); problem != "" {
					findings = append(findings, LintFinding{Line: line, Message: problem})
				}
			}
			for _, text := range texts {
				for _, m := range placeholderPattern.FindAllStringSubmatch(text, -1) {
					name := m[1]
					if defined[name] || strings.HasPrefix(name, "random.") {
						continue
					}
					findings = append(findings, LintFinding{Line: line, Warning: true, Message: fmt.Sprintf("<%s> is not defined: no example column, recipe parameter or stored value has that name", name)})
				}
			}
		})
	}
	if conf != nil {
		findings = append(findings, conf.findings...)
	}
	return dedupeFindings(findings), nil
}

// LintReport is what LintFile found in one file. probe lint prints the References errors first, then Program, then the
// References warnings, each line numbered where it has a line.
type LintReport struct {
	// Program holds the warnings of the program rules. Message is the text probe lint prints; Line is the line the parser
	// has for the finding (the test's line for a test with no steps or a dart share, the step's line for a count check),
	// or 0 where it has none.
	Program []LintFinding
	// References holds the reference findings, in line order, as LintReferences returns them.
	References []LintFinding
}

// LintFile lints the file at path without running anything: the program rules on its parse, and the reference rules against
// cfg. It never touches a device.
//
// cfg decides where recipes come from. cfg.Recipes is used as given, so a relative path is relative to the working directory,
// not to the probe.yaml it was read from. probe lint loads cfg from --config when it is set, and otherwise from the probe.yaml
// in the working directory, falling back to the defaults (recipes_folder "tests/recipes") when there is none. A caller such as
// Studio must load cfg the same way and run from the same working directory, or the same file can resolve different recipes.
func LintFile(cfg *config.Config, path string, opts ...LintOption) (LintReport, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return LintReport{}, err
	}
	return LintSource(cfg, path, string(src), opts...)
}

// LintSource lints content as if it were the file at path: the editor's text, say, which may differ from the file on disk. It
// gives the same report as LintFile for the same content. It never touches a device, and it writes no package state, so
// concurrent calls are safe; cfg is only read.
//
// path is used for one thing: resolving the `use` targets and the CSV examples of `with examples from` relative to the file's
// folder, as they are for a file on disk. The file need not exist. The config rules are LintFile's: cfg.Recipes is used as
// given, so a relative path is relative to the working directory, and probe lint loads cfg from --config or from the probe.yaml
// in the working directory, with the defaults when there is none. A caller such as Studio must load cfg the same way and run
// from the same working directory.
//
// The error is returned only when content cannot be parsed or the recipes folder cannot be read. A finding is not an error.
//
// With LintWithRoot the reads are fenced to the workspace; see that option. The fence is opt-in: without it the lint reads as it
// always has.
func LintSource(cfg *config.Config, path string, content string, opts ...LintOption) (LintReport, error) {
	var set lintSettings
	for _, opt := range opts {
		opt(&set)
	}
	prog, err := parser.ParseFile(content)
	if err != nil {
		return LintReport{}, err
	}
	var conf *lintConfinement
	if set.rootSet {
		if set.root == "" {
			return LintReport{}, fmt.Errorf("LintWithRoot: the workspace root is empty")
		}
		conf, err = newLintConfinement(set.root, path)
		if err != nil {
			return LintReport{}, err
		}
	}
	refs, err := lintReferencesSource(cfg, path, content, conf)
	if err != nil {
		return LintReport{}, err
	}
	return LintReport{Program: lintProgram(prog), References: refs}, nil
}

// LintOption changes what LintSource and LintFile read. Without options they read exactly as the CLI does.
type LintOption func(*lintSettings)

type lintSettings struct {
	rootSet bool
	root    string
}

// LintWithRoot fences a lint to the workspace at root, for a caller that lints text it does not fully trust, such as an editor's
// unsaved text. With it, a lint reads a file only when the file, with symlinks resolved, lies inside root:
//   - a `use` target, a CSV source, or the recipes folder outside root is not read, and one warning with fixed text is reported
//     for it, naming no file;
//   - with a recipe source refused (the folder, a recipe file or a use target), an unresolved recipe call is not reported as an
//     unknown step: the lint cannot tell whether the refused source defines it, so it reports neither as an error;
//   - a file inside root that does not parse is reported by its relative name and the parser's line, never by its text. Parse
//     errors of the linted text itself are returned as before.
//
// An empty root is an error, so a caller cannot lint unfenced by accident.
func LintWithRoot(root string) LintOption {
	return func(s *lintSettings) {
		s.rootSet = true
		s.root = root
	}
}

// pressKeyTypo reports the word of a `press <word>` line that is within two edits of one of the keys press takes, so
// `press tba` can be pointed at `tab`. Such a line is a recipe call; the warning only suggests the key.
func pressKeyTypo(name string) (string, bool) {
	fields := strings.Fields(strings.ToLower(name))
	if len(fields) != 2 || fields[0] != "press" {
		return "", false
	}
	word := fields[1]
	for _, key := range parser.PressKeys() {
		if editDistance(word, key) <= 2 {
			return word, true
		}
	}
	return "", false
}

// editDistance is the Levenshtein distance between a and b, counted in bytes (the keys are ASCII).
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// markStored records the names a step stores, which a placeholder may use later in the run.
func markStored(s parser.Step, defined map[string]bool) {
	if a, ok := s.(parser.ActionStep); ok && (a.Verb == parser.VerbStore || a.Verb == parser.VerbReadWithAI || a.Verb == parser.VerbStoreWebView) {
		defined[a.Name] = true
	}
}

// selectorProblem names why a selector can never match, or returns "" when it can. A plain text selector is not checked:
// an empty one is how a step with no target is written, and some steps take no target at all.
func selectorProblem(sel parser.Selector) string {
	switch sel.Kind {
	case parser.SelectorID:
		if sel.Text == "" || sel.Text == "#" {
			return "a #id selector needs a name after the #"
		}
	case parser.SelectorOrdinal:
		if sel.Ordinal < 1 || sel.Text == "" {
			return "an ordinal selector needs a number and text, such as 2nd \"Buy\""
		}
	case parser.SelectorPositional:
		if sel.Container == "" {
			return "the container after \"in\" is empty"
		}
	case parser.SelectorRelational:
		if sel.Anchor == "" {
			return fmt.Sprintf("the %s selector has no anchor text after it", sel.Relation)
		}
	}
	return ""
}

// stepLine is the 1-based line a step starts on, or 0 when the step does not record one.
func stepLine(s parser.Step) int {
	if n, ok := s.(parser.Node); ok {
		return n.GetLine()
	}
	return 0
}

// walkSteps calls visit for every step in steps and, depth first, for the steps nested in each of them.
func walkSteps(steps []parser.Step, visit func(parser.Step)) {
	for _, s := range steps {
		if s == nil {
			continue
		}
		visit(s)
		walkSteps(nestedSteps(s), visit)
	}
}

// nestedSteps returns the steps held in a step's fields: a conditional's branches, a loop's or retry's body, a timed
// block's inner step and the step a device section wraps.
func nestedSteps(s parser.Step) []parser.Step {
	var out []parser.Step
	v := reflect.ValueOf(s)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanInterface() {
			continue
		}
		switch f.Type() {
		case stepSliceType:
			for j := 0; j < f.Len(); j++ {
				if st, ok := f.Index(j).Interface().(parser.Step); ok {
					out = append(out, st)
				}
			}
		case stepType:
			if !f.IsNil() {
				if st, ok := f.Interface().(parser.Step); ok {
					out = append(out, st)
				}
			}
		}
	}
	return out
}

// scanStep collects the text values and the selectors in a step's own fields. Nested steps are left out, because
// walkSteps visits them on their own.
func scanStep(s parser.Step) ([]string, []parser.Selector) {
	var texts []string
	var sels []parser.Selector
	if rc, ok := s.(parser.RecipeCall); ok {
		// The name of a call holds <arg> markers in the places of its quoted arguments; they are not variables. The
		// arguments can hold placeholders of their own.
		scanValue(reflect.ValueOf(rc.Args), &texts, &sels)
		scanValue(reflect.ValueOf(rc.NumArgs), &texts, &sels)
		return texts, sels
	}
	scanValue(reflect.ValueOf(s), &texts, &sels)
	return texts, sels
}

func scanValue(v reflect.Value, texts *[]string, sels *[]parser.Selector) {
	switch v.Kind() {
	case reflect.String:
		*texts = append(*texts, v.String())
	case reflect.Interface, reflect.Pointer:
		if !v.IsNil() {
			scanValue(v.Elem(), texts, sels)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			scanValue(v.Index(i), texts, sels)
		}
	case reflect.Struct:
		if v.Type() == selectorType && v.CanInterface() {
			*sels = append(*sels, v.Interface().(parser.Selector))
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Type() == stepType || f.Type() == stepSliceType {
				continue
			}
			scanValue(f, texts, sels)
		}
	}
}

// dedupeFindings drops repeated findings and orders the rest by line, keeping the order of the file within a line.
func dedupeFindings(findings []LintFinding) []LintFinding {
	seen := map[string]bool{}
	var out []LintFinding
	for _, f := range findings {
		key := fmt.Sprintf("%d|%t|%s", f.Line, f.Warning, f.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}
