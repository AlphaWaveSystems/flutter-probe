import { expect, test } from "@playwright/test";
import { openStudio, workspace } from "./helpers";

// The toolbar at the width of the desktop window (1175 px): the brand, the badge and the status stay on one line each, whatever the device is
// called, and the bar does not grow taller or wider than the window. The full status text is the tooltip.

test.describe("Toolbar at the window's default width", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  test.use({ viewport: { width: 1175, height: 800 } });

  async function oneLine(page: import("@playwright/test").Page, selector: string) {
    return page.locator(selector).evaluate((el) => {
      const lineHeight = parseFloat(getComputedStyle(el).lineHeight) || parseFloat(getComputedStyle(el).fontSize) * 1.4;
      return el.getBoundingClientRect().height <= lineHeight + 8; // a wrapped text is at least two lines tall
    });
  }

  test("tl-01 disconnected: the brand, the badge and the status are single lines", async ({ page }) => {
    await openStudio(page);
    for (const sel of [".brand-text", ".brand-tag", "#status-text"]) expect(await oneLine(page, sel), sel).toBe(true);
  });

  test("tl-02 a long device name does not wrap the status or the badge, is cut with an ellipsis, and the bar keeps its height", async ({ page }) => {
    await openStudio(page);
    const heightBefore = await page.locator("#topbar").evaluate((el) => el.getBoundingClientRect().height);
    const longName = "ProbeStudioParity-iPhone17-with-a-very-long-simulator-name · ios";
    await page.evaluate((name) => {
      const text = document.getElementById("status-text")!;
      text.textContent = name;
      text.title = name; // setStatus gives the full text as the tooltip
      document.getElementById("status-dot")!.className = "status-dot status-connected";
    }, longName);
    for (const sel of [".brand-text", ".brand-tag", "#status-text"]) expect(await oneLine(page, sel), sel).toBe(true);
    const cut = await page.locator("#status-text").evaluate((el) => ({ over: el.scrollWidth > el.clientWidth, overflow: getComputedStyle(el).textOverflow }));
    expect(cut).toEqual({ over: true, overflow: "ellipsis" });
    await expect(page.locator("#status-text")).toHaveAttribute("title", longName);
    expect(await page.locator("#topbar").evaluate((el) => el.getBoundingClientRect().height)).toBe(heightBefore);
    // the bar fits the window: nothing pushes it wider than the viewport
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    // the badge keeps its shape: as wide as its text and padding, not squeezed
    const badge = await page.locator(".brand-tag").evaluate((el) => ({ w: el.getBoundingClientRect().width, sw: el.scrollWidth }));
    expect(badge.w).toBeGreaterThanOrEqual(badge.sw - 1);
  });
});
