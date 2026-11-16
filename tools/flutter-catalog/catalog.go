package main

// The widget catalog of an installed Flutter SDK (docs/compat/README.md). Everything here only READS files of the SDK: the version stamp and the Dart sources of the
// widgets, material and cupertino libraries. It never writes into the SDK, never runs anything from it and never uses the network. What it records are facts about
// names (a class name, the library that exports it, its direct superclass, whether it is abstract or marked deprecated); no documentation text is copied.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// schemaVersion changes when the shape of catalog.json changes.
const schemaVersion = 1

// libraries is the fixed order of the three libraries. A widget belongs to the first library whose source folder holds its file, so the order is part of the output.
var libraries = []string{"widgets", "material", "cupertino"}

// Catalog is the content of catalog.json.
type Catalog struct {
	Schema  int      `json:"schema"`
	Flutter Versions `json:"flutter"`
	Counts  Counts   `json:"counts"`
	Widgets []Widget `json:"widgets"`
}

// Versions is the stamp of the SDK the catalog was made from.
type Versions struct {
	Version  string `json:"version"`
	Channel  string `json:"channel,omitempty"`
	Revision string `json:"revision,omitempty"`
	Dart     string `json:"dart"`
}

// Counts are the number of widgets per library and in all.
type Counts struct {
	Total      int            `json:"total"`
	ByLibrary  map[string]int `json:"by_library"`
	Abstract   int            `json:"abstract"`
	Deprecated int            `json:"deprecated"`
}

// Widget is one public widget class.
type Widget struct {
	Name       string `json:"name"`
	Library    string `json:"library"`
	Base       string `json:"base"`
	Abstract   bool   `json:"abstract,omitempty"`
	Deprecated bool   `json:"deprecated,omitempty"`
}

// class is one class declaration found in a source file.
type class struct {
	name       string
	super      string
	abstract   bool
	deprecated bool
	file       string // slash path relative to the lib folder
}

var (
	versionFile = filepath.Join("bin", "cache", "flutter.version.json")
	libDir      = filepath.Join("packages", "flutter", "lib")

	exportRE = regexp.MustCompile(`^export\s+'([^']+)'(?:\s+(show|hide)\s+([^;]+))?;`)
	classRE  = regexp.MustCompile(`^((?:(?:abstract|base|final|sealed|interface|mixin)\s+)*)class\s+(\w+)`)
	superRE  = regexp.MustCompile(`\bextends\s+(\w+)`)
)

// Generate reads the SDK at root and returns its widget catalog.
func Generate(root string) (*Catalog, error) {
	v, err := readVersions(root)
	if err != nil {
		return nil, err
	}
	lib := filepath.Join(root, libDir)

	// Every class of the three source folders (private ones too: a public widget can extend a private base), keyed by name, for the superclass chains.
	byName := map[string]*class{}
	perFile := map[string][]*class{}
	for _, l := range libraries {
		dir := filepath.Join(lib, "src", l)
		var files []string
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".dart") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("read the %s sources: %w", l, err)
		}
		sort.Strings(files)
		for _, p := range files {
			rel, _ := filepath.Rel(lib, p)
			rel = filepath.ToSlash(rel)
			src, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			for _, c := range parseClasses(string(src), rel) {
				perFile[rel] = append(perFile[rel], c)
				if _, taken := byName[c.name]; !taken {
					byName[c.name] = c
				}
			}
		}
	}

	var out []Widget
	seen := map[string]bool{}
	for _, l := range libraries {
		exports, err := exportedSources(lib, l)
		if err != nil {
			return nil, err
		}
		for _, e := range exports {
			for _, c := range perFile[e.file] {
				if strings.HasPrefix(c.name, "_") || seen[c.name] || !e.allows(c.name) || !isWidget(c, byName) {
					continue
				}
				seen[c.name] = true
				out = append(out, Widget{Name: c.name, Library: l, Base: c.super, Abstract: c.abstract, Deprecated: c.deprecated})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if li, lj := libIndex(out[i].Library), libIndex(out[j].Library); li != lj {
			return li < lj
		}
		return out[i].Name < out[j].Name
	})

	c := &Catalog{Schema: schemaVersion, Flutter: v, Widgets: out, Counts: Counts{ByLibrary: map[string]int{}}}
	for _, l := range libraries {
		c.Counts.ByLibrary[l] = 0
	}
	for _, w := range out {
		c.Counts.Total++
		c.Counts.ByLibrary[w.Library]++
		if w.Abstract {
			c.Counts.Abstract++
		}
		if w.Deprecated {
			c.Counts.Deprecated++
		}
	}
	return c, nil
}

