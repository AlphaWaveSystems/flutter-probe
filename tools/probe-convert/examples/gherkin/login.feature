Feature: Login
  Users should be able to log in with valid credentials

  Background:
    Given the app is launched
    And I wait until "Sign In" appears

  @smoke @critical
  Scenario: User can log in with email
    When I tap on "Sign In"
    And I type "user@test.com" into "Email"
    And I type "pass123" into "Password"
    And I tap on "Continue"
    Then I should see "Dashboard"

  @smoke
  Scenario: User sees error for wrong credentials
    When I tap on "Sign In"
    And I type "wrong@test.com" into "Email"
    And I type "badpass" into "Password"
    And I tap on "Continue"
    Then I should see "Invalid credentials"
    And I should not see "Dashboard"

  Scenario: User can navigate back from login
    When I tap on "Sign In"
    And I go back
    Then I should see "Welcome"
