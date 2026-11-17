import { expect, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { expectNoCall, openPath, openStudio, recordCalls, workspace } from "./helpers";

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

  /** Opens the chat and puts a message in it; Send then asks for the preview (the destination has not been confirmed yet). */
  async function chatPreview(page: import("@playwright/test").Page) {
    await openStudio(page);
    await page.locator("body").press("ControlOrMeta+Shift+a");
    await page.fill("#chat-input", "why did it fail?");
    await page.click("#btn-chat-send");
    await expect(page.locator("#ai-preview-overlay")).toBeVisible();
  }

  test("pf-06 the chat's preview: Tab goes round (Cancel, Send, text), Escape is Cancel and sends nothing, the focus goes back to the chat input", async ({ page }) => {
    const calls = recordCalls(page);
    await chatPreview(page);
    await expect(page.locator("#btn-ai-preview-cancel")).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(page.locator("#btn-ai-preview-send")).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(page.locator("#ai-preview-text")).toBeFocused();
    // reached with the keyboard, the box shows a deliberate ring (not the browser's default and not none)
    const ring = await page.locator("#ai-preview-text").evaluate((el) => {
      const c = getComputedStyle(el);
      return { style: c.outlineStyle, width: c.outlineWidth };
    });
    expect(ring.style).not.toBe("none");
    expect(ring.width).toBe("2px");
    await page.keyboard.press("Tab");
    await expect(page.locator("#btn-ai-preview-cancel")).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(page.locator("#ai-preview-overlay")).toBeHidden();
    await expect(page.locator("#chat-input")).toBeFocused();
    await expect(page.locator("#chat-input")).toHaveValue("why did it fail?"); // Cancel keeps the text
    await expectNoCall(page, calls, "Chat"); // with a control that the recorder sees a call in this test
    // the Cancel button is the same answer
    await page.click("#btn-chat-send");
    await expect(page.locator("#ai-preview-overlay")).toBeVisible();
    await page.click("#btn-ai-preview-cancel");
    await expect(page.locator("#ai-preview-overlay")).toBeHidden();
    await expectNoCall(page, calls, "Chat");
  });

  test("pf-07 Cancel and Escape on the fix and explain previews send nothing", async ({ page }) => {
    const calls = recordCalls(page);
    await openStudio(page);
    await openPath(page, "probe-tests/counter.probe");
    for (const [button, overlay, cancel] of [["#btn-fix", "#fix-preview-overlay", "#btn-fix-preview-cancel"], ["#btn-explain", "#explain-preview-overlay", "#btn-explain-preview-cancel"]] as const) {
      await page.click(button);
      await expect(page.locator(overlay)).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(page.locator(overlay)).toBeHidden();
      await page.click(button);
      await expect(page.locator(overlay)).toBeVisible();
      await page.click(cancel);
      await expect(page.locator(overlay)).toBeHidden();
    }
    await expectNoCall(page, calls, "SelectorFixPreviewed", "ExplainPreviewed", "Chat");
  });

  test("pf-08 a ? with a preview open does not open the help card, and one Escape closes the preview and nothing else", async ({ page }) => {
    await chatPreview(page);
    await page.keyboard.press("Tab");
    await page.keyboard.press("Tab");
    await expect(page.locator("#ai-preview-text")).toBeFocused();
    await page.keyboard.press("?");
    await expect(page.locator("#help-overlay")).toBeHidden();
    await page.keyboard.press("Escape");
    await expect(page.locator("#ai-preview-overlay")).toBeHidden();
    await expect(page.locator("#help-overlay")).toBeHidden();
  });

  test("pf-09 closing a preview removes its key listener: three open and close cycles leave one listener per open preview, none after", async ({ page }) => {
    await page.addInitScript(() => {
      let live = 0;
      const add = document.addEventListener.bind(document);
      const remove = document.removeEventListener.bind(document);
      const seen = new Map<unknown, number>();
      document.addEventListener = ((type: string, fn: any, opts?: any) => {
        if (type === "keydown" && !seen.has(fn)) {
          seen.set(fn, 1);
          live++;
        }
        return add(type, fn, opts);
      }) as any;
      document.removeEventListener = ((type: string, fn: any, opts?: any) => {
        if (type === "keydown" && seen.has(fn)) {
          seen.delete(fn);
          live--;
        }
        return remove(type, fn, opts);
      }) as any;
      (window as any).__keydownListeners = () => live;
    });
    await openStudio(page);
    await openPath(page, "probe-tests/counter.probe");
    const live = () => page.evaluate(() => (window as any).__keydownListeners() as number);
    const base = await live();
    for (let i = 0; i < 3; i++) {
      await page.click("#btn-fix");
      await expect(page.locator("#fix-preview-overlay")).toBeVisible();
      expect(await live(), `open ${i + 1}`).toBe(base + 1);
      await page.keyboard.press("Escape");
      await expect(page.locator("#fix-preview-overlay")).toBeHidden();
      expect(await live(), `closed ${i + 1}`).toBe(base);
    }
  });

  test("pf-10 when what opened the chat's preview cannot take the focus back, the chat pane does", async ({ page }) => {
    await chatPreview(page);
    // the chat input stays disabled (as while a reply streams)
    await page.evaluate(() => {
      const i = document.getElementById("chat-input") as HTMLTextAreaElement;
      new MutationObserver(() => {
        if (!i.disabled) i.disabled = true;
      }).observe(i, { attributes: true });
      i.disabled = true;
    });
    await page.keyboard.press("Escape");
    await expect(page.locator("#ai-preview-overlay")).toBeHidden();
    await expect(page.locator("#chat-pane")).toBeFocused();
  });

  test("pf-11 Escape belongs to the preview: with another overlay open behind it, one Escape closes only the preview", async ({ page }) => {
    await openStudio(page);
    await openPath(page, "probe-tests/counter.probe");
    await page.evaluate(() => {
      document.getElementById("help-overlay")!.hidden = false; // an overlay behind (the window's own Escape handler would close it)
      document.getElementById("btn-fix")!.click();
    });
    await expect(page.locator("#fix-preview-overlay")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.locator("#fix-preview-overlay")).toBeHidden();
    await expect(page.locator("#help-overlay")).toBeVisible();
  });
});
