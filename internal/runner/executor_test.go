package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func newTestExecutor() *Executor {
	return NewExecutor(nil, nil, nil, 0, false)
}

// TestRunRecipeCall_UnknownRecipeErrors covers PT-02(a): an unrecognized
// recipe call used to silently no-op ("may be a filler line"), masking typos
// and broken recipe references indefinitely. It must now error.
func TestRunRecipeCall_UnknownRecipeErrors(t *testing.T) {
	e := newTestExecutor()
	err := e.runRecipeCall(context.Background(), parser.RecipeCall{
		Name: "this recipe does not exist",
		Line: 7,
	})
	if err == nil {
		t.Fatal("expected an error for an unknown recipe call, got nil")
	}
	if !strings.Contains(err.Error(), "line 7") {
		t.Errorf("error should reference the line number, got: %v", err)
	}
	if !strings.Contains(err.Error(), "this recipe does not exist") {
		t.Errorf("error should name the unresolved call, got: %v", err)
	}
}

// TestRunRecipeCall_KnownRecipeStillWorks confirms the fix didn't break the
// existing exact-match path.
func TestRunRecipeCall_KnownRecipeStillWorks(t *testing.T) {
	e := newTestExecutor()
	e.RegisterRecipe(parser.RecipeDef{
		Name: "do nothing",
		Body: nil,
	})
	err := e.runRecipeCall(context.Background(), parser.RecipeCall{Name: "do nothing", Line: 3})
	if err != nil {
		t.Fatalf("expected a known recipe with an empty body to succeed, got: %v", err)
	}
}

// TestRunRecipeCall_KnownRecipeViaStrippedFillersStillWorks confirms the
// fuzzy fallback match (stripping <arg>/filler words) still succeeds and
// doesn't spuriously error.
func TestRunRecipeCall_KnownRecipeViaStrippedFillersStillWorks(t *testing.T) {
	e := newTestExecutor()
	e.RegisterRecipe(parser.RecipeDef{Name: "sign in", Body: nil})
	err := e.runRecipeCall(context.Background(), parser.RecipeCall{
		Name: "sign in with <arg> and <arg>",
		Args: []string{"user@test.com", "pw"},
		Line: 5,
	})
	if err != nil {
		t.Fatalf("expected the filler-stripped match to succeed, got: %v", err)
	}
}

// TestResolve_SelfReferentialBindingDoesNotHang covers PT-02(c): a variable
// bound to a value containing its own placeholder marker (e.g. passing the
// unquoted literal "<email>" as a recipe argument) used to make resolve()
// loop forever, hanging the CLI with no error. This must now terminate.
func TestResolve_SelfReferentialBindingDoesNotHang(t *testing.T) {
	e := newTestExecutor()
	e.vars["email"] = "<email>" // self-referential binding

	done := make(chan string, 1)
	go func() { done <- e.resolve("type <email>") }()

	select {
	case result := <-done:
		// No progress could ever be made on a truly self-referential
		// binding, so the placeholder is left unresolved rather than
		// silently fabricating a value.
		if result != "type <email>" {
			t.Errorf("resolve() = %q, want the input left unchanged", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resolve() did not return within 2s — self-referential binding caused a hang")
	}
}

// TestResolve_NormalSubstitutionStillWorks confirms the iteration cap didn't
// break ordinary (non-self-referential) substitution.
func TestResolve_NormalSubstitutionStillWorks(t *testing.T) {
	e := newTestExecutor()
	e.vars["email"] = "user@test.com"
	got := e.resolve(`type "<email>" into the field`)
	want := `type "user@test.com" into the field`
	if got != want {
		t.Errorf("resolve() = %q, want %q", got, want)
	}
}

// TestResolve_RepeatedPlaceholderStillWorks confirms multiple occurrences of
// the same (non-self-referential) placeholder all get substituted, not just
// the first — the iteration cap must not cut this short.
func TestResolve_RepeatedPlaceholderStillWorks(t *testing.T) {
	e := newTestExecutor()
	e.vars["x"] = "5"
	got := e.resolve("<x> <x> <x>")
	if got != "5 5 5" {
		t.Errorf("resolve() = %q, want %q", got, "5 5 5")
	}
}
