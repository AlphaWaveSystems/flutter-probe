describe('Login', () => {
  beforeEach(async () => {
    await browser.pause(1000);
  });

  it('should login with valid credentials', async () => {
    await $('~Sign In').click();
    await $('~emailInput').setValue('user@test.com');
    await $('~passwordInput').setValue('pass123');
    await $('#loginButton').click();
    await $('~dashboardTitle').waitForDisplayed({ timeout: 5000 });
  });

  it('should show error for wrong credentials', async () => {
    await $('~Sign In').click();
    await $('~emailInput').setValue('wrong@test.com');
    await $('~passwordInput').setValue('badpass');
    await $('#loginButton').click();
    await browser.pause(2000);
    await browser.saveScreenshot('wrong_creds.png');
  });

  it('should navigate back', async () => {
    await $('~Sign In').click();
    await browser.back();
    await $('~Welcome').waitForExist({ timeout: 3000 });
  });
});
