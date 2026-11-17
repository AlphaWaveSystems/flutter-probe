import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { openPath, openStudio, workspace } from "./helpers";

// A REPORT, not a gate: for each of the 11 overlays, four keyboard properties are measured and printed (`KBD …` lines, attached as JSON):
//   in      the focus is inside the overlay when it opens
//   tab     Tab and Shift+Tab never leave the overlay (12 presses forward, 3 back), measured on a second opening
//   esc     Escape closes it
//   back    after Escape the focus is on what opened it (the button, or the page for an overlay a key opened)
// Every overlay is opened by its real control, with no unhiding. The spec fails only when an overlay does not open; it decides nothing about the answers.

const yamlPath = path.join(workspace, "probe.yaml");
type Row = { overlay: string; opener: string; in: boolean; tab: boolean; esc: boolean; back: boolean; after: string };
const rows: Row[] = [];

async function describeFocus(page: Page): Promise<string> {
  return page.evaluate(() => {
    const a = document.activeElement as HTMLElement | null;
    if (!a || a === document.body) return "body";
    return a.id ? `#${a.id}` : a.tagName.toLowerCase();
  });
}

async function measure(page: Page, overlay: string, opener: string, open: () => Promise<void>) {
  const box = page.locator(overlay);
  const inside = () => page.evaluate((sel) => !!document.querySelector(sel)?.contains(document.activeElement), overlay);
  // Open and press Escape at once: does the focus start inside, does Escape close it, and where is the focus afterwards. (Tab presses first would move the
  // focus, and the focus the overlay gives back is the one it was opened from, so the two are measured on two openings.)
  await open();
  await expect(box, `${overlay} did not open`).toBeVisible();
  const focusIn = await inside();
  await page.keyboard.press("Escape");
  await page.waitForTimeout(300);
  const closes = await box.isHidden();
  const after = await describeFocus(page);
  const back = closes && (opener === "body" ? after === "body" : after === opener);
  // Tab: on a second opening (or on the first one when Escape did not close it), 12 presses forward and 3 back must all stay inside.
  if (closes) {
    await open();
    await expect(box, `${overlay} did not open the second time`).toBeVisible();
  }
  let tabStays = true;
  for (let i = 0; i < 12; i++) {
    await page.keyboard.press("Tab");
    if (!(await inside())) tabStays = false;
  }
  for (let i = 0; i < 3; i++) {
    await page.keyboard.press("Shift+Tab");
    if (!(await inside())) tabStays = false;
  }
  rows.push({ overlay: overlay.slice(1), opener, in: focusIn, tab: tabStays, esc: closes, back, after: closes ? after : "(still open)" });
}

