*** Settings ***
Library           AppiumLibrary
Suite Setup       Open Application    http://localhost:4723    platformName=Android    app=com.example.app
Suite Teardown    Close Application

*** Keywords ***
Login As User
    [Arguments]    ${email}    ${password}
    Click Element    text=Sign In
    Input Text    id=emailField    ${email}
    Input Text    id=passwordField    ${password}
    Click Element    id=loginButton
    Wait Until Page Contains    Dashboard

Add Item To Cart
    [Arguments]    ${item_name}
    Click Element    text=Shop
    Click Text    ${item_name}
    Click Element    id=addToCartBtn
    Sleep    1s

*** Test Cases ***
Add Product To Cart
    [Tags]    cart    smoke
    Login As User    user@test.com    pass123
    Add Item To Cart    Running Shoes
    Page Should Contain Text    Cart (1)
    Capture Page Screenshot

Remove Product From Cart
    [Tags]    cart
    Login As User    user@test.com    pass123
    Add Item To Cart    Running Shoes
    Click Element    text=Cart
    Long Press    id=cartItem1
    Click Element    text=Remove
    Page Should Contain Text    Cart is empty

Check Product Visibility
    [Tags]    ui
    Login As User    user@test.com    pass123
    Click Element    text=Shop
    Wait Until Element Is Visible    id=productList
    Element Should Be Visible    id=featuredBanner
    Element Should Not Be Visible    id=loadingSpinner
