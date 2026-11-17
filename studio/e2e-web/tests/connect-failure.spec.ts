import { expect, Page, test } from "@playwright/test";
import { openStudio, workspace } from "./helpers";

// A Connect that fails (the app is not running, so no agent token) leaves its reason in the status until the person acts: it used to fall back to
// "disconnected" after 2.5 seconds, and the toast with the reason is gone after 8. The Go calls are answered by hand.

const reason = "agent token: timeout waiting for the token (is the app running with PROBE_AGENT=true?)";
const secondReason = "dial: connection refused on port 48690";

async function failingConnect(page: Page) {
  let calls = 0;
  await page.route("**/__headless/call", async (route) => {
    const body = route.request().postData() ?? "";
    if (body.includes('"method":"ListDevices"')) {
      return route.fulfill({ json: { result: [
        { id: "emu-1", name: "Pixel Test", platform: "android", kind: "emulator", state: "device", booted: true },
        { id: "emu-2", name: "Pixel Two", platform: "android", kind: "emulator", state: "device", booted: true },
      ] } });
    }
    if (body.includes('"method":"DeviceListNote"')) return route.fulfill({ json: { result: "" } });
    if (body.includes('"method":"Connect"')) return route.fulfill({ json: { error: calls++ === 0 ? reason : secondReason } });
    await route.continue();
  });
  await openStudio(page);
  await page.click("#btn-refresh");
  await page.locator("#device-picker").selectOption({ label: "Pixel Test  ·  android" });
}

test.describe("A failed connection", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test("cf-01 the reason stays in the status after the toast and the old 2.5 s reset, and goes when another device is picked", async ({ page }) => {
    await failingConnect(page);
    await page.click("#btn-connect");
    const status = page.locator("#status-text");
    await expect(status).toContainText("connection failed: agent token: timeout");
    await expect(status).toHaveAttribute("title", /PROBE_AGENT=true/);
    await page.waitForTimeout(3500); // the status used to say "disconnected" by now
    await expect(status).toContainText("connection failed: agent token: timeout");
    await page.locator("#device-picker").selectOption({ label: "Pixel Two  ·  android" });
    await expect(status).toHaveText("disconnected");
  });

  test("cf-02 a new Connect replaces the failure with its own reason, not a stale line", async ({ page }) => {
    await failingConnect(page);
    await page.click("#btn-connect");
    await expect(page.locator("#status-text")).toContainText("connection failed");
    await page.click("#btn-connect");
    await expect(page.locator("#status-text")).toContainText("connection failed: dial: connection refused"); // the second attempt's own reason, not the first one's line
  });

  test("cf-03 a failure that comes back after a newer Connect has succeeded does not replace the connected status", async ({ page }) => {
    let calls = 0;
    await page.route("**/__headless/call", async (route) => {
      const body = route.request().postData() ?? "";
      if (body.includes('"method":"ListDevices"')) return route.fulfill({ json: { result: [{ id: "emu-1", name: "Pixel Test", platform: "android", kind: "emulator", state: "device", booted: true }] } });
      if (body.includes('"method":"DeviceListNote"')) return route.fulfill({ json: { result: "" } });
      if (body.includes('"method":"Connect"')) {
        if (calls++ === 0) {
          await new Promise((r) => setTimeout(r, 1500)); // the first attempt is slow, and then fails
          return route.fulfill({ json: { error: reason } });
        }
        return route.fulfill({ json: { result: { connected: true, deviceId: "emu-1", deviceName: "Pixel Test", platform: "android" } } });
      }
      await route.continue();
    });
    await openStudio(page);
    await page.click("#btn-refresh");
    await page.locator("#device-picker").selectOption({ label: "Pixel Test  ·  android" });
    await page.click("#btn-connect"); // A: slow
    await page.click("#btn-connect"); // B: succeeds at once (Connect is not disabled while connecting)
    await expect(page.locator("#status-text")).toContainText("Pixel Test");
    await page.waitForTimeout(2200); // A fails now
    await expect(page.locator("#status-text")).toContainText("Pixel Test");
    await expect(page.locator("#status-text")).not.toContainText("connection failed");
  });
});
