// Package appium converts Appium tests (Python/Java/JS) to ProbeScript.
package appium

import (
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/probe-convert/convert"
)

// Converter handles Appium → ProbeScript conversion.
type Converter struct{}

// New creates a new Appium converter.
func New() *Converter { return &Converter{} }

func (c *Converter) Format() convert.Format    { return convert.FormatAppium }
func (c *Converter) Extensions() []string       { return []string{".py", ".java", ".kt", ".js", ".ts"} }

func (c *Converter) Convert(source []byte, path string) (*convert.Result, error) {
	ext := strings.ToLower(filepath.Ext(path))
	var probe string
	var warnings []convert.Warning

	switch ext {
	case ".py":
		probe, warnings = convertPython(string(source), path)
	case ".java", ".kt":
		probe, warnings = convertJava(string(source), path)
	case ".js", ".ts":
		probe, warnings = convertJS(string(source), path)
	default:
		probe, warnings = convertPython(string(source), path)
	}

	return &convert.Result{
		InputPath: path,
		ProbeCode: probe,
		Warnings:  warnings,
	}, nil
}
