package runner

import (
	"reflect"
	"testing"
)

func setOf(names ...string) func(string) bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return func(n string) bool { return m[n] }
}

func TestUnsetEnvRefsFindsUnsetReferencesInStepStrings(t *testing.T) {
	src := "test \"login\"\n  open the app\n  type \"${USER_EMAIL}\" into the \"email\" field\n  type \"${API_TOKEN}\" into the \"token\" field\n  tap \"${USER_EMAIL}\"\n"
	got, err := UnsetEnvRefs(src, setOf("API_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	want := []EnvRef{{Name: "USER_EMAIL", Line: 3}, {Name: "USER_EMAIL", Line: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if none, _ := UnsetEnvRefs(src, setOf("USER_EMAIL", "API_TOKEN")); len(none) != 0 {
		t.Errorf("all set, got %+v", none)
	}
}

// A reference in a comment is not a step string: the parser drops comments, and so does this.
func TestUnsetEnvRefsIgnoresCommentsAndTestNames(t *testing.T) {
	src := "# set ${IN_A_COMMENT} before you run this\ntest \"uses ${IN_A_NAME}\"\n  # tap \"${COMMENTED_OUT}\"\n  open the app\n"
	got, err := UnsetEnvRefs(src, setOf())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got {
		if r.Name == "IN_A_COMMENT" || r.Name == "COMMENTED_OUT" {
			t.Errorf("a comment gave a reference: %+v", r)
		}
	}
}

func TestUnsetEnvRefsReadsHooksRecipesAndNestedSteps(t *testing.T) {
	src := "recipe \"sign in\"\n  type \"${IN_RECIPE}\" into the \"email\" field\n\nbefore each test\n  type \"${IN_HOOK}\" into the \"x\" field\n\ntest \"t\"\n  if visible \"Welcome\"\n    tap \"${NESTED}\"\n  open the app\n"
	got, err := UnsetEnvRefs(src, setOf())
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, r := range got {
		names[r.Name] = true
	}
	for _, n := range []string{"IN_RECIPE", "IN_HOOK", "NESTED"} {
		if !names[n] {
			t.Errorf("%s was not found in %+v", n, got)
		}
	}
}

// Only the braced form is a reference, as the engine reads it; a bare $ stays text.
func TestUnsetEnvRefsReadsTheBracedFormOnly(t *testing.T) {
	got, err := UnsetEnvRefs("test \"t\"\n  tap \"$5 and $name and ${} and ${1BAD} and ${OK_1}\"\n", setOf())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "OK_1" {
		t.Errorf("got %+v, want only OK_1", got)
	}
}

func TestUnsetEnvRefsReturnsTheParseError(t *testing.T) {
	if _, err := UnsetEnvRefs("test \"t\"\n  tap \"unterminated\n", setOf()); err == nil {
		t.Error("a text that does not parse gave no error")
	}
}
