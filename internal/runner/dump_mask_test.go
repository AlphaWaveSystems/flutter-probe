package runner

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// dumpClient answers the widget-tree dump with a fixed tree.
type dumpClient struct {
	*fakeAIClient
	tree string
}

func (c dumpClient) DumpWidgetTree(context.Context) (string, error) { return c.tree, nil }

// captureDumpOutput returns what fn prints to standard output.
func captureDumpOutput(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	captured := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		captured <- b.String()
	}()
	fn()
	os.Stdout = saved
	w.Close()
	out := <-captured
	r.Close()
	return out
}

// The tree is text on screen, so a value the run resolved from the environment is masked in the dump a step prints.
func TestWidgetTreeDumpMasksResolvedValues(t *testing.T) {
	const secret = "dump-secret-5151"
	sysdialog.NoteSecret(secret)
	client := dumpClient{fakeAIClient: &fakeAIClient{}, tree: "EditableText value=" + secret}
	e := NewExecutor(client, nil, nil, time.Second, false)
	out := captureDumpOutput(t, func() {
		if err := e.runAction(context.Background(), parser.ActionStep{Verb: parser.VerbDumpTree, Line: 1}); err != nil {
			t.Errorf("dump: %v", err)
		}
	})
	if strings.Contains(out, secret) || !strings.Contains(out, "EditableText value=****") {
		t.Errorf("the dump printed the resolved value: %q", out)
	}
}
