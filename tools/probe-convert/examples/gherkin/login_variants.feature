Feature: Login variants
  Test login with different user roles

  @parametrized
  Scenario Outline: Login with <role> credentials
    Given the app is launched
    When I tap on "Sign In"
    And I type "<email>" into "Email"
    And I type "<password>" into "Password"
    And I tap on "Continue"
    Then I should see "<expected>"

  Examples:
    | role   | email            | password | expected       |
    | admin  | admin@test.com   | admin123 | Admin Panel    |
    | user   | user@test.com    | pass123  | Dashboard      |
    | guest  | guest@test.com   | guest1   | Limited Access |
