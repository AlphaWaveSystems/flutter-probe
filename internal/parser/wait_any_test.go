package parser_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func TestParser_WaitUntilAnyOf(t *testing.T) {
	prog := mustParse(t, "test \"t\"\n  wait until any of \"Got it\", \"Login\" or \"Home\" appears\n  tap \"OK\"\n")
	body := prog.Tests[0].Body
	if len(body) != 2 {
		t.Fatalf("want 2 steps, got %d: %+v", len(body), body)
	}
	w, ok := body[0].(parser.WaitStep)
	if !ok || w.Kind != parser.WaitAny || len(w.Any) != 3 || w.Any[0] != "Got it" || w.Any[2] != "Home" {
		t.Fatalf("got %#v", body[0])
	}
}
