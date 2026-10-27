package mcp

import (
	"reflect"
	"testing"
)

// studio_run_file hands line, tag and failed_only to Studio's run_file method under the names the automation endpoint
// reads, and sends only the fields the caller set.
func TestStudioRunFileSendsTheScope(t *testing.T) {
	calls := fakeStudio(t, map[string]any{"run_file": map[string]any{"started": true}})
	callTool(t, "studio_run_file", map[string]any{"path": "/w/a.probe", "line": 12})
	callTool(t, "studio_run_file", map[string]any{"path": "/w/a.probe", "tag": "smoke"})
	callTool(t, "studio_run_file", map[string]any{"path": "/w/a.probe", "failed_only": true})
	want := []map[string]any{
		{"path": "/w/a.probe", "line": float64(12)},
		{"path": "/w/a.probe", "tag": "smoke"},
		{"path": "/w/a.probe", "failedOnly": true},
	}
	if len(*calls) != len(want) {
		t.Fatalf("got %d calls, want %d", len(*calls), len(want))
	}
	for i, w := range want {
		if got := (*calls)[i]["params"]; !reflect.DeepEqual(got, w) {
			t.Errorf("call %d params = %v, want %v", i, got, w)
		}
	}
}
