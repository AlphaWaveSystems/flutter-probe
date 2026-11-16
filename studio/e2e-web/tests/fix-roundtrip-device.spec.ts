import { expect, test } from "@playwright/test";
import * as fs from "node:fs";
import * as http from "node:http";
import * as net from "node:net";
import * as path from "node:path";
import { connect, disconnect, openStudio, simName, udid, workspace } from "./helpers";

// The fix round trip on a real, named simulator: a test with a mistyped label fails on the device, Suggest a fix (against a fake provider on
// 127.0.0.1) proposes the right label, Replace the line puts it in the editor, the person saves, Re-run runs that test once and passes.
// The device is named in the annotation and asserted in the status line. scripts/studio-e2e-web.sh prepares the simulator and the fixture app.

async function fakeProvider(reply: string) {
  let requests = 0;
  const server = http.createServer((req, res) => {
    req.on("data", () => undefined);
    req.on("end", () => {
      res.setHeader("Content-Type", "application/json");
      if (req.method === "POST" && (req.url ?? "").endsWith("/chat/completions")) {
        requests++;
        res.end(JSON.stringify({ choices: [{ message: { content: reply } }] }));
        return;
      }
      res.end(JSON.stringify({ data: [{ id: "fake-model" }] }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as net.AddressInfo).port;
  return { url: `http://127.0.0.1:${port}`, requests: () => requests, close: () => new Promise<void>((resolve) => server.close(() => resolve())) };
}

const mistyped = [
  'test "fix round trip"',
  "  open the app",
  "  tap #nav_counter",
  '  wait until "Taps: 0" appears',
  '  tap "Tap Mee"', // line 5: the label is "Tap Me"
  '  see "Taps: 1"',
  "",
].join("\n");

test.describe("Fix round trip on a named simulator", () => {
  test.skip(!udid || !workspace, "no device prepared (STUDIO_E2E_UDID unset)");
  const yamlPath = path.join(workspace, "probe.yaml");
  const file = path.join(workspace, "probe-tests", "fix-roundtrip.probe");
  let original = "";

  test.beforeEach(() => {
    original = fs.readFileSync(yamlPath, "utf8");
  });
  test.afterEach(async ({ page }) => {
    fs.writeFileSync(yamlPath, original);
    fs.rmSync(file, { force: true });
    await disconnect(page);
  });

  test("dg-01 a failed tap is fixed, saved by the person, re-run once and passes", async ({ page }) => {
    test.setTimeout(300_000);
    test.info().annotations.push({ type: "device", description: simName });
    const f = await fakeProvider('```probe\ntap "Tap Me"\n```\nThe button is labelled "Tap Me".');
    try {
      fs.writeFileSync(file, mistyped);
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: fake-model\nstudio:\n  ai_context:\n    run_results: true\n`);
      await openStudio(page);
      await connect(page);
      await expect(page.locator("#status-text")).toContainText(simName); // the named device
      await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
      await page.locator("#file-list li", { hasText: "fix-roundtrip.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("fix-roundtrip.probe");

      // the run fails on the mistyped label
      await page.click("#btn-run");
      await expect(page.locator("#results-list li.fail", { hasText: "fix round trip" })).toBeVisible({ timeout: 180_000 });
      await expect(page.locator("#btn-run")).toBeEnabled({ timeout: 60_000 });

      // suggest, send, replace
      await page.click("#btn-fix");
      await expect(page.locator("#fix-preview-overlay")).toBeVisible();
      await page.click("#btn-fix-preview-send");
      await expect(page.locator("#fix-card")).toBeVisible();
      await page.click("#btn-fix-replace");
      await expect(page.locator("#editor .view-lines")).toContainText('tap "Tap Me"');
      expect(fs.readFileSync(file, "utf8")).toBe(mistyped); // nothing is written yet

      // Re-run waits for the person's Save
      await expect(page.locator("#btn-fix-rerun")).toBeDisabled();
      await expect(page.locator("#fix-rerun-hint")).toContainText("Save the file first");
      await page.click("#btn-save");
      await expect(page.locator("#btn-fix-rerun")).toBeEnabled();
      expect(fs.readFileSync(file, "utf8")).toContain('tap "Tap Me"');
      expect(fs.readFileSync(file, "utf8")).not.toContain("Tap Mee");

      // one click, one run, one verdict
      await page.click("#btn-fix-rerun");
      await expect(page.locator("#fix-verdict")).toContainText("Passed: fix round trip", { timeout: 180_000 });
      expect(f.requests()).toBe(1); // one suggestion, nothing else asked the provider
    } finally {
      await f.close();
    }
  });
});
