Feature: Variables written without quotes

  Scenario Outline: Fill a form with outline variables
    Given I launch the app
    When I type <email> into "Email"
    And I enter <password> in the "Password" field
    And I fill the <field> field with "x"
    And I type <value> into <target>
    And I tap on <button>
    Then I should see "<expected>"

    Examples:
      | email         | password | field | value | target | button | expected |
      | a@example.com | secret1  | Name  | v1    | Notes  | Go     | Done     |
