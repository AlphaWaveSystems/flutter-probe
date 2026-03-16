Feature: Checkout
  Verify the end-to-end checkout flow

  Background:
    Given the app is launched
    And I tap on "Sign In"
    And I type "user@test.com" into "Email"
    And I type "pass123" into "Password"
    And I tap on "Continue"

  @cart
  Scenario: Add item to cart
    When I tap on "Shop"
    And I tap on "Running Shoes"
    And I tap on "Add to Cart"
    Then I should see "Cart (1)"

  @cart
  Scenario: Remove item from cart
    When I tap on "Cart"
    And I long press on "Running Shoes"
    And I tap on "Remove"
    Then I should see "Cart is empty"

  Scenario: Swipe through products
    When I tap on "Shop"
    And I swipe left
    And I swipe left
    Then I should see "Featured"
    And I take a screenshot
