package parser_test

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// storeCompatCases are lines around `store` and the words text, of, css and webview, as ordinary words, recipe names, other steps and blocks. Their parse trees are
// recorded from the parser as it was before `store text of css:"..." in webview as name` existed (testdata/store_webview_compat.golden.json); the test says that every
// one of them still parses exactly as it did, except the lines of the new form, which are in store_webview_steps_test.go. compatSource and compatDump are the helpers of the V1 corpus.
var storeCompatCases = []string{
	`store text of css:x in webview as y`,
	`store text of css = "x" in webview as y`,
	`store text of css x "y" in webview as t`,
	`store text of css "x" in webview as y`,
	`store "hello" as greeting`,
	`store "hello" as "greeting"`,
	`store "hello" greeting`,
	`store "hello"`,
	`store hello as greeting`,
	`store hello`,
	`store "a b c" as spaced`,
	`store "<x>" as copy`,
	`store "${HOME}" as home`,
	`store "css:x" as c`,
	`store "text" as t`,
	`store "of" as o`,
	`store "in webview" as w`,
	`store text as foo`,
	`store text`,
	`store text as "foo"`,
	`store of as foo`,
	`store css as foo`,
	`store webview as foo`,
	`store "x" as css`,
	`store "x" as text`,
	`store "x" as webview`,
	`store "x" as of`,
	`store "x" as in`,
	`store response "/api/me" json "data.plan" as plan`,
	`store response "/api/me" as r`,
	`store the response "/api/me" json "id" as id`,
	`store the "hello" as greeting`,
	`store a "hello" as greeting`,
	`store "hello" as greeting within 5 seconds`,
	`store "hello" as greeting optional`,
	`store "" as empty`,
	`store "é💥" as uni`,
	`store "\"quoted\"" as q`,
	`store "tab\there" as tabbed`,
	`store text of as foo`,
	`store text of`,
	`store text of "x" as y`,
	`store text of "x"`,
	`store text of css as foo`,
	`store text of css "x" as y`,
	`store text of css:x as y`,
	`store text of css: as y`,
	`store text css as y`,
	`store text "x" in "List" as y`,
	`store text of "x" in "List" as y`,
	`store text of x in webview as y`,
	`store text in webview as y`,
	`store the text of "x" as y`,
	`store texts of "x" as y`,
	`store textual of "x" as y`,
	`tap "x" in webview`,
	`see "x" in webview`,
	`type "x" into css:"#f" in webview`,
	`wait until "x" appears in webview`,
	`tap css:"#a" in webview "p"`,
	`log "store text of css:\"#a\" in webview as t"`,
	`log "store text"`,
	`see "store text of"`,
	`tap "text of"`,
	`tap "store"`,
	`open the app`,
	`tap "Item" in "List"`,
	`read "x" with ai into y`,
	`read "text of x" with ai into text`,
	`if "x" appears
  store "a" as b`,
	`repeat 2 times
  store "a" as b`,
	`retry 2 times
  store "a" as b`,
	`store "a" as b`,
	`store "c" as d`,
	`store "<b><d>" as e`,
	`store text of css:"#a" as y`,
	`store text of css:"#a" in "webview" as y`,
	`store text of css:"#a" in the webview as y`,
	`store text of css:"#a" on webview as y`,
	`store text of css:"#a" webview as y`,
	`store css:"#a" as y`,
	`store css:"#a" in webview as y`,
	`store "x" in webview as y`,
	`store value of css:"#a" in webview as y`,
	`store html of css:"#a" in webview as y`,
}

func storeGoldenPath() string { return "testdata/store_webview_compat.golden.json" }

func TestStoreCompatCorpusParsesAsBefore(t *testing.T) {
	if len(storeCompatCases) < 70 {
		t.Fatalf("the corpus has %d cases, want at least 70", len(storeCompatCases))
	}
	current := map[string]string{}
	for _, c := range storeCompatCases {
		current[c] = compatDump(compatSource(c))
	}
	if os.Getenv("STORE_COMPAT_UPDATE") == "1" {
		b, err := json.MarshalIndent(current, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(storeGoldenPath(), b, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Skip("golden file written")
	}
	raw, err := os.ReadFile(storeGoldenPath())
	if err != nil {
		t.Fatal(err)
	}
	golden := map[string]string{}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range current {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, c := range keys {
		want, ok := golden[c]
		if !ok {
			t.Errorf("no recorded parse for %q (re-record with STORE_COMPAT_UPDATE=1 on a parser without the step)", c)
			continue
		}
		if current[c] != want {
			t.Errorf("%q parses differently than before:\n  before: %s\n  now:    %s", c, want, current[c])
		}
	}
	for c := range golden {
		if _, ok := current[c]; !ok {
			t.Errorf("the golden file has %q, which is not in the corpus any more", c)
		}
	}
}
