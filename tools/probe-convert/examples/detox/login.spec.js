describe('Login', () => {
  beforeEach(async () => {
    await device.launchApp({ newInstance: true });
  });

  afterEach(async () => {
    await device.takeScreenshot('after_test');
  });

  it('should log in with valid credentials', async () => {
    await element(by.id('emailInput')).typeText('user@test.com');
    await element(by.id('passwordInput')).typeText('pass123');
    await element(by.text('Sign In')).tap();
    await waitFor(element(by.text('Dashboard'))).toBeVisible().withTimeout(5000);
    await expect(element(by.text('Dashboard'))).toBeVisible();
  });

  it('should show error for wrong credentials', async () => {
    await element(by.id('emailInput')).typeText('wrong@test.com');
    await element(by.id('passwordInput')).typeText('badpass');
    await element(by.text('Sign In')).tap();
    await expect(element(by.text('Invalid credentials'))).toBeVisible();
    await expect(element(by.text('Dashboard'))).not.toBeVisible();
  });

  it('should navigate back from login', async () => {
    await element(by.text('Sign In')).tap();
    await element(by.id('backButton')).tap();
    await expect(element(by.text('Welcome'))).toBeVisible();
  });
});
