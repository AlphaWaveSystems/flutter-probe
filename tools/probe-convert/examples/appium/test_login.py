import unittest
import time
from appium import webdriver
from appium.webdriver.common.appiumby import AppiumBy
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC


class LoginTest(unittest.TestCase):
    def setUp(self):
        caps = {
            'platformName': 'Android',
            'app': '/path/to/app.apk',
        }
        self.driver = webdriver.Remote('http://localhost:4723', caps)

    def tearDown(self):
        self.driver.quit()

    def test_login_success(self):
        self.driver.find_element(AppiumBy.ACCESSIBILITY_ID, 'Sign In').click()
        self.driver.find_element(AppiumBy.ID, 'emailInput').click()
        self.driver.find_element(AppiumBy.ID, 'emailInput').send_keys('user@test.com')
        self.driver.find_element(AppiumBy.ID, 'passwordInput').send_keys('pass123')
        self.driver.find_element(AppiumBy.ID, 'loginButton').click()
        WebDriverWait(self.driver, 10).until(
            EC.visibility_of_element_located((AppiumBy.ID, 'dashboardTitle'))
        )

    def test_login_wrong_credentials(self):
        self.driver.find_element(AppiumBy.ACCESSIBILITY_ID, 'Sign In').click()
        self.driver.find_element(AppiumBy.ID, 'emailInput').send_keys('wrong@test.com')
        self.driver.find_element(AppiumBy.ID, 'passwordInput').send_keys('badpass')
        self.driver.find_element(AppiumBy.ID, 'loginButton').click()
        time.sleep(3)
        self.driver.save_screenshot('wrong_creds.png')

    def test_navigate_back(self):
        self.driver.find_element(AppiumBy.ACCESSIBILITY_ID, 'Sign In').click()
        self.driver.back()
        WebDriverWait(self.driver, 5).until(
            EC.visibility_of_element_located((AppiumBy.ACCESSIBILITY_ID, 'Welcome'))
        )

    def test_xpath_login(self):
        self.driver.find_element(AppiumBy.XPATH, '//android.widget.Button[@text="Sign In"]').click()
        self.driver.find_element(AppiumBy.XPATH, '//android.widget.EditText[@content-desc="Email"]').send_keys('user@test.com')


if __name__ == '__main__':
    unittest.main()
