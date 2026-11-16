import { expect, test } from "@playwright/test";
import { connect, disconnect, openStudio, simName, udid, workspace } from "./helpers";

// While a run is in flight, Run is disabled and says why, and it keeps saying why when the person types (typing re-reads the guidance, and
// it used to read the button's own state, which is "disabled", and give no reason). On a real, named simulator: the run's call is held
// back at the page, so the run stays in flight while the editor is edited.

test.describe("Run says why while a run is in flight", () => {
  test.skip(!udid || !workspace, "no simulator (run scripts/studio-e2e-web.sh without STUDIO_E2E_NO_DEVICE)");

  test("rd-01 typing during a run keeps Run's reason, and the reason goes when the run ends", async ({ page }) => {
    test.info().annotations.push({ type: "device", description: simName });
    let release!: () => void;
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route("**/__headless/call", async (route) => {
      if ((route.request().postData() ?? "").includes('"method":"RunFileScoped"')) await held;
      await route.continue();
    });
    try {
      await openStudio(page);
      await connect(page);
      await expect(page.locator("#status-text")).toContainText(simName);
      await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
      await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("counter.probe");
      await expect(page.locator("#btn-run")).toBeEnabled();
      await expect(page.locator("#btn-run")).not.toHaveAttribute("aria-describedby", /.+/);
      await page.click("#btn-run");
      await expect(page.locator("#btn-run")).toBeDisabled();
      await expect(page.locator("#reason-run")).toHaveText("A run is in progress.");
      // type while it runs: the guidance is re-read
      await page.locator("#editor .view-line").first().click();
      await page.keyboard.press("End");
      await page.keyboard.type("x");
      await expect(page.locator("#editor-dirty")).toContainText("unsaved");
      await expect(page.locator("#btn-run")).toBeDisabled();
      await expect(page.locator("#btn-run")).toHaveAttribute("aria-describedby", "reason-run");
      await expect(page.locator("#reason-run")).toHaveText("A run is in progress.");
      await expect(page.locator("#btn-run")).toHaveAttribute("title", "A run is in progress.");
      release();
      await expect(page.locator("#btn-run")).toBeEnabled({ timeout: 120_000 });
      await expect(page.locator("#reason-run")).toHaveText("");
      await expect(page.locator("#btn-run")).not.toHaveAttribute("aria-describedby", /.+/);
    } finally {
      release();
      await disconnect(page);
    }
  });
});