test.describe("Keyboard report (11 overlays x 4 properties; report only)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  let original = "";
  test.beforeEach(() => {
    original = fs.readFileSync(yamlPath, "utf8");
    if (!original.endsWith("\n")) original += "\n";
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));
  test.afterAll(async ({}, testInfo) => {
    const yn = (b: boolean) => (b ? "yes" : "NO ");
    const lines = rows.map((r) => `KBD ${r.overlay.padEnd(22)} opener ${r.opener.padEnd(22)} in=${yn(r.in)} tab=${yn(r.tab)} esc=${yn(r.esc)} back=${yn(r.back)} (focus after: ${r.after})`);
    const full = rows.filter((r) => r.in && r.tab && r.esc && r.back).length;
    console.log(["KBD REPORT: " + `${full} of ${rows.length} overlays have all four`, ...lines].join("\n"));
    const file = testInfo.outputPath("keyboard-report.json");
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, JSON.stringify(rows, null, 1));
    await testInfo.attach("keyboard-report.json", { path: file, contentType: "application/json" });
  });

  const aiOn = () => fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: http://127.0.0.1:9/v1\n  model: report-model\nstudio:\n  ai_context:\n    run_results: true\n`);

  test("kbd-01 help (the ? key)", async ({ page }) => {
    await openStudio(page);
    await measure(page, "#help-overlay", "body", async () => {
      await page.locator("body").press("?");
    });
  });

  test("kbd-02 settings (the gear)", async ({ page }) => {
    await openStudio(page);
    await measure(page, "#settings-overlay", "#btn-settings", async () => {
      await page.click("#btn-settings");
    });
  });

  test("kbd-03 WiFi devices (the antenna)", async ({ page }) => {
    await openStudio(page);
    await measure(page, "#wifi-overlay", "#btn-wifi", async () => {
      await page.click("#btn-wifi");
    });
  });

  test("kbd-04 the key form of the chat", async ({ page }) => {
    await openStudio(page);
    await page.locator("body").press("ControlOrMeta+Shift+a");
    await measure(page, "#api-key-overlay", "#btn-chat-key", async () => {
      await page.click("#btn-chat-key");
    });
  });

  test("kbd-05 the preview of Suggest a fix", async ({ page }) => {
    aiOn();
    await openStudio(page);
    await measure(page, "#fix-preview-overlay", "#btn-fix", async () => {
      await page.click("#btn-fix");
    });
  });

  test("kbd-06 the preview of Explain failures", async ({ page }) => {
    aiOn();
    await openStudio(page);
    await openPath(page, "probe-tests/counter.probe");
    await measure(page, "#explain-preview-overlay", "#btn-explain", async () => {
      await page.click("#btn-explain");
    });
  });

  test("kbd-07 the preview of the chat's first message", async ({ page }) => {
    aiOn();
    await openStudio(page);
    await page.locator("body").press("ControlOrMeta+Shift+a");
    await page.fill("#chat-input", "why did it fail?");
    await measure(page, "#ai-preview-overlay", "#chat-input", async () => {
      await page.click("#btn-chat-send");
    });
  });

  test("kbd-08 Save as (a buffer with no file)", async ({ page }) => {
    await openStudio(page);
    await page.locator("#editor .view-line").first().click();
    await page.keyboard.type("test \"x\"");
    await measure(page, "#save-as-overlay", "#btn-save", async () => {
      await page.click("#btn-save");
    });
  });

  test("kbd-09 discard unsaved changes (Close workspace with an edit)", async ({ page }) => {
    await openStudio(page);
    await openPath(page, "probe-tests/counter.probe");
    await page.locator("#editor .view-line").first().click();
    await page.keyboard.type("x");
    await measure(page, "#discard-overlay", "#btn-close-workspace", async () => {
      await page.click("#btn-close-workspace");
    });
  });

  test("kbd-10 clear app data (a confirm:request event)", async ({ page }) => {
    await page.addInitScript(() => {
      const handlers: Record<string, Array<(...a: unknown[]) => void>> = {};
      (window as any).__handlers = handlers;
      let rt: any;
      Object.defineProperty(window, "runtime", {
        configurable: true,
        get: () => rt,
        set: (v) => {
          rt = v;
          const orig = v.EventsOnMultiple.bind(v);
          v.EventsOnMultiple = (name: string, cb: (...a: unknown[]) => void, max: number) => {
            (handlers[name] = handlers[name] || []).push(cb);
            return orig(name, cb, max);
          };
        },
      });
    });
    await openStudio(page);
    await measure(page, "#confirm-overlay", "body", async () => {
      await page.evaluate(() => (((window as any).__handlers["confirm:request"]) || []).forEach((cb: (...a: unknown[]) => void) => cb({ id: "id-1", kind: "clear-app-data", appId: "com.example.app", deviceName: "Pixel 8" })));
    });
  });

  test("kbd-11 Send requests (the first request to a host)", async ({ page }) => {
    await openStudio(page);
    await page.click("#tab-network");
    await page.click("#btn-net-send");
    await page.selectOption("#send-method", "GET");
    await page.fill("#send-url", "http://127.0.0.1:9/report");
    await measure(page, "#send-allow-overlay", "#btn-send", async () => {
      await page.click("#btn-send");
    });
  });
});
