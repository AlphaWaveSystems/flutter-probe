import { expect, test } from "@playwright/test";
import { app, openPath, openStudio, workspace } from "./helpers";

// Opt-in: a physical phone running the fixture app (native-test-apps/flutter) built in profile mode with
// PROBE_AGENT=true and PROBE_WIFI=true. Skipped unless STUDIO_E2E_PHYSICAL=1 and the phone's address and token
// are given, and never in CI. See studio/e2e-web/PHYSICAL.md for the manual checklist and expected results.
const host = process.env.STUDIO_E2E_PHONE_HOST ?? "";
const token = process.env.STUDIO_E2E_PHONE_TOKEN ?? "";
const port = process.env.STUDIO_E2E_PHONE_PORT ?? "48686";

test.describe("Studio with a physical phone (opt-in)", () => {
  test.skip(process.env.STUDIO_E2E_PHYSICAL !== "1", "opt-in: set STUDIO_E2E_PHYSICAL=1 (see PHYSICAL.md)");
  test.skip(!host || !token, "STUDIO_E2E_PHONE_HOST and STUDIO_E2E_PHONE_TOKEN are required");
  test.skip(!!process.env.CI, "never runs in CI");
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-physical.sh)");

  async function fillManual(page: import("@playwright/test").Page) {
    await page.click("#btn-wifi");
    await page.locator("#wifi-manual summary").click();
    await page.fill("#wifi-manual-host", host);
    await page.fill("#wifi-manual-port", port);
    await page.fill("#wifi-manual-token", token);
  }

  test("30 Test connection finds the agent on the phone and accepts the token", async ({ page }) => {
    await openStudio(page);
    await fillManual(page);
    await page.click("#btn-wifi-manual-test");
    await expect(page.locator("#wifi-manual-msg")).toContainText("Agent reachable", { timeout: 30_000 });
    await expect(page.locator("#status-text")).toHaveText("disconnected"); // a test does not connect
  });

  test("31 Connect shows the phone's screen and the fixture's widget tree", async ({ page }) => {
    await openStudio(page);
    await fillManual(page);
    await page.click("#btn-wifi-manual-connect");
    await expect(page.locator("#status-text")).toContainText(host, { timeout: 30_000 });
    await expect(page.locator("#device-view img, #device-view canvas").first()).toBeVisible({ timeout: 30_000 });
    await expect(page.locator("#inspector-content")).toContainText("Native Test App", { timeout: 30_000 });
  });

  test("32 a probe file runs against the phone and passes", async ({ page }) => {
    await openStudio(page);
    await fillManual(page);
    await page.click("#btn-wifi-manual-connect");
    await expect(page.locator("#status-text")).toContainText(host, { timeout: 30_000 });
    await app(page, "WriteFile", `${workspace}/probe-tests/physical-smoke.probe`, 'test "the phone shows the fixture title"\n  see "Native Test App"\n');
    await openPath(page, "probe-tests/physical-smoke.probe");
    await page.click("#btn-run");
    await expect(page.locator("#results-list li.pass", { hasText: "the phone shows the fixture title" })).toBeVisible({ timeout: 120_000 });
    await app(page, "Disconnect").catch(() => undefined);
  });

  test("33 a wrong token is refused with a message that says what to check", async ({ page }) => {
    await openStudio(page);
    await fillManual(page);
    await page.fill("#wifi-manual-token", "this-is-not-the-token-1234567890");
    await page.click("#btn-wifi-manual-test");
    await expect(page.locator("#wifi-manual-msg")).toHaveClass(/err/, { timeout: 30_000 });
    await expect(page.locator("#wifi-manual-msg")).toContainText("token");
    await expect(page.locator("#wifi-manual-msg")).not.toContainText("this-is-not-the-token-1234567890");
  });
});
