import { expect, Page, test } from "@playwright/test";
import { openStudio, workspace } from "./helpers";

// When adb cannot be found, Android emulators and phones are silently absent from the device list. The first-run line says so when no device is
// usable. The Go calls are answered by hand (an empty device list and the note) so the spec does not depend on what runs on this machine.

const note = "adb was not found, so Android emulators and phones are not listed. Install the Android SDK platform-tools, or set tools.adb in probe.yaml to the full path of adb (Studio started from the Dock or Finder does not have your shell's PATH).";

async function answerDeviceCalls(page: Page, devices: unknown[], adbNote: string) {
  await page.route("**/__headless/call", async (route) => {
    const body = route.request().postData() ?? "";
    if (body.includes('"method":"ListDevices"')) return route.fulfill({ json: { result: devices } });
    if (body.includes('"method":"DeviceListNote"')) return route.fulfill({ json: { result: adbNote } });
    await route.continue();
  });
}

test.describe("Device list note about adb", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test("dn-01 no usable device and adb not found: the next step says why Android devices are missing", async ({ page }) => {
    await answerDeviceCalls(page, [], note);
    await openStudio(page);
    await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
    await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
    await page.click("#btn-refresh");
    await expect(page.locator("#next-step")).toContainText("No device found");
    await expect(page.locator("#next-step")).toContainText("adb was not found, so Android emulators and phones are not listed");
    await expect(page.locator("#next-step")).toContainText("tools.adb in probe.yaml");
  });

  test("dn-02 adb found: the no-device line says nothing about adb", async ({ page }) => {
    await answerDeviceCalls(page, [], "");
    await openStudio(page);
    await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
    await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
    await page.click("#btn-refresh");
    await expect(page.locator("#next-step")).toContainText("No device found");
    await expect(page.locator("#next-step")).not.toContainText("adb");
  });

  test("dn-03 the note is plain text: markup in it is not interpreted", async ({ page }) => {
    await answerDeviceCalls(page, [], "<img src=x onerror=alert(1)> adb was not found");
    await openStudio(page);
    await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
    await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
    await page.click("#btn-refresh");
    await expect(page.locator("#next-step")).toContainText("<img src=x onerror=alert(1)>");
    expect(await page.locator("#next-step").evaluate((el) => el.children.length)).toBe(0);
  });
});
