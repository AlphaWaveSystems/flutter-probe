import { expect, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { app, openPath, openStudio, workspace } from "./helpers";

test.describe("Studio UI (no device needed)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test("01 launch shows the workspace, the toolbar and a disconnected status", async ({ page }) => {
    await openStudio(page);
    await expect(page.locator("#file-path")).toContainText(path.basename(workspace));
    await expect(page.locator("#file-list")).toContainText("probe-tests");
    await expect(page.locator("#status-text")).toHaveText("disconnected");
    await expect(page.locator("#btn-run")).toBeDisabled();
    await expect(page.locator("#btn-connect")).toBeDisabled();
    await expect(page.locator("#results-list")).toContainText("No run yet");
  });

  test("02 opening a file shows its name and content in the editor", async ({ page }) => {
    await openStudio(page);
    await openPath(page, "probe-tests/counter.probe");
    await expect(page.locator("#editor-filename")).toContainText("counter.probe");
    await expect(page.locator("#editor .view-lines")).toContainText("counter increments and resets");
  });

  test("03 the device picker is listed and Connect stays disabled until a booted device is picked", async ({ page }) => {
    await openStudio(page);
    await page.click("#btn-refresh");
    await expect(page.locator("#device-picker option").first()).toHaveText(/select device/);
    await expect(page.locator("#btn-connect")).toBeDisabled();
  });

  test("04 settings overlay reads probe.yaml and saves a changed port", async ({ page }) => {
    const yamlPath = path.join(workspace, "probe.yaml");
    // Start from a known file, and put it back afterwards even when the test fails half way.
    const original = "app:\n  id: com.example.fixture\nagent:\n  port: 48686\ndefaults:\n  timeout: 30s\n";
    fs.writeFileSync(yamlPath, original);
    try {
      await openStudio(page);
      await page.click("#btn-settings");
      await expect(page.locator("#settings-overlay")).toBeVisible();
      await expect(page.locator("#settings-port")).toHaveValue("48686"); // the baseline written above
      await page.fill("#settings-port", "48777");
      await page.click("#btn-settings-save");
      await expect(page.locator("#toast-container")).toContainText("Saved probe.yaml");
      expect(fs.readFileSync(yamlPath, "utf8")).toMatch(/port:\s*48777/);
      await expect(page.locator("#settings-overlay")).toBeHidden(); // a successful save closes the form
    } finally {
      fs.writeFileSync(yamlPath, original);
    }
  });

  test("05 the AI chat pane toggles and asks for an API key", async ({ page }) => {
    await openStudio(page);
    await expect(page.locator("#chat-pane")).toBeHidden();
    await page.click("#btn-ai-toggle");
    await expect(page.locator("#chat-pane")).toBeVisible();
    await page.click("#btn-chat-key");
    await expect(page.locator("#api-key-overlay")).toBeVisible();
    await page.click("#btn-api-key-close");
    await expect(page.locator("#api-key-overlay")).toBeHidden();
    await page.click("#btn-ai-toggle");
    await expect(page.locator("#chat-pane")).toBeHidden();
  });

  test("06 the WiFi discovery overlay opens and closes", async ({ page }) => {
    await openStudio(page);
    await page.click("#btn-wifi");
    await expect(page.locator("#wifi-overlay")).toBeVisible();
    await page.click("#btn-wifi-close");
    await expect(page.locator("#wifi-overlay")).toBeHidden();
  });

  test("07 the help overlay opens with ? and closes with Escape", async ({ page }) => {
    await openStudio(page);
    await page.locator("body").press("?");
    await expect(page.locator("#help-overlay")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.locator("#help-overlay")).toBeHidden();
  });

  test("08 running without a device explains what to do", async ({ page }) => {
    await openStudio(page);
    const err = await app<string>(page, "RunFile", path.join(workspace, "probe-tests", "counter.probe")).catch((e) => String(e));
    expect(String(err)).toContain("not connected");
  });

  test("09 the bridge is up: the event stream is connected and Go methods answer", async ({ page }) => {
    await openStudio(page);
    await page.waitForFunction(() => (window as any).__headlessEventsReady);
    await app(page, "SetWorkspace", workspace);
    expect(await app<{ connected: boolean }>(page, "Status")).toMatchObject({ connected: false });
  });
});