// Marshal is the exact text of catalog.json: indented, keys in struct order (map keys sorted by encoding/json), one trailing newline.
func Marshal(c *Catalog) ([]byte, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func libIndex(l string) int {
	for i, x := range libraries {
		if x == l {
			return i
		}
	}
	return len(libraries)
}

func readVersions(root string) (Versions, error) {
	b, err := os.ReadFile(filepath.Join(root, versionFile))
	if err != nil {
		return Versions{}, fmt.Errorf("not a Flutter SDK folder (no %s): %w", filepath.ToSlash(versionFile), err)
	}
	var raw struct {
		FrameworkVersion  string `json:"frameworkVersion"`
		Channel           string `json:"channel"`
		FrameworkRevision string `json:"frameworkRevision"`
		DartSdkVersion    string `json:"dartSdkVersion"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return Versions{}, fmt.Errorf("read %s: %w", filepath.ToSlash(versionFile), err)
	}
	if raw.FrameworkVersion == "" || raw.DartSdkVersion == "" {
		return Versions{}, fmt.Errorf("%s has no frameworkVersion or dartSdkVersion", filepath.ToSlash(versionFile))
	}
	return Versions{Version: raw.FrameworkVersion, Channel: raw.Channel, Revision: raw.FrameworkRevision, Dart: raw.DartSdkVersion}, nil
}

// export is one `export 'src/...'` line of a library file: the file it exports and the names it limits that to.
type export struct {
	file string
	mode string // "", "show" or "hide"
	set  map[string]bool
}

func (e export) allows(name string) bool {
	switch e.mode {
	case "show":
		return e.set[name]
	case "hide":
		return !e.set[name]
	}
	return true
}

// exportedSources lists, in file order, the source files under src/ that the library file <lib>.dart exports. Exports of other libraries (`export 'widgets.dart'`)
// and of packages are not followed: a widget belongs to the library whose own folder holds it.
func exportedSources(lib, name string) ([]export, error) {
	b, err := os.ReadFile(filepath.Join(lib, name+".dart"))
	if err != nil {
		return nil, err
	}
	var out []export
	for _, line := range strings.Split(string(b), "\n") {
		m := exportRE.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil || !strings.HasPrefix(m[1], "src/"+name+"/") {
			continue
		}
		e := export{file: m[1], mode: m[2]}
		if m[2] != "" {
			e.set = map[string]bool{}
			for _, n := range strings.Split(m[3], ",") {
				e.set[strings.TrimSpace(n)] = true
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// isWidget reports whether the chain of superclasses of c reaches Widget. A chain through a class this tool did not read ends the walk (not a widget).
func isWidget(c *class, byName map[string]*class) bool {
	seen := map[string]bool{}
	for cur := c; cur != nil && cur.super != ""; cur = byName[cur.super] {
		if cur.super == "Widget" {
			return true
		}
		if seen[cur.name] {
			return false
		}
		seen[cur.name] = true
		if byName[cur.super] == nil {
			return false
		}
	}
	return false
}

// parseClasses finds the top-level class declarations of one Dart source: the modifiers, the name, the direct superclass and whether an @Deprecated annotation is
// attached. It looks at lines that start in column 0 only (nested and local classes are not exported), reads a declaration up to its first `{` or `;`, and drops
// generic arguments before looking for `extends`.
func parseClasses(src, file string) []*class {
	lines := strings.Split(src, "\n")
	var out []*class
	for i, line := range lines {
		m := classRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		c := &class{name: m[2], file: file, abstract: strings.Contains(m[1], "abstract"), deprecated: hasDeprecated(lines, i)}
		var decl strings.Builder
		for j := i; j < len(lines) && j < i+12; j++ {
			l := lines[j]
			if k := strings.IndexAny(l, "{;"); k >= 0 {
				decl.WriteString(l[:k])
				break
			}
			decl.WriteString(l + " ")
		}
		if s := superRE.FindStringSubmatch(stripGenerics(decl.String())); s != nil {
			c.super = s[1]
		}
		out = append(out, c)
	}
	return out
}

// stripGenerics removes every balanced <...> from s.
func stripGenerics(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>' && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hasDeprecated looks upward from the class line over comment, blank and annotation lines (an annotation can span lines) up to the end of the previous
// declaration, and reports whether one of them is an @Deprecated or @deprecated annotation. Doc comments are skipped, so words in them never count.
func hasDeprecated(lines []string, at int) bool {
	for j := at - 1; j >= 0; j-- {
		t := strings.TrimSpace(lines[j])
		switch {
		case t == "", strings.HasPrefix(t, "//"):
			continue
		case strings.HasPrefix(t, "@Deprecated"), strings.HasPrefix(t, "@deprecated"):
			return true
		case strings.HasSuffix(t, ";"), strings.HasSuffix(t, "}"), strings.HasPrefix(t, "import "), strings.HasPrefix(t, "export "):
			return false
		}
	}
	return false
}
