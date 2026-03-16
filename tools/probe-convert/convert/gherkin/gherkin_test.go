package gherkin

import (
	"strings"
	"testing"
)

func TestConvert_SimpleScenario(t *testing.T) {
	feature := `Feature: Login
  @smoke
  Scenario: User can log in
    Given the app is launched
    When I tap on "Sign In"
    And I type "user@test.com" into "Email"
    Then I should see "Dashboard"
`
	c := New()
	result, err := c.Convert([]byte(feature), "login.feature")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "Feature: Login")
	assertContains(t, result.ProbeCode, `test "User can log in"`)
	assertContains(t, result.ProbeCode, "@smoke")
	assertContains(t, result.ProbeCode, "open the app")
	assertContains(t, result.ProbeCode, `tap on "Sign In"`)
	assertContains(t, result.ProbeCode, `type "user@test.com" into "Email"`)
	assertContains(t, result.ProbeCode, `see "Dashboard"`)
}

func TestConvert_Background(t *testing.T) {
	feature := `Feature: Checkout
  Background:
    Given the app is launched
    And I tap on "Sign In"

  Scenario: Add to cart
    When I tap on "Add"
    Then I should see "Cart (1)"
`
	c := New()
	result, err := c.Convert([]byte(feature), "checkout.feature")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "before each")
	assertContains(t, result.ProbeCode, "open the app")
	assertContains(t, result.ProbeCode, `test "Add to cart"`)
	assertContains(t, result.ProbeCode, `tap on "Add"`)
}

func TestConvert_ScenarioOutline(t *testing.T) {
	feature := `Feature: Login variants
  Scenario Outline: Login with <role>
    Given the app is launched
    When I type "<email>" into "Email"
    And I type "<password>" into "Password"
    Then I should see "<expected>"

  Examples:
    | role   | email          | password | expected  |
    | admin  | admin@test.com | pass123  | Dashboard |
    | user   | user@test.com  | pass456  | Home      |
`
	c := New()
	result, err := c.Convert([]byte(feature), "outline.feature")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `test "Login with <role>"`)
	assertContains(t, result.ProbeCode, `type "<email>" into "Email"`)
	assertContains(t, result.ProbeCode, "with examples:")
	assertContains(t, result.ProbeCode, "admin")
}

func TestConvert_WaitAndSwipe(t *testing.T) {
	feature := `Feature: Nav
  Scenario: Scroll through
    When I wait 3 seconds
    And I swipe up
    And I wait until "Footer" appears
`
	c := New()
	result, err := c.Convert([]byte(feature), "nav.feature")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "wait 3 seconds")
	assertContains(t, result.ProbeCode, "swipe up")
	assertContains(t, result.ProbeCode, `wait until "Footer" appears`)
}

func TestConvert_Assertions(t *testing.T) {
	feature := `Feature: Visibility
  Scenario: Check elements
    Then I should see "Welcome"
    And I should not see "Error"
    And "Login" should be visible
`
	c := New()
	result, err := c.Convert([]byte(feature), "vis.feature")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `see "Welcome"`)
	assertContains(t, result.ProbeCode, `don't see "Error"`)
	assertContains(t, result.ProbeCode, `see "Login"`)
}

func TestConvert_UnmatchedStep(t *testing.T) {
	feature := `Feature: Custom
  Scenario: Unknown action
    Given some custom precondition that cannot be mapped
`
	c := New()
	result, err := c.Convert([]byte(feature), "custom.feature")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "# TODO:")
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for unmatched step")
	}
}

func TestConvert_MultipleTags(t *testing.T) {
	feature := `Feature: Tagged
  @smoke @critical @login
  Scenario: Tagged test
    Given the app is launched
`
	c := New()
	result, err := c.Convert([]byte(feature), "tagged.feature")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "@smoke")
	assertContains(t, result.ProbeCode, "@critical")
	assertContains(t, result.ProbeCode, "@login")
}

func TestConvert_Permissions(t *testing.T) {
	feature := `Feature: Permissions
  Scenario: Grant camera
    When I allow the "camera" permission
    And I grant all permissions
`
	c := New()
	result, err := c.Convert([]byte(feature), "perms.feature")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `allow permission "camera"`)
	assertContains(t, result.ProbeCode, "grant all permissions")
}

func TestConvert_LongPress(t *testing.T) {
	feature := `Feature: Gestures
  Scenario: Long press
    When I long press on "Delete"
`
	c := New()
	result, err := c.Convert([]byte(feature), "gestures.feature")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `long press on "Delete"`)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q\ngot:\n%s", needle, haystack)
	}
}
