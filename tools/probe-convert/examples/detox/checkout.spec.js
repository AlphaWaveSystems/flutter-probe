describe('Checkout', () => {
  beforeAll(async () => {
    await device.launchApp();
    await element(by.id('emailInput')).typeText('user@test.com');
    await element(by.id('passwordInput')).typeText('pass123');
    await element(by.text('Sign In')).tap();
    await waitFor(element(by.text('Dashboard'))).toBeVisible().withTimeout(5000);
  });

  it('should add item to cart', async () => {
    await element(by.text('Shop')).tap();
    await element(by.text('Running Shoes')).tap();
    await element(by.id('addToCartBtn')).tap();
    await expect(element(by.text('Cart (1)'))).toBeVisible();
    await device.takeScreenshot('item_added');
  });

  it('should edit cart item quantity', async () => {
    await element(by.text('Cart')).tap();
    await element(by.id('quantityInput')).clearText();
    await element(by.id('quantityInput')).replaceText('3');
    await expect(element(by.id('totalPrice'))).toHaveText('$299.97');
  });

  it('should perform gestures', async () => {
    await element(by.id('productList')).swipe('up');
    await element(by.id('promoItem')).longPress();
    await expect(element(by.text('Promo Details'))).toBeVisible();
    await element(by.id('productCarousel')).scroll(200, 'down');
  });

  it('should handle device operations', async () => {
    await device.reloadReactNative();
    await waitFor(element(by.text('Dashboard'))).toBeVisible().withTimeout(5000);
    await device.takeScreenshot('after_reload');
  });
});
