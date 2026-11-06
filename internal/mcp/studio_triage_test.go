package mcp

import (
	"strings"
	"testing"
)

// studio_run_state returns each finished step with its triage, so the tool description must say a failed
// step carries a triage. Agents read the description to know the field is there.
func TestStudioRunStateDescriptionMentionsTriage(t *testing.T) {
	for _, tool := range studioTools {
		if tool.Name != "studio_run_state" {
			continue
		}
		if !strings.Contains(tool.Description, "triage") {
			t.Errorf("studio_run_state description = %q, want it to say a failed step carries a triage", tool.Description)
		}
		return
	}
	t.Fatal("studio_run_state is not in studioTools")
}
