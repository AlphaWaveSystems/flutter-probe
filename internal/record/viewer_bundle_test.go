package record

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The page must carry the exchange bodies and widget trees the timeline names, because a page opened from disk
// cannot read them, and the script data must not be able to close the script tag.
func TestWriteViewerInlinesExchangesAndTrees(t *testing.T) {
	b, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := b.WriteExchange(Exchange{
		ID: "0001", Seq: 1, Method: "POST", URL: "/login", Status: 200,
		RequestBody: "</script><b>hi</b>", ResponseBody: `{"ok":true}`,
		RequestHeaders: map[string]string{}, ResponseHeaders: map[string]string{}, Source: "dart_io",
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.WriteTree("0001", "Scaffold\n  Text \"Sign in\""); err != nil {
		t.Fatal(err)
	}
	timeline := Timeline{Steps: []Step{{
		Index: 1, Description: "tap sign in", Status: "passed", Exchanges: []string{"0001"}, Tree: "0001",
	}}}
	if err := b.WriteViewer(Manifest{SchemaVersion: SchemaVersion, RunID: "run-1", TestName: "login"}, timeline); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(b.Dir(), "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	for _, want := range []string{`"request_body"`, `Scaffold`, `</script>`, `ok\":true`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	if n := strings.Count(html, "</script>"); n != 1 {
		t.Fatalf("found %d closing script tags, want 1", n)
	}
}

func TestWriteViewerRejectsBadIDsAndMissingFiles(t *testing.T) {
	b, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bad := Timeline{Steps: []Step{{Exchanges: []string{"../secret"}}}}
	if err := b.WriteViewer(Manifest{}, bad); err == nil {
		t.Fatal("a traversal ID was accepted")
	}
	missing := Timeline{Steps: []Step{{Exchanges: []string{"0007"}}}}
	if err := b.WriteViewer(Manifest{}, missing); err == nil {
		t.Fatal("a missing exchange file was ignored")
	}
	if _, err := os.Stat(filepath.Join(b.Dir(), "index.html")); err == nil {
		t.Fatal("index.html was written despite the error")
	}
}
