*** Settings ***
Library           AppiumLibrary
Suite Setup       Open Application    http://localhost:4723    platformName=Android    app=com.example.app
Suite Teardown    Close Application

*** Variables ***
${EMAIL}          user@test.com
${PASSWORD}       pass123

*** Test Cases ***
Login With Valid Credentials
    [Tags]    smoke    critical
    Click Element    text=Sign In
    Input Text    id=emailField    ${EMAIL}
    Input Text    id=passwordField    ${PASSWORD}
    Click Element    id=loginButton
    Wait Until Page Contains    Dashboard
    Capture Page Screenshot

Login With Wrong Credentials Shows Error
    [Tags]    smoke
    Click Element    text=Sign In
    Input Text    id=emailField    wrong@test.com
    Input Text    id=passwordField    badpass
    Click Element    id=loginButton
    Sleep    2s
    Page Should Contain Text    Invalid credentials
    Page Should Not Contain Text    Dashboard

Navigate Back From Login
    Click Element    text=Sign In
    Go Back
    Wait Until Page Contains    Welcome
    Page Should Contain Text    Welcome
