Feature: Permissions
  Verify the app handles permission dialogs correctly

  @permissions
  Scenario: Grant camera permission
    Given the app is launched
    When I allow the "camera" permission
    And I tap on "Take Photo"
    Then I should see "Camera Preview"

  @permissions
  Scenario: Deny location permission
    Given the app is launched
    When I deny the "location" permission
    And I tap on "Find Nearby"
    Then I should see "Location permission required"

  @permissions
  Scenario: Grant all permissions at once
    Given the app is launched
    When I grant all permissions
    And I tap on "Full Access"
    Then I should see "All features unlocked"
