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

  test("19 manual WiFi entry refuses a malformed host and says why", async ({ page }) => {
    await openStudio(page);
    await page.click("#btn-wifi");
    await page.locator("#wifi-manual summary").click();
    await page.fill("#wifi-manual-host", "ws://10.0.0.5");
    await page.fill("#wifi-manual-token", "abcdefghijklmnopqrstuvwxyz012345");
    await page.click("#btn-wifi-manual-test");
    const msg = page.locator("#wifi-manual-msg");
    await expect(msg).toBeVisible();
    await expect(msg).toContainText("no scheme");
    await expect(msg).toHaveClass(/err/);
    await expect(page.locator("#status-text")).toHaveText("disconnected");
  });

  test("20 manual WiFi entry to an unreachable agent says what to check", async ({ page }) => {
    await openStudio(page);
    await page.click("#btn-wifi");
    await page.locator("#wifi-manual summary").click();
    await page.fill("#wifi-manual-host", "127.0.0.1");
    await page.fill("#wifi-manual-port", "1"); // nothing listens on port 1
    await page.fill("#wifi-manual-token", "abcdefghijklmnopqrstuvwxyz012345");
    await page.click("#btn-wifi-manual-test");
    const msg = page.locator("#wifi-manual-msg");
    await expect(msg).toContainText("PROBE_WIFI=true", { timeout: 20_000 });
    await expect(msg).toContainText("foreground");
    await expect(page.locator("#btn-wifi-manual-connect")).toBeEnabled();
    await expect(page.locator("#status-text")).toHaveText("disconnected");
  });

  test("21 the manual entry's token is not kept when the overlay is reopened", async ({ page }) => {
    await openStudio(page);
    await page.click("#btn-wifi");
    await page.locator("#wifi-manual summary").click();
    await page.fill("#wifi-manual-token", "abcdefghijklmnopqrstuvwxyz012345");
    await page.click("#btn-wifi-close");
    await page.click("#btn-wifi");
    await page.locator("#wifi-manual summary").click();
    await expect(page.locator("#wifi-manual-token")).toHaveValue("");
    await expect(page.locator("#wifi-manual-port")).toHaveValue("48686");
  });

  test("22 the picker shows physical phones with what they need instead of hiding them", async ({ page }) => {
    // The headless bridge's App is a Proxy, so the Go call is answered at the network level instead.
    await page.route("**/__headless/call", async (route) => {
      const body = JSON.parse(route.request().postData() || "{}");
      if (body.method !== "ListDevices") return route.continue();
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          result: [
            { id: "SIM-1", name: "Test Sim", platform: "ios", kind: "simulator", state: "booted", osVersion: "iOS 18", booted: true },
            { id: "EMU-1", name: "Test Emulator", platform: "android", kind: "emulator", state: "online", osVersion: "Android 15", booted: true },
            { id: "U-USB", name: "Wired Phone", platform: "ios", kind: "physical", state: "online", osVersion: "iOS 18", booted: true, transport: "usb", model: "iPhone 15", hint: "Attached by USB (needs iproxy): the app must be running in the foreground with the agent." },
            { id: "U-NET", name: "Wireless Phone", platform: "ios", kind: "physical", state: "paired", osVersion: "iOS 18", booted: false, transport: "network", model: "iPhone 14", hint: "Paired over the network, not attached by USB: use the WiFi button, then Connect manually." },
          ],
        }),
      });
    });
    await openStudio(page);
    await page.click("#btn-refresh");
    const picker = page.locator("#device-picker");
    await expect(picker.locator("option", { hasText: "Wireless Phone (iPhone 14)" })).toHaveText(/physical.*WiFi only/);
    await expect(picker.locator("option", { hasText: "Wired Phone (iPhone 15)" })).toHaveText(/physical.*USB/);
    // The plain platform tag of a simulator and of an emulator (native Test02 checked the simulator's tag).
    await expect(picker.locator("option", { hasText: "Test Sim" })).toHaveText(/·\s+ios$/);
    await expect(picker.locator("option", { hasText: "Test Emulator" })).toHaveText(/·\s+android$/);
    // A refresh that started before the choice resets it when it finishes: let the in-flight ones end, then choose.
    await page.waitForTimeout(500);
    const choose = (id: string) =>
      expect(async () => {
        await picker.selectOption(id);
        await expect(picker).toHaveValue(id, { timeout: 500 });
      }).toPass();
    await choose("U-NET");
    await expect(page.locator("#device-hint")).toBeVisible();
    await expect(page.locator("#device-hint")).toContainText("Connect manually");
    await expect(page.locator("#btn-connect")).toBeDisabled();
    await expect(page.locator("#btn-connect")).toHaveAttribute("title", /WiFi button/);
    await choose("U-USB");
    await expect(page.locator("#device-hint")).toContainText("foreground");
    await expect(page.locator("#btn-connect")).toBeEnabled();
    await choose("SIM-1");
    await expect(page.locator("#device-hint")).toBeHidden();
  });

  test("23 manual WiFi entry with an empty host says a host is needed", async ({ page }) => {
    await openStudio(page);
    await page.click("#btn-wifi");
    await page.locator("#wifi-manual summary").click();
    await page.click("#btn-wifi-manual-test");
    const msg = page.locator("#wifi-manual-msg");
    await expect(msg).toBeVisible();
    await expect(msg).toContainText("IP address or hostname");
    await expect(msg).toHaveClass(/err/);
    await expect(page.locator("#status-text")).toHaveText("disconnected");
  });

  test("24 Record and Save: disabled without a device or an edited file; Save enables once the file is edited", async ({ page }) => {
    await openStudio(page);
    await expect(page.locator("#btn-record")).toBeDisabled();
    await expect(page.locator("#btn-save")).toBeDisabled();
    await openPath(page, "probe-tests/counter.probe");
    await expect(page.locator("#editor-filename")).toContainText("counter.probe");
    await expect(page.locator("#btn-save")).toBeDisabled();
    await expect(page.locator("#editor-dirty")).toBeEmpty();
    await page.locator("#editor .view-lines").click();
    await page.keyboard.press("End");
    await page.keyboard.type(" ");
    await expect(page.locator("#editor-dirty")).toHaveText("● unsaved");
    await expect(page.locator("#btn-save")).toBeEnabled();
    await expect(page.locator("#btn-record")).toBeDisabled();
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
