import { expect, test } from "@playwright/test";
import { connect, disconnect, openStudio, simName, udid, workspace } from "./helpers";

// A real run on a named simulator with a step that waits for a text that never appears: after ten seconds the toolbar says Studio is still waiting
// for that step, the progress shows how long it has been going, and Cancel ends the run and takes the line away.

test.describe("A step that waits, on a named simulator", () => {
  test.skip(!udid || !workspace, "no simulator (run scripts/studio-e2e-web.sh without STUDIO_E2E_NO_DEVICE)");

  test("sw-10 the waiting line and the elapsed time appear, and Cancel ends the run", async ({ page }) => {
    test.info().annotations.push({ type: "device", description: simName });
    await openStudio(page);
    await connect(page);
    await expect(page.locator("#status-text")).toContainText(simName);
    await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
    await page.locator("#file-list li", { hasText: "slow-step.probe" }).first().click();
    await expect(page.locator("#editor-filename")).toContainText("slow-step.probe");
    await page.click("#btn-run");
    await expect(page.locator("#run-waiting")).toBeVisible({ timeout: 25_000 });
    await expect(page.locator("#run-waiting")).toContainText('Still waiting for wait until "This text never appears"');
    await expect(page.locator("#run-waiting")).toContainText("Cancel stops the run.");
    await expect(page.locator("#run-progress")).toContainText(/ · \d+ s/);
    await page.click("#btn-cancel");
    await expect(page.locator("#btn-run")).toBeEnabled({ timeout: 60_000 });
    await expect(page.locator("#run-waiting")).toBeHidden({ timeout: 15_000 });
    await disconnect(page);
  });
});
