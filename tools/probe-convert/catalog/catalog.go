// Package catalog defines the formal grammar constructs for every supported
// test framework and their mappings to ProbeScript. It is the single source
// of truth for what each converter should recognize and produce.
//
// The catalog serves three purposes:
//   1. Documentation — auto-generate reference docs per language
//   2. Coverage — verify converters handle every catalogued construct
//   3. Drift detection — when a language adds constructs, add them here first;
//      coverage tests will fail until the converter catches up
package catalog

// Level classifies how well a construct can be converted.
type Level int

const (
	Full    Level = iota // lossless 1:1 mapping
	Partial              // lossy but usable (e.g. xpath → best-effort text extraction)
	Manual               // requires human review (emitted as # TODO)
	Skip                 // intentionally ignored (imports, boilerplate)
)

func (l Level) String() string {
	switch l {
	case Full:
		return "full"
	case Partial:
		return "partial"
	case Manual:
		return "manual"
	case Skip:
		return "skip"
	default:
		return "unknown"
	}
}

// Category groups constructs by semantic role.
type Category string

const (
	CatStructure   Category = "structure"   // test/describe/class definitions
	CatLifecycle   Category = "lifecycle"   // setup/teardown/hooks
	CatAction      Category = "action"      // tap, type, swipe, scroll
	CatAssertion   Category = "assertion"   // see, don't see, expect
	CatWait        Category = "wait"        // wait, sleep, waitFor
	CatNavigation  Category = "navigation"  // back, close keyboard
	CatGesture     Category = "gesture"     // long press, double tap, drag
	CatScreenshot  Category = "screenshot"  // take screenshot
	CatData        Category = "data"        // variables, examples, parameters
	CatPermission  Category = "permission"  // allow, deny, grant permissions
	CatAppControl  Category = "app_control" // launch, restart, clear data
	CatFlow        Category = "flow"        // repeat, conditional, runFlow
	CatUnsupported Category = "unsupported" // no ProbeScript equivalent
)

// Construct describes a single grammar element in a source test language.
type Construct struct {
	// ID is a unique, stable identifier for this construct (e.g. "maestro.tapOn").
	ID string

	// Name is the human-readable source syntax (e.g. "tapOn", "element(by.id()).tap()").
	Name string

	// EBNF is the formal grammar rule for this construct.
	// Uses standard EBNF notation: { } for repetition, [ ] for optional,
	// | for alternation, "..." for terminals, <...> for non-terminals.
	EBNF string

	// Example is a concrete source-language snippet demonstrating this construct.
	Example string

	// ProbeTemplate is the ProbeScript output pattern.
	// Uses $1, $2 for captured values, or empty if unsupported.
	ProbeTemplate string

	// ProbeExample is a concrete ProbeScript output for the given Example.
	ProbeExample string

	Category Category
	Level    Level

	// Notes for partial/manual constructs explaining limitations.
	Notes string
}

// Language holds the complete grammar catalog for one source framework.
type Language struct {
	// Name is the format identifier (matches convert.Format).
	Name string

	// DisplayName is the human-readable name.
	DisplayName string

	// FileExtensions lists recognized file extensions.
	FileExtensions []string

	// Version is the language/framework version this catalog targets.
	Version string

	// StructureEBNF describes the top-level file grammar.
	StructureEBNF string

	// Constructs lists every recognized grammar element.
	Constructs []Construct
}

// All returns every registered language catalog.
func All() []Language {
	return []Language{
		Maestro(),
		Gherkin(),
		Robot(),
		Detox(),
		AppiumPython(),
		AppiumJava(),
		AppiumJS(),
	}
}

// ByName returns the catalog for a given language name.
func ByName(name string) (Language, bool) {
	for _, l := range All() {
		if l.Name == name {
			return l, true
		}
	}
	return Language{}, false
}

// Stats returns coverage statistics for a language.
func (l Language) Stats() (total, full, partial, manual, skip int) {
	total = len(l.Constructs)
	for _, c := range l.Constructs {
		switch c.Level {
		case Full:
			full++
		case Partial:
			partial++
		case Manual:
			manual++
		case Skip:
			skip++
		}
	}
	return
}

// ByCategory returns constructs filtered by category.
func (l Language) ByCategory(cat Category) []Construct {
	var out []Construct
	for _, c := range l.Constructs {
		if c.Category == cat {
			out = append(out, c)
		}
	}
	return out
}

// IDs returns all construct IDs for this language.
func (l Language) IDs() []string {
	ids := make([]string, len(l.Constructs))
	for i, c := range l.Constructs {
		ids[i] = c.ID
	}
	return ids
}
