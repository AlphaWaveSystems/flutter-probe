import { expect, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { openStudio, workspace } from "./helpers";

// The text a preview shows (what would be sent to the AI provider) can be longer than its box, so it scrolls; a scrolling area that the keyboard cannot
// reach is out of reach for a person without a mouse (axe: scrollable-region-focusable). It has a tab stop, a role and a name, and the arrow keys scroll it.

const yamlPath = path.join(workspace, "probe.yaml");

test.describe("The preview text can be reached with the keyboard", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  let original = "";
  test.beforeEach(() => {
    original = fs.readFileSync(yamlPath, "utf8");
    if (!original.endsWith("\n")) original += "\n";
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: http://127.0.0.1:9/v1\n  model: preview-model\nstudio:\n  ai_context:\n    run_results: true\n`);
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));

  for (const [name, button, overlay, text, label] of [
    ["Suggest a fix", "#btn-fix", "#fix-preview-overlay", "#fix-preview-text", "What Suggest a fix would send"],
    ["Explain", "#btn-explain", "#explain-preview-overlay", "#explain-preview-text", "What Explain would send"],
  ] as const) {
    test(`pf-0${name === "Explain" ? 2 : 1} the ${name} preview text is a named tab stop: Shift+Tab from the focused Cancel reaches it, and the arrow keys scroll it`, async ({ page }) => {
      await openStudio(page);
      if (name === "Explain") {
        await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
        await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
      }
      await page.click(button);
      await expect(page.locator(overlay)).toBeVisible();
      const box = page.locator(text);
      await expect(box).toHaveAttribute("tabindex", "0");
      await expect(box).toHaveAttribute("role", "region");
      await expect(box).toHaveAttribute("aria-label", label);
      await page.keyboard.press("Shift+Tab"); // Cancel has the focus when the preview opens; the text is just before it
      await expect(box).toBeFocused();
      // make it scroll, then scroll it with the keyboard (what a sighted keyboard user needs from a long preview)
      await box.evaluate((el) => {
        el.textContent = Array.from({ length: 200 }, (_, i) => `line ${i}`).join("\n");
      });
      expect(await box.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
      const before = await box.evaluate((el) => el.scrollTop);
      await page.keyboard.press("PageDown");
      await expect.poll(() => box.evaluate((el) => el.scrollTop)).toBeGreaterThan(before);
    });

    test(`pf-0${name === "Explain" ? 5 : 4} the ${name} preview keeps Tab inside (text, Cancel, Send, round again), and Escape gives the focus back to the button that opened it`, async ({ page }) => {
      await openStudio(page);
      if (name === "Explain") {
        await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
        await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
      }
      await page.click(button);
      await expect(page.locator(overlay)).toBeVisible();
      const cancel = page.locator(overlay + " button", { hasText: "Cancel" });
      const send = page.locator(overlay + " button", { hasText: "Send" });
      await expect(cancel).toBeFocused(); // the safe choice
      await page.keyboard.press("Tab");
      await expect(send).toBeFocused();
      await page.keyboard.press("Tab");
      await expect(page.locator(text)).toBeFocused(); // round: the text comes next, not the page behind
      await page.keyboard.press("Tab");
      await expect(cancel).toBeFocused();
      await page.keyboard.press("Shift+Tab");
      await expect(page.locator(text)).toBeFocused();
      expect(await page.evaluate((o) => document.querySelector(o)!.contains(document.activeElement), overlay)).toBe(true);
      await page.keyboard.press("Escape");
      await expect(page.locator(overlay)).toBeHidden();
      await expect(page.locator(button)).toBeFocused(); // back where it was
    });
  }

  test("pf-03 the chat's preview text has the same tab stop, role and name", async ({ page }) => {
    await openStudio(page);
    const box = page.locator("#ai-preview-text");
    await expect(box).toHaveAttribute("tabindex", "0");
    await expect(box).toHaveAttribute("role", "region");
    await expect(box).toHaveAttribute("aria-label", "What the chat would send");
  });
});
