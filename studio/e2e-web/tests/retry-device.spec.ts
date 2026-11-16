import { expect, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { connect, disconnect, openStudio, simName, udid, workspace } from "./helpers";

// A real run with defaults.retry_failed_tests on a named simulator: a test that fails every time is run twice, and the Results pane says so and
// lists the first attempt's failure. (A flaky pass needs a test that fails and then passes, which the fixture app does not give; the browser specs
// rn-07 to rn-09 show that note with seeded events.)

test.describe("Retried tests on a named simulator", () => {
  test.skip(!udid || !workspace, "no simulator (run scripts/studio-e2e-web.sh without STUDIO_E2E_NO_DEVICE)");

  test("rt-01 a test that fails every time says it failed on all attempts and lists the first", async ({ page }) => {
    test.info().annotations.push({ type: "device", description: simName });
    const yamlPath = path.join(workspace, "probe.yaml");
    const original = fs.readFileSync(yamlPath, "utf8");
    try {
      // the defaults block may already exist: a second `defaults:` key would replace it, so the line goes inside it
      const merged = /^defaults:/m.test(original) ? original.replace(/^defaults:\s*\n/m, "defaults:\n  retry_failed_tests: 1\n") : original.trimEnd() + "\ndefaults:\n  retry_failed_tests: 1\n";
      fs.writeFileSync(yamlPath, merged);
      await openStudio(page);
      await connect(page);
      await expect(page.locator("#status-text")).toContainText(simName);
      await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
      await page.locator("#file-list li", { hasText: "failing.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("failing.probe");
      await page.click("#btn-run");
      await expect(page.locator("#results-list li.note-retry")).toHaveText("Failed on all 2 attempts (defaults.retry_failed_tests re-ran it).", { timeout: 180_000 });
      await expect(page.locator("#results-list li.note-attempt")).toHaveCount(1);
      await expect(page.locator("#results-list li.note-attempt")).toContainText("Attempt 1 failed:");
      await expect(page.locator("#btn-run")).toBeEnabled({ timeout: 60_000 });
      await disconnect(page);
    } finally {
      fs.writeFileSync(yamlPath, original);
    }
  });
});
