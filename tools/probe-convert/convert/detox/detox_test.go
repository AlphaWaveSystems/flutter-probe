package detox

import (
	"strings"
	"testing"
)

func TestConvert_BasicDetox(t *testing.T) {
	js := `
describe('Login', () => {
  beforeEach(async () => {
    await device.launchApp();
  });

  it('should log in successfully', async () => {
    await element(by.id('emailInput')).typeText('user@test.com');
    await element(by.id('passwordInput')).typeText('pass123');
    await element(by.text('Sign In')).tap();
    await expect(element(by.text('Dashboard'))).toBeVisible();
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "login.spec.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "before each")
	assertContains(t, result.ProbeCode, "open the app")
	assertContains(t, result.ProbeCode, `test "should log in successfully"`)
	assertContains(t, result.ProbeCode, `type "user@test.com" into #emailInput`)
	assertContains(t, result.ProbeCode, `tap on "Sign In"`)
	assertContains(t, result.ProbeCode, `see "Dashboard"`)
}

func TestConvert_Assertions(t *testing.T) {
	js := `
describe('Visibility', () => {
  it('checks elements', async () => {
    await expect(element(by.text('Welcome'))).toBeVisible();
    await expect(element(by.text('Error'))).not.toBeVisible();
    await expect(element(by.id('title'))).toHaveText('Hello');
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "vis.spec.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `see "Welcome"`)
	assertContains(t, result.ProbeCode, `don't see "Error"`)
	assertContains(t, result.ProbeCode, `see "Hello"`)
}

func TestConvert_WaitFor(t *testing.T) {
	js := `
describe('Wait', () => {
  it('waits for element', async () => {
    await waitFor(element(by.text('Loading'))).toBeVisible();
    await waitFor(element(by.id('spinner'))).toBeVisible();
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "wait.spec.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, `wait until "Loading" appears`)
	assertContains(t, result.ProbeCode, "wait until #spinner appears")
}

func TestConvert_Gestures(t *testing.T) {
	js := `
describe('Gestures', () => {
  it('performs gestures', async () => {
    await element(by.id('item')).longPress();
    await element(by.id('list')).swipe('up');
    await element(by.id('scrollView')).scroll(200, 'down');
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "gestures.spec.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "long press on #item")
	assertContains(t, result.ProbeCode, "swipe up on #list")
	assertContains(t, result.ProbeCode, "scroll down on #scrollView")
}

func TestConvert_DeviceOps(t *testing.T) {
	js := `
describe('Device', () => {
  it('device operations', async () => {
    await device.launchApp();
    await device.reloadReactNative();
    await device.takeScreenshot('home');
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "device.spec.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "open the app")
	assertContains(t, result.ProbeCode, "restart the app")
	assertContains(t, result.ProbeCode, `take a screenshot called "home"`)
}

func TestConvert_ClearAndReplace(t *testing.T) {
	js := `
describe('Edit', () => {
  it('edits text', async () => {
    await element(by.id('input')).clearText();
    await element(by.id('input')).replaceText('new value');
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "edit.spec.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "clear #input")
	assertContains(t, result.ProbeCode, `type "new value" into #input`)
}

func TestConvert_ContinuationLines(t *testing.T) {
	js := `
describe('Chained', () => {
  it('handles continuation', async () => {
    await element(by.id('btn'))
      .tap();
  });
});
`
	c := New()
	result, err := c.Convert([]byte(js), "chain.spec.js")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "tap on #btn")
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q\ngot:\n%s", needle, haystack)
	}
}
