package sysdialog

import "testing"

// A resolved value can reach printed text in a form other than the raw one. A %q-quoted value escapes a double quote and
// a backslash, and a URL or an error that quotes a request carries the value percent-encoded. ScrubKnown must mask those
// forms too, or they print in the clear.
func TestScrubKnownMasksQuotedAndEncodedForms(t *testing.T) {
	NoteSecret(`qt-"x"\y`)   // a double quote and a backslash: %q prints it as qt-\"x\"\\y
	NoteSecret("ops@mail-1") // an at sign: url.QueryEscape prints it as %40
	NoteSecret("pw p+1")     // a space and a plus: QueryEscape gives pw+p%2B1, PathEscape gives pw%20p+1

	for _, tc := range []struct{ name, printed, want string }{
		{"%q of the value with a quote and a backslash", `"qt-\"x\"\\y"`, `"****"`},
		{"query-escaped value with a quote and a backslash", "https://h/p?k=qt-%22x%22%5Cy", "https://h/p?k=****"},
		{"path-escaped value with a quote and a backslash", "https://h/qt-%22x%22%5Cy", "https://h/****"},
		{"query-escaped value with an at sign", "https://h/p?user=ops%40mail-1", "https://h/p?user=****"},
		{"%q of the value with an at sign", `"ops@mail-1"`, `"****"`},
		{"query-escaped value with a space and a plus", "https://h/p?q=pw+p%2B1", "https://h/p?q=****"},
		{"path-escaped value with a space and a plus", "https://h/pw%20p+1", "https://h/****"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ScrubKnown(tc.printed); got != tc.want {
				t.Fatalf("ScrubKnown(%q) = %q, want %q", tc.printed, got, tc.want)
			}
		})
	}
}
