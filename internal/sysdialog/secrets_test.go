package sysdialog

import "testing"

func TestScrubKnownMasksNotedSecretsLongestFirst(t *testing.T) {
	NoteSecret("")
	NoteSecret("kn-short")
	NoteSecret("kn-short-and-more")
	NoteSecret("kn-short")
	if got, want := ScrubKnown("a kn-short-and-more b kn-short c"), "a **** b **** c"; got != want {
		t.Fatalf("ScrubKnown = %q, want %q", got, want)
	}
	if got := ScrubKnown("nothing to mask here"); got != "nothing to mask here" {
		t.Fatalf("ScrubKnown changed text that holds no secret: %q", got)
	}
}
