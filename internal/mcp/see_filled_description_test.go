package mcp

import (
	"strings"
	"testing"
)

// write_test's description is what an agent reads to write ProbeScript. Its password advice must name the check that says whether a password field
// holds a character without reading it, and the capability it needs, so an agent does not fall back on mirroring the value in a Text.
func TestWriteTestDescriptionNamesSeeFilledForPasswordFields(t *testing.T) {
	for _, tool := range tools {
		if tool.Name != "write_test" {
			continue
		}
		for _, want := range []string{"see #pw is filled", "see #pw is empty", "see_filled"} {
			if !strings.Contains(tool.Description, want) {
				t.Errorf("write_test description does not contain %q: an agent would not learn the check", want)
			}
		}
		return
	}
	t.Fatal("write_test is not in tools")
}
