package robot

import (
	"strings"
	"testing"
)

func TestConvert_BasicTestCase(t *testing.T) {
	robot := `*** Test Cases ***
Login Test
    [Tags]    smoke    critical
    Open Application    http://localhost:4723    platformName=Android
    Click Element    id=loginButton
    Input Text    id=emailField    user@test.com
    Page Should Contain Text    Dashboard
    Capture Page Screenshot
`
	c := New()
	result, err := c.Convert([]byte(robot), "login.robot")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `test "Login Test"`)
	assertContains(t, result.ProbeCode, "@smoke")
	assertContains(t, result.ProbeCode, "@critical")
	assertContains(t, result.ProbeCode, "tap on #loginButton")
	assertContains(t, result.ProbeCode, `type "user@test.com" into #emailField`)
	assertContains(t, result.ProbeCode, `see "Dashboard"`)
	assertContains(t, result.ProbeCode, "take a screenshot")
}

func TestConvert_Keywords(t *testing.T) {
	robot := `*** Keywords ***
Login With Credentials
    [Arguments]    ${email}    ${password}
    Click Element    id=emailField
    Input Text    id=emailField    ${email}
    Input Text    id=passField    ${password}
    Click Element    id=loginBtn
`
	c := New()
	result, err := c.Convert([]byte(robot), "keywords.robot")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `recipe "Login With Credentials"`)
	assertContains(t, result.ProbeCode, "tap on #emailField")
	assertContains(t, result.ProbeCode, "<email>")
	assertContains(t, result.ProbeCode, "<password>")
}

func TestConvert_SuiteSetupTeardown(t *testing.T) {
	robot := `*** Settings ***
Suite Setup    Open Application    http://localhost:4723
Suite Teardown    Close Application
`
	c := New()
	result, err := c.Convert([]byte(robot), "setup.robot")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "before each")
	assertContains(t, result.ProbeCode, "after each")
}

func TestConvert_WaitAndSleep(t *testing.T) {
	robot := `*** Test Cases ***
Wait Test
    Wait Until Element Is Visible    id=spinner
    Sleep    5s
    Wait Until Page Contains    Ready
`
	c := New()
	result, err := c.Convert([]byte(robot), "wait.robot")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "wait until #spinner appears")
	assertContains(t, result.ProbeCode, "wait 5 seconds")
	assertContains(t, result.ProbeCode, `wait until "Ready" appears`)
}

func TestConvert_NegativeAssertions(t *testing.T) {
	robot := `*** Test Cases ***
Visibility Check
    Page Should Not Contain Text    Error
    Element Should Not Be Visible    id=loadingSpinner
`
	c := New()
	result, err := c.Convert([]byte(robot), "neg.robot")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `don't see "Error"`)
	assertContains(t, result.ProbeCode, `don't see #loadingSpinner`)
}

func TestConvert_TextLocator(t *testing.T) {
	robot := `*** Test Cases ***
Text Click
    Click Element    text=Sign In
    Click Text    Continue
`
	c := New()
	result, err := c.Convert([]byte(robot), "text.robot")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `tap on "Sign In"`)
	assertContains(t, result.ProbeCode, `tap on "Continue"`)
}

func TestConvert_VariableSubstitution(t *testing.T) {
	robot := `*** Test Cases ***
Parameterized
    Input Text    id=email    ${EMAIL}
    Input Text    id=pass    ${PASSWORD}
`
	c := New()
	result, err := c.Convert([]byte(robot), "vars.robot")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "<email>")
	assertContains(t, result.ProbeCode, "<password>")
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q\ngot:\n%s", needle, haystack)
	}
}
