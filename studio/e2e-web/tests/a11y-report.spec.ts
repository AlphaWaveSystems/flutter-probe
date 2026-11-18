import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { openStudio, workspace } from "./helpers";

// A REPORT, not a gate: axe-core is run on the main window and on each of the 11 overlays, and what it finds is printed (and attached as JSON) per
// target. The spec fails only when a scan could not run or an overlay it opened by its control is not on screen. It decides nothing about the
// violations: that is for a review of the numbers (the target and the fixes come after). The way each overlay was opened is part of the report:
// "control" is the real button or key; "unhidden" is the overlay's own markup made visible (its dynamic content, which the real flow fills in, is not
// there), so its numbers say less.

const axeSource = require.resolve("axe-core/axe.min.js");
const yamlPath = path.join(workspace, "probe.yaml");

type Violation = { id: string; impact: string | null; help: string; nodes: number };
type Row = { target: string; how: "control" | "unhidden" | "page"; violations: Violation[]; incomplete: number; passes: number };
const rows: Row[] = [];

/** Runs axe on the document, or on one element (the overlay), with the WCAG A/AA rules plus best practices, and records the result. */
async function scan(page: Page, target: string, how: Row["how"], selector?: string) {
  // A colour that is still fading in is measured half way, so the count changes from run to run: transitions and animations are switched off for the scan
  // (the colours they end at are what is measured), and the page gets a moment to settle.
  await page.addStyleTag({ content: "*, *::before, *::after { transition: none !important; animation: none !important; }" });
  await page.waitForTimeout(300);
  await page.addScriptTag({ path: axeSource });
  const res = await page.evaluate(async (sel) => {
    const axe = (window as any).axe;
    const ctx = sel ? { include: [sel] } : document;
    const out = await axe.run(ctx, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "best-practice"] }, resultTypes: ["violations", "incomplete", "passes"] });
    return {
      violations: out.violations.map((v: any) => ({ id: v.id, impact: v.impact, help: v.help, nodes: v.nodes.length })),
      incomplete: out.incomplete.length,
      passes: out.passes.length,
    };
  }, selector ?? null);
  expect(Array.isArray(res.violations), `the axe scan of ${target} did not run`).toBe(true);
  rows.push({ target, how, ...res });
}

test.describe("Accessibility report (axe-core; report only)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  let original = "";
  test.beforeEach(() => {
    original = fs.readFileSync(yamlPath, "utf8");
    if (!original.endsWith("\n")) original += "\n";
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));
  test.afterAll(async ({}, testInfo) => {
    const impacts: [string, string][] = [["critical", "crit"], ["serious", "serious"], ["moderate", "mod"], ["minor", "minor"]];
    const lines = rows.map((r) => {
      const by = impacts.map(([i, short]) => `${short}=${r.violations.filter((v) => v.impact === i).reduce((n, v) => n + v.nodes, 0)}`).join(" ");
      return `A11Y ${r.target.padEnd(22)} [${r.how}] ${String(r.violations.length).padStart(2)} rules, nodes ${by}${r.violations.length ? "  " + r.violations.map((v) => v.id).join(", ") : ""}`;
    });
    console.log(["A11Y REPORT (axe-core, WCAG 2.0/2.1 A and AA plus best practices; nodes by impact)", ...lines].join("\n"));
    const file = testInfo.outputPath("a11y-report.json");
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, JSON.stringify({ axe: "4.14.0", rows }, null, 1));
    await testInfo.attach("a11y-report.json", { path: file, contentType: "application/json" });
  });

  /** The provider switch the fix and explain buttons need: probe.yaml names a local provider (nothing is asked of it) and lets a suggestion read the last run. */
  const aiOn = () => fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: http://127.0.0.1:9/v1\n  model: report-model\nstudio:\n  ai_context:\n    run_results: true\n`);

  test("a11y-00 the main window, no overlay open", async ({ page }) => {
    await openStudio(page);
    await scan(page, "main window", "page");
  });

  test("a11y-01 help (the ? key)", async ({ page }) => {
    await openStudio(page);
    await page.locator("body").press("?");
    await expect(page.locator("#help-overlay")).toBeVisible();
    await scan(page, "help-overlay", "control", "#help-overlay");
  });

  test("a11y-02 workspace settings (the gear)", async ({ page }) => {
    await openStudio(page);
    await page.click("#btn-settings");
    await expect(page.locator("#settings-overlay")).toBeVisible();
    await scan(page, "settings-overlay", "control", "#settings-overlay");
  });

  test("a11y-03 WiFi devices (the antenna)", async ({ page }) => {
    await openStudio(page);
    await page.click("#btn-wifi");
    await expect(page.locator("#wifi-overlay")).toBeVisible();
    await scan(page, "wifi-overlay", "control", "#wifi-overlay");
  });

  test("a11y-04 the key form of the chat", async ({ page }) => {
    await openStudio(page);
    await page.locator("body").press("ControlOrMeta+Shift+a");
    await expect(page.locator("#chat-pane")).toBeVisible();
    await page.click("#btn-chat-key");
    await expect(page.locator("#api-key-overlay")).toBeVisible();
    await scan(page, "api-key-overlay", "control", "#api-key-overlay");
  });

  test("a11y-05 the preview of Suggest a fix", async ({ page }) => {
    aiOn();
    await openStudio(page);
    await expect(page.locator("#btn-fix")).toBeVisible();
    await page.click("#btn-fix");
    await expect(page.locator("#fix-preview-overlay")).toBeVisible();
    await scan(page, "fix-preview-overlay", "control", "#fix-preview-overlay");
  });

  test("a11y-06 the preview of Explain failures", async ({ page }) => {
    aiOn();
    await openStudio(page);
    await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
    await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
    await expect(page.locator("#btn-explain")).toBeVisible();
    await page.click("#btn-explain");
    await expect(page.locator("#explain-preview-overlay")).toBeVisible();
    await scan(page, "explain-preview-overlay", "control", "#explain-preview-overlay");
  });

  // The rest are opened by a flow that needs a device, a dirty buffer or a request to send; here their own markup is made visible and scanned.
  for (const id of ["ai-preview-overlay", "send-allow-overlay", "confirm-overlay", "discard-overlay", "save-as-overlay"]) {
    test(`a11y-07 ${id} (markup only: unhidden)`, async ({ page }) => {
      await openStudio(page);
      await page.evaluate((i) => {
        document.getElementById(i)!.hidden = false;
      }, id);
      await expect(page.locator(`#${id}`)).toBeVisible();
      await scan(page, id, "unhidden", `#${id}`);
    });
  }
});
