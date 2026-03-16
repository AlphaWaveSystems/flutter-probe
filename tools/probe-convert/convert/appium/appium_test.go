package appium

import (
	"strings"
	"testing"
)

// ---- Python tests ----

func TestConvert_PythonBasic(t *testing.T) {
	py := `
import unittest
from appium import webdriver
from appium.webdriver.common.appiumby import AppiumBy

class LoginTest(unittest.TestCase):
    def setUp(self):
        self.driver = webdriver.Remote('http://localhost:4723', caps)

    def tearDown(self):
        self.driver.quit()

    def test_login_success(self):
        self.driver.find_element(AppiumBy.ID, 'emailInput').click()
        self.driver.find_element(AppiumBy.ID, 'emailInput').send_keys('user@test.com')
        self.driver.find_element(AppiumBy.ACCESSIBILITY_ID, 'Sign In').click()
        time.sleep(3)
`
	c := New()
	result, err := c.Convert([]byte(py), "test_login.py")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "before each")
	assertContains(t, result.ProbeCode, "after each")
	assertContains(t, result.ProbeCode, `test "login success"`)
	assertContains(t, result.ProbeCode, "tap on #emailInput")
	assertContains(t, result.ProbeCode, `type "user@test.com" into #emailInput`)
	assertContains(t, result.ProbeCode, `tap on "Sign In"`)
	assertContains(t, result.ProbeCode, "wait 3 seconds")
}

func TestConvert_PythonWait(t *testing.T) {
	py := `
class WaitTest(unittest.TestCase):
    def test_wait_for_element(self):
        WebDriverWait(self.driver, 10).until(
            EC.visibility_of_element_located((AppiumBy.ID, 'spinner'))
        )
`
	c := New()
	result, err := c.Convert([]byte(py), "test_wait.py")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "wait until #spinner appears")
}

func TestConvert_PythonXPath(t *testing.T) {
	py := `
class XPathTest(unittest.TestCase):
    def test_xpath_click(self):
        self.driver.find_element(AppiumBy.XPATH, '//android.widget.Button[@text="Submit"]').click()
`
	c := New()
	result, err := c.Convert([]byte(py), "test_xpath.py")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `tap on "Submit"`)
}

func TestConvert_PythonBack(t *testing.T) {
	py := `
class NavTest(unittest.TestCase):
    def test_navigate_back(self):
        self.driver.back()
`
	c := New()
	result, err := c.Convert([]byte(py), "test_nav.py")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "go back")
}

// ---- Java tests ----

func TestConvert_JavaBasic(t *testing.T) {
	java := `
import io.appium.java_client.AppiumBy;

public class LoginTest {
    @Before
    public void setUp() {
        driver = new AndroidDriver(new URL("http://localhost:4723"), caps);
    }

    @After
    public void tearDown() {
        driver.quit();
    }

    @Test
    public void testLoginWithEmail() {
        driver.findElement(AppiumBy.id("emailInput")).click();
        driver.findElement(AppiumBy.id("emailInput")).sendKeys("user@test.com");
        driver.findElement(AppiumBy.accessibilityId("Sign In")).click();
        Thread.sleep(3000);
    }
}
`
	c := New()
	result, err := c.Convert([]byte(java), "LoginTest.java")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "before each")
	assertContains(t, result.ProbeCode, "after each")
	assertContains(t, result.ProbeCode, `test "login with email"`)
	assertContains(t, result.ProbeCode, "tap on #emailInput")
	assertContains(t, result.ProbeCode, `type "user@test.com" into #emailInput`)
	assertContains(t, result.ProbeCode, `tap on "Sign In"`)
	assertContains(t, result.ProbeCode, "wait 3 seconds")
}

func TestConvert_JavaCamelCase(t *testing.T) {
	java := `
public class CheckoutTest {
    @Test
    public void testAddItemToCartAndCheckout() {
        driver.findElement(By.id("addBtn")).click();
    }
}
`
	c := New()
	result, err := c.Convert([]byte(java), "CheckoutTest.java")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `test "add item to cart and checkout"`)
}

// ---- JS tests ----

func TestConvert_JSBasic(t *testing.T) {
	js := `
describe('Login', () => {
  it('should login', async () => {
    await $('~emailInput').click();
    await $('~emailInput').setValue('user@test.com');
    await $('#loginBtn').click();
    await browser.pause(2000);
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "login.test.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `test "should login"`)
	assertContains(t, result.ProbeCode, `tap on "emailInput"`)
	assertContains(t, result.ProbeCode, `type "user@test.com" into "emailInput"`)
	assertContains(t, result.ProbeCode, "tap on #loginBtn")
	assertContains(t, result.ProbeCode, "wait 2 seconds")
}

func TestConvert_JSBack(t *testing.T) {
	js := `
describe('Nav', () => {
  it('goes back', async () => {
    await browser.back();
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "nav.test.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "go back")
}

func TestConvert_JSScreenshot(t *testing.T) {
	js := `
describe('Screenshot', () => {
  it('takes screenshot', async () => {
    await browser.saveScreenshot('home.png');
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "screen.test.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `take a screenshot called "home.png"`)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q\ngot:\n%s", needle, haystack)
	}
}
