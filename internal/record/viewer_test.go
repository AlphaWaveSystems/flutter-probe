package record

import (
	"strings"
	"testing"
)

func TestViewerInlineJSON(t *testing.T) {
	manifest := `{"name":"recording","version":1}`
	timeline := `{"steps":[{"index":1,"description":"Tap button","status":"passed"}]}`
	output, err := RenderViewer([]byte(manifest), []byte(timeline))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"const MANIFEST = " + manifest, "const TIMELINE = " + timeline} {
		if !strings.Contains(string(output), want) {
			t.Errorf("output missing %q", want)
		}
	}
	for _, forbidden := range []string{"/*__MANIFEST__*/", "/*__TIMELINE__*/", "http://", "https://", "innerHTML", "fetch("} {
		if strings.Contains(string(output), forbidden) {
			t.Errorf("output contains %q", forbidden)
		}
	}
}

func TestViewerEscapesScriptData(t *testing.T) {
	payload := `{"steps":[{"description":"</script><script>alert(1)</script>&"}]}`
	output, err := RenderViewer([]byte(`{"name":"<>&"}`), []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	page := string(output)
	if strings.Count(page, "</script>") != 1 || strings.Count(page, "<script>") != 1 {
		t.Fatal("bundle data introduced an extra script boundary")
	}
	if !strings.Contains(page, `\u003c/script\u003e\u003cscript\u003ealert(1)\u003c/script\u003e\u0026`) {
		t.Fatal("script payload was not escaped")
	}
	if !strings.Contains(page, `"name":"\u003c\u003e\u0026"`) {
		t.Fatal("manifest was not escaped")
	}
}

func TestViewerInvalidJSON(t *testing.T) {
	for _, tc := range []struct{ manifest, timeline string }{
		{"", `{}`}, {`{`, `{}`}, {`{}`, ""}, {`{}`, `{"steps":]}`},
	} {
		if output, err := RenderViewer([]byte(tc.manifest), []byte(tc.timeline)); err == nil || output != nil {
			t.Errorf("RenderViewer(%q, %q) = %q, %v; want nil, error", tc.manifest, tc.timeline, output, err)
		}
	}
}

func TestViewerReplacementIsNotRecursive(t *testing.T) {
	manifest := `{"name":"/*__TIMELINE__*/null"}`
	output, err := RenderViewer([]byte(manifest), []byte(`{"steps":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "const MANIFEST = "+manifest) {
		t.Fatal("placeholder inside bundle data was replaced")
	}
}
