package parser_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/l10n"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func TestL10nStringLexesAsString(t *testing.T) {
	src := "test \"t\"\n  tap l10n \"saveButton\"\n  see l10n  \"title\" in l10n \"sheet\"\n  tap l10nKey\n"
	toks, err := parser.NewLexer(src).Tokenize()
	if err != nil {
		t.Fatal(err)
	}
	var strs, words []string
	for _, tk := range toks {
		switch tk.Type {
		case parser.TOKEN_STRING:
			strs = append(strs, tk.Literal)
		case parser.TOKEN_IDENT:
			words = append(words, tk.Literal)
		}
	}
	want := []string{"t", l10n.Marker("saveButton"), l10n.Marker("title"), l10n.Marker("sheet")}
	if len(strs) != len(want) {
		t.Fatalf("strings = %q, want %q", strs, want)
	}
	for i := range want {
		if strs[i] != want[i] {
			t.Errorf("string %d = %q, want %q", i, strs[i], want[i])
		}
	}
	found := false
	for _, w := range words {
		if w == "l10nKey" {
			found = true
		}
	}
	if !found {
		t.Errorf("l10nKey must stay a plain word, words = %q", words)
	}
}

func TestL10nNeedsKey(t *testing.T) {
	if _, err := parser.NewLexer("test \"t\"\n  tap l10n \"\"\n").Tokenize(); err == nil {
		t.Fatal("empty l10n key must be an error")
	}
}
