package probelink

import (
	"encoding/json"
	"testing"
)

// The newer keys of visible_summary say whether the snapshot is complete. A reader must be able to tell "an older agent that does not say" from "a newer
// agent that saw none": an absent key and an empty list must not look the same, and a key that is not of its own type is absent.

func decodeSummary(t *testing.T, doc string) (VisibleSummaryResult, error) {
	t.Helper()
	var r VisibleSummaryResult
	err := json.Unmarshal([]byte(doc), &r)
	return r, err
}

func TestVisibleSummaryDetailIsReadOnlyWhenAllOfItIsThere(t *testing.T) {
	full := `{"texts":["Sign In"],"keys":["k"],"editable_texts":["abc"],"rich_texts":["rich"],"max":25,"counts":{"texts":1,"keys":1,"editable_texts":1,"rich_texts":1},"truncated":false}`
	r, err := decodeSummary(t, full)
	if err != nil || r.Detail == nil {
		t.Fatalf("%+v, %v", r, err)
	}
	d := r.Detail
	if len(r.Texts) != 1 || r.Texts[0] != "Sign In" || len(d.EditableTexts) != 1 || d.EditableTexts[0] != "abc" || len(d.RichTexts) != 1 || d.Max != 25 || d.Truncated || d.Counts["texts"] != 1 {
		t.Errorf("%+v %+v", r, d)
	}
	// a newer agent that saw none: the lists are there and empty, which is not the same as absent
	r, err = decodeSummary(t, `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":[],"max":25,"truncated":false}`)
	if err != nil || r.Detail == nil || r.Detail.EditableTexts == nil || len(r.Detail.EditableTexts) != 0 {
		t.Errorf("empty lists: %+v, %v", r, err)
	}
	r, _ = decodeSummary(t, `{"texts":["a"],"keys":[],"editable_texts":[],"rich_texts":[],"max":2,"counts":{"texts":9},"truncated":true}`)
	if r.Detail == nil || !r.Detail.Truncated || r.Detail.Counts["texts"] != 9 {
		t.Errorf("truncated: %+v", r.Detail)
	}
}

func TestVisibleSummaryDetailIsAbsentForAnOlderAgentAndForAnythingMalformed(t *testing.T) {
	for name, doc := range map[string]string{
		"an older agent":                `{"texts":["a"],"keys":["k"]}`,
		"no editable_texts":             `{"texts":[],"keys":[],"rich_texts":[],"max":25,"truncated":false}`,
		"no rich_texts":                 `{"texts":[],"keys":[],"editable_texts":[],"max":25,"truncated":false}`,
		"no max":                        `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":[],"truncated":false}`,
		"no truncated":                  `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":[],"max":25}`,
		"editable_texts is a string":    `{"texts":[],"keys":[],"editable_texts":"abc","rich_texts":[],"max":25,"truncated":false}`,
		"editable_texts holds a number": `{"texts":[],"keys":[],"editable_texts":[1],"rich_texts":[],"max":25,"truncated":false}`,
		"editable_texts is null":        `{"texts":[],"keys":[],"editable_texts":null,"rich_texts":[],"max":25,"truncated":false}`,
		"rich_texts is an object":       `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":{},"max":25,"truncated":false}`,
		"max is zero":                   `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":[],"max":0,"truncated":false}`,
		"max is negative":               `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":[],"max":-3,"truncated":false}`,
		"max is a fraction":             `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":[],"max":2.5,"truncated":false}`,
		"max is a string":               `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":[],"max":"25","truncated":false}`,
		"truncated is a string":         `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":[],"max":25,"truncated":"false"}`,
		"truncated is a number":         `{"texts":[],"keys":[],"editable_texts":[],"rich_texts":[],"max":25,"truncated":0}`,
	} {
		r, err := decodeSummary(t, doc)
		if err != nil {
			t.Errorf("%s: decoding failed (a malformed newer key must not break the old fields): %v", name, err)
		}
		if r.Detail != nil {
			t.Errorf("%s: Detail = %+v, want nil", name, r.Detail)
		}
		if len(r.Texts) > 0 && r.Texts[0] != "a" {
			t.Errorf("%s: texts = %v", name, r.Texts)
		}
	}
}

// texts and keys keep their old strictness: a wrong type there is an error, as before.
func TestVisibleSummaryTextsAndKeysAreStillStrict(t *testing.T) {
	if _, err := decodeSummary(t, `{"texts":"a","keys":[]}`); err == nil {
		t.Error("texts as a string was accepted")
	}
	if _, err := decodeSummary(t, `{"texts":[],"keys":{}}`); err == nil {
		t.Error("keys as an object was accepted")
	}
	if _, err := decodeSummary(t, `[1]`); err == nil {
		t.Error("a list was accepted")
	}
}
