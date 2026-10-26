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
	prog, err := parser.ParseFile(string(src))
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	r := New(cfg, nil, nil, RunOptions{DryRun: true, Files: []string{path}})
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
		if _, err := os.Stat(target); os.IsNotExist(err) {
			// A missing file is a warning: a run only fails when a call needs one of its recipes, and that call is
			// reported as an unknown step.
			findings = append(findings, LintFinding{Line: u.Line, Warning: true, Message: fmt.Sprintf("use %q: no file at %s", u.Path, target)})
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
	for _, name := range runtimeVars {
		defined[name] = true
	}
	for _, t := range prog.Tests {
		if t.Examples == nil {
			continue
		}
		headers := t.Examples.Headers
		if t.Examples.Source != "" {
			if csv, err := loadCSVExamples(filepath.Dir(path), t.Examples.Source); err == nil {
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
		walkSteps(body, func(s parser.Step) { markStored(s, defined) })
	}

	for _, body := range own {
		walkSteps(body, func(s parser.Step) {
			line := stepLine(s)
			if rc, ok := s.(parser.RecipeCall); ok {
				if _, _, _, found := resolveCall(r.recipes, rc); !found {
					msg := fmt.Sprintf("unknown step %q: not a built-in step and no recipe with that name is defined (check the use statements and the recipes folder)", rc.Name)
					if strings.HasPrefix(rc.Name, "use ") || rc.Name == "use" {
						// The parser reads use only at the top of a file; inside a test it is an ordinary call.
						msg = "a use statement must come before the first test; inside a test it is not read"
					}
					findings = append(findings, LintFinding{Line: rc.Line, Message: msg})
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
	return dedupeFindings(findings), nil
}

// markStored records the names a step stores, which a placeholder may use later in the run.
func markStored(s parser.Step, defined map[string]bool) {
	if a, ok := s.(parser.ActionStep); ok && (a.Verb == parser.VerbStore || a.Verb == parser.VerbReadWithAI) {
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
