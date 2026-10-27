package sysdialog

import (
	"encoding/json"
	"strings"
	"testing"
)

// A JSON body can hold a value with <, > and & written as escapes, so a recorded body or a printed JSON document carries
// the value in that form. json.Marshal writes the escapes, and ScrubKnown must mask the escaped form.
func TestScrubKnownMasksJSONEscapedForm(t *testing.T) {
	const value = "pa<ss>&word-zq-7731"
	NoteSecret(value)
	escaped, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"token":` + string(escaped) + `}`
	if strings.Contains(body, value) {
		t.Fatalf("the body holds the raw value, not its JSON-escaped form: %s", body)
	}
	if got, want := ScrubKnown(body), `{"token":"****"}`; got != want {
		t.Fatalf("ScrubKnown(%s) = %s, want %s", body, got, want)
	}
}
