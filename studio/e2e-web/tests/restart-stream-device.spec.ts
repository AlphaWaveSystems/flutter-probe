import { expect, test } from "@playwright/test";
import { connect, disconnect, openStudio, simName, udid, workspace } from "./helpers";

// A `restart the app` step in a Studio run kills the app and reconnects the run to the new agent. The live view must come back with it:
// after the run the device label shows a frame rate again (not "no signal") and a second run on the same connection still works.

test.describe("The live view after a restart step", () => {
  test.skip(!udid || !workspace, "no simulator (run scripts/studio-e2e-web.sh without STUDIO_E2E_NO_DEVICE)");

  test("rs-01 after restart the app the live view recovers, and a second run passes", async ({ page }) => {
    test.info().annotations.push({ type: "device", description: simName });
    const warnings: string[] = [];
    page.on("console", (m) => {
      if (m.text().includes("stream error")) warnings.push(m.text());
    });
    try {
      await openStudio(page);
      await connect(page);
      await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
      await page.locator("#file-list li", { hasText: "restart.probe" }).first().click();
      for (const round of [1, 2]) {
        await page.click("#btn-run");
        await expect(page.locator("#btn-run")).toBeEnabled({ timeout: 120_000 });
        await expect(page.locator("#results-list li.pass", { hasText: "restart resets the counter" }).first()).toBeVisible({ timeout: 20_000 });
        // the view is live again: a frame rate, not "no signal"
        await expect(page.locator("#device-fps"), `round ${round}: ${warnings.slice(-1)[0] ?? ""}`).toContainText(/fps/, { timeout: 15_000 });
      }
    } finally {
      await disconnect(page);
    }
  });
});
