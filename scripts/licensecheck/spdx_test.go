package main

import (
	"strings"
	"testing"
)

// The cases of the old check, measured on 2026-11-16 with `license-checker --failOn "GPL-2.0;GPL-3.0;AGPL-1.0;AGPL-3.0;SSPL-1.0;EUPL-1.1;EUPL-1.2"` on fake packages:
// "GPL-3.0" exited 1 (caught); "GPL-3.0-or-later", "GPL-2.0-only", "AGPL-3.0-only", "(GPL-3.0 AND MIT)" and "(MIT OR GPL-3.0)" all exited 0 (not caught). The table below is
// what this check says about each of them, and about the rest.
func TestEvaluate(t *testing.T) {
	cases := []struct {
		expr   string
		strong bool   // a strong copyleft licence is mandatory: the package fails
		note   string // a text every note list must contain (empty: no note at all is wanted when the field wantNoNote is true)
	}{
		// permissive: nothing to say
		{"MIT", false, ""}, {"ISC", false, ""}, {"Apache-2.0", false, ""}, {"BSD-3-Clause", false, ""}, {"MIT*", false, ""},
		{"(MIT OR Apache-2.0)", false, ""}, {"MPL-2.0", false, ""}, {"Artistic-2.0", false, ""}, {"Apache-2.0 WITH LLVM-exception", false, ""},
		{"(MPL-2.0 OR Apache-2.0)", false, ""},
		// a strong licence in every spelling the old list missed
		{"GPL-3.0", true, ""}, {"GPL-2.0", true, ""}, {"GPL-3.0-or-later", true, ""}, {"GPL-2.0-only", true, ""}, {"GPL-3.0+", true, ""},
		{"AGPL-3.0", true, ""}, {"AGPL-3.0-only", true, ""}, {"AGPL-1.0", true, ""}, {"SSPL-1.0", true, ""}, {"EUPL-1.2", true, ""}, {"EUPL-1.1", true, ""},
		{"gpl-3.0", true, ""}, {"GPLv3", true, ""}, {"GPL-3.0*", true, ""},
		// mandatory: an AND operand, or every choice
		{"(GPL-3.0 AND MIT)", true, ""}, {"(MIT AND GPL-3.0-only)", true, ""}, {"MIT and GPL-3.0", true, ""},
		{"(GPL-2.0 OR GPL-3.0)", true, ""}, {"(GPL-2.0-only OR AGPL-3.0-or-later)", true, ""},
		{"MIT AND (GPL-2.0 OR AGPL-3.0)", true, ""},
		// a choice that includes a strong licence is free to take: a note, no failure
		{"(MIT OR GPL-3.0)", false, "one of the choices is a strong copyleft licence"},
		{"MIT AND (Apache-2.0 OR GPL-3.0)", false, "one of the choices is a strong copyleft licence"},
		{"(GPL-2.0 OR MIT OR Apache-2.0)", false, "one of the choices is a strong copyleft licence"},
		// an exception: read it, do not fail
		{"GPL-2.0 WITH Classpath-exception-2.0", false, "with an exception"},
		{"GPL-2.0-with-classpath-exception", false, "with an exception"},
		{"(GPL-3.0-only WITH GCC-exception-3.1 AND MIT)", false, "with an exception"},
		// weak copyleft, unknown, custom: a note
		{"LGPL-3.0-or-later", false, "weak copyleft"}, {"LGPL-2.1", false, "weak copyleft"}, {"(LGPL-2.1 OR MIT)", false, "weak copyleft"},
		{"UNKNOWN", false, "no licence is declared"}, {"UNLICENSED", false, "no licence is declared"}, {"", false, "no licence is declared"},
		{"Custom: https://example.org/licence", false, "custom licence"}, {"SEE LICENSE IN LICENSE.md", false, "custom licence"},
		// what cannot be read: a note, unless it names a strong licence (then it fails, closed)
		{"(MIT OR", false, "cannot be read"}, {"Some Proprietary Terms", false, "cannot be read"},
		{"MIT, GPL-3.0", true, "names a strong copyleft licence"}, {"GPL-3.0 AND (", true, "names a strong copyleft licence"}, {"(GPL-2.0-only OR", true, "names a strong"},
		// one identifier that is not a known strong one but contains a strong licence's name: fail closed (the six of the review of this check)
		{"LicenseRef-GPL-3.0", true, "names a strong copyleft licence"}, {"GNU-GPL-3.0", true, "names a strong copyleft licence"},
		{"MIT/GPL-3.0", true, "names a strong copyleft licence"}, {"MIT,GPL-3.0", true, "names a strong copyleft licence"},
		{"MIT;GPL-3.0", true, "names a strong copyleft licence"}, {"MIT\u00a0AND\u00a0GPL-3.0", true, ""}, {"MIT\u2003AND\u2003AGPL-3.0-only", true, ""},
		{"(MIT\u00a0OR\u00a0GPL-3.0)", false, "one of the choices is a strong copyleft licence"}, // read as a choice, not as one strange identifier
		{"LicenseRef-AGPL", true, "names a strong copyleft licence"}, {"Foo-SSPL-1.0", true, "names a strong copyleft licence"},
		// precedence: AND binds tighter than OR, with no parentheses at the top level
		{"MIT OR GPL-3.0", false, "one of the choices is a strong copyleft licence"},
		{"GPL-3.0 OR MIT AND ISC", false, "one of the choices is a strong copyleft licence"},
		{"MIT AND GPL-3.0 OR Apache-2.0", false, "one of the choices is a strong copyleft licence"},
		{"GPL-3.0 AND MIT OR GPL-2.0", true, ""},
		{"MIT AND ISC OR Apache-2.0", false, ""},
		{"GPL-2.0-only OR GPL-3.0-only OR AGPL-3.0-only", true, ""},
		// names that merely look like a strong licence, or contain one inside a longer word
		{"LGPL-3.0-only", false, "weak copyleft"}, {"LicenseRef-LGPL-2.1", false, ""}, {"Apache-2.0", false, ""}, {"MIT-0", false, ""},
		// names that only look like a strong licence
		{"Apache-2.0", false, ""}, {"Unlicense", false, ""}, {"BlueOak-1.0.0", false, ""},
	}
	for _, tc := range cases {
		got := evaluate(tc.expr)
		if got.strong != tc.strong {
			t.Errorf("%q: strong = %v, want %v (notes %q)", tc.expr, got.strong, tc.strong, got.notes)
		}
		joined := strings.Join(got.notes, " | ")
		if tc.note != "" && !strings.Contains(joined, tc.note) {
			t.Errorf("%q: notes %q lack %q", tc.expr, got.notes, tc.note)
		}
		if tc.note == "" && !tc.strong && len(got.notes) != 0 {
			t.Errorf("%q: unexpected notes %q", tc.expr, got.notes)
		}
	}
}

func TestSplitNameVersion(t *testing.T) {
	for in, want := range map[string][2]string{
		"left-pad@1.3.0":                 {"left-pad", "1.3.0"},
		"@vscode/vsce-sign@2.1.1":        {"@vscode/vsce-sign", "2.1.1"},
		"@img/sharp-libvips-linux-x64@*": {"@img/sharp-libvips-linux-x64", "*"},
		"nover":                          {"nover", ""},
		"@scope/only":                    {"@scope/only", ""},
	} {
		n, v := splitNameVersion(in)
		if n != want[0] || v != want[1] {
			t.Errorf("%q -> (%q, %q), want %v", in, n, v, want)
		}
	}
}
