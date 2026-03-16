// Package convert defines the core types for multi-format test conversion.
package convert

// Format identifies a source test framework.
type Format string

const (
	FormatMaestro Format = "maestro"
	FormatGherkin Format = "gherkin"
	FormatRobot   Format = "robot"
	FormatDetox   Format = "detox"
	FormatAppium  Format = "appium"
)

// AllFormats lists every supported format.
var AllFormats = []Format{FormatMaestro, FormatGherkin, FormatRobot, FormatDetox, FormatAppium}

// Severity indicates how lossy a conversion warning is.
type Severity int

const (
	Info  Severity = iota // lossless conversion note
	Warn                  // lossy — some semantics lost
	Error                 // unconvertible — requires manual work
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warn:
		return "warn"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

// Warning is a single conversion issue on a specific line.
type Warning struct {
	Line     int
	Severity Severity
	Message  string
}

// Result holds the output of a single file conversion.
type Result struct {
	InputPath  string
	OutputPath string
	ProbeCode  string
	Warnings   []Warning
	Err        error
}

// Converter converts source test files to ProbeScript.
type Converter interface {
	// Format returns which format this converter handles.
	Format() Format
	// Convert transforms source content (from path) into ProbeScript.
	Convert(source []byte, path string) (*Result, error)
	// Extensions returns file extensions this converter recognizes.
	Extensions() []string
}
