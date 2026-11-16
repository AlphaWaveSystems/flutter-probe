import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { openStudio, workspace } from "./helpers";

// The three run knobs of Settings (test time limit, retries, wait for a target) are written to probe.yaml under defaults, read back, and
// refused with the reason when they are wrong. The workspace's probe.yaml is put back after each test, byte for byte.

const yamlPath = path.join(workspace, "probe.yaml");
let original = "";

async function openSettings(page: Page) {
  await openStudio(page);
  await page.click("#btn-settings");
  await expect(page.locator("#settings-overlay")).toBeVisible();
  await expect(page.locator("#settings-port")).toHaveValue(/^\d+$/); // the load has filled the form
}

test.describe("Settings run knobs", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  test.beforeEach(() => {
    original = fs.readFileSync(yamlPath, "utf8");
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));

  test("sk-01 the three knobs are saved to probe.yaml and shown again when Settings is reopened", async ({ page }) => {
    await openSettings(page);
    await page.fill("#settings-test-timeout", "90s");
    await page.fill("#settings-retry", "2");
    await page.fill("#settings-implicit-wait", "2s");
    await page.click("#btn-settings-save");
    await expect(page.locator("#settings-overlay")).toBeHidden();
    const yaml = fs.readFileSync(yamlPath, "utf8");
    expect(yaml).toMatch(/test_timeout: 90s/);
    expect(yaml).toMatch(/retry_failed_tests: 2/);
    expect(yaml).toMatch(/implicit_wait: 2s/);
    await page.click("#btn-settings");
    await expect(page.locator("#settings-test-timeout")).toHaveValue("1m30s");
    await expect(page.locator("#settings-retry")).toHaveValue("2");
    await expect(page.locator("#settings-implicit-wait")).toHaveValue("2s");
  });

  test("sk-02 a wrong value is refused with the reason, and probe.yaml is not changed", async ({ page }) => {
    await openSettings(page);
    await page.fill("#settings-test-timeout", "500ms");
    await page.click("#btn-settings-save");
    await expect(page.locator("#toast-container .toast", { hasText: "at least 1s" })).toBeVisible();
    await expect(page.locator("#settings-overlay")).toBeVisible(); // the form stays open
    expect(fs.readFileSync(yamlPath, "utf8")).toBe(original);
    await page.fill("#settings-test-timeout", "");
    await page.fill("#settings-retry", "11");
    await page.click("#btn-settings-save");
    await expect(page.locator("#toast-container .toast", { hasText: "outside 0 to 10" })).toBeVisible();
    expect(fs.readFileSync(yamlPath, "utf8")).toBe(original);
  });

  test("sk-03 clearing a knob removes its key", async ({ page }) => {
    expect(original).toContain("  timeout: 30s"); // the fixture has a defaults block
    fs.writeFileSync(yamlPath, original.replace("  timeout: 30s", "  timeout: 30s\n  test_timeout: 45s"));
    await openSettings(page);
    await expect(page.locator("#settings-test-timeout")).toHaveValue("45s");
    await page.fill("#settings-test-timeout", "");
    await page.click("#btn-settings-save");
    await expect(page.locator("#settings-overlay")).toBeHidden();
    expect(fs.readFileSync(yamlPath, "utf8")).not.toMatch(/test_timeout/);
  });
});
