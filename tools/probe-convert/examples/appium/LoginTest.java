package com.example.tests;

import io.appium.java_client.AppiumBy;
import io.appium.java_client.android.AndroidDriver;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import org.openqa.selenium.By;
import org.openqa.selenium.remote.DesiredCapabilities;

import java.net.URL;

public class LoginTest {
    private AndroidDriver driver;

    @Before
    public void setUp() {
        DesiredCapabilities caps = new DesiredCapabilities();
        caps.setCapability("platformName", "Android");
        caps.setCapability("app", "/path/to/app.apk");
        driver = new AndroidDriver(new URL("http://localhost:4723"), caps);
    }

    @After
    public void tearDown() {
        driver.quit();
    }

    @Test
    public void testLoginWithValidCredentials() {
        driver.findElement(AppiumBy.accessibilityId("Sign In")).click();
        driver.findElement(AppiumBy.id("emailInput")).click();
        driver.findElement(AppiumBy.id("emailInput")).sendKeys("user@test.com");
        driver.findElement(AppiumBy.id("passwordInput")).sendKeys("pass123");
        driver.findElement(AppiumBy.id("loginButton")).click();
        Thread.sleep(3000);
    }

    @Test
    public void testLoginWithWrongCredentials() {
        driver.findElement(AppiumBy.accessibilityId("Sign In")).click();
        driver.findElement(AppiumBy.id("emailInput")).sendKeys("wrong@test.com");
        driver.findElement(AppiumBy.id("passwordInput")).sendKeys("badpass");
        driver.findElement(AppiumBy.id("loginButton")).click();
        Thread.sleep(2000);
    }

    @Test
    public void testNavigateBack() {
        driver.findElement(AppiumBy.accessibilityId("Sign In")).click();
        driver.navigate().back();
    }
}
