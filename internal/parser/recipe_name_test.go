package parser_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func TestSimulateRecipeCall(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params int
		want   parser.CallReading
		call   string // the name of the call the parser gives, when it is one recipe call
	}{
		{"sign in as", 2, parser.CallIsRecipeCall, "sign in as <arg> <arg>"},
		{"login as", 2, parser.CallIsRecipeCall, "login as <arg> <arg>"},
		{"add item to cart", 1, parser.CallIsRecipeCall, "add item to cart <arg>"},
		{"Sign In", 0, parser.CallIsRecipeCall, "sign in"}, // the parser lower-cases the call
		{"the login", 1, parser.CallIsRecipeCall, "login <arg>"},
		{"log in as", 2, parser.CallIsOtherStep, ""}, // log is a keyword: the log step plus a stray call
		{"take a break", 0, parser.CallIsOtherStep, ""},
		{"pause the app", 0, parser.CallIsOtherStep, ""},
		{"tap the logo", 1, parser.CallIsOtherStep, ""},
		{"go back", 0, parser.CallIsOtherStep, ""},
		{"restart everything", 0, parser.CallIsOtherStep, ""},
		// A filler word at the start of a step is dropped, so the call is not written with the recipe's name.
		{"on the home screen", 0, parser.CallIsRecipeCall, "home screen"},
		{"to", 1, parser.CallIsRecipeCall, "<arg>"},
		{"in the cart", 1, parser.CallIsRecipeCall, "cart <arg>"},
		{"is signed in", 0, parser.CallIsRecipeCall, "signed in"},
		{"the", 0, parser.CallIsNothing, ""}, // every word is a filler: no step at all
		{"the", 2, parser.CallIsRecipeCall, "<arg> <arg>"},
		{"", 0, parser.CallNotSimulated, ""},
		{`say "hi"`, 0, parser.CallNotSimulated, ""},
		{"fill <email>", 1, parser.CallNotSimulated, ""},
	} {
		call, how, first := parser.SimulateRecipeCall(tc.name, tc.params)
		if how != tc.want {
			t.Errorf("SimulateRecipeCall(%q, %d) = reading %d, want %d", tc.name, tc.params, how, tc.want)
			continue
		}
		if how == parser.CallIsRecipeCall && call.Name != tc.call {
			t.Errorf("SimulateRecipeCall(%q, %d) call name = %q, want %q", tc.name, tc.params, call.Name, tc.call)
		}
		if how != parser.CallNotSimulated && first == "" {
			t.Errorf("SimulateRecipeCall(%q, %d) gave no first word", tc.name, tc.params)
		}
	}
}
