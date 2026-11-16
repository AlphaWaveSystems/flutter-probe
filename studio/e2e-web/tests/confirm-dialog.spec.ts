import { expect, Page, test } from "@playwright/test";
import { openStudio, workspace } from "./helpers";

// The question before `clear app data`, in the window, with no device: Go's `confirm:request` event is fired the way the run fires it (the
// handlers the page registered). Cancel has the focus, Escape and Cancel say no, Enter never presses Clear data, and every text is text.

async function captureEvents(page: Page) {
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
}

const fire = (page: Page, name: string, ...args: unknown[]) =>
  page.evaluate(([n, a]) => (((window as any).__handlers[n as string]) || []).forEach((cb: (...x: unknown[]) => void) => cb(...(a as unknown[]))), [name, args] as const);

const request = (id = "id-1", appId = "com.example.app", deviceName = "Pixel 8") => ({ id, kind: "clear-app-data", appId, deviceName });

/** Records the answers the page sends to Go. */
function answers(page: Page): Array<[string, boolean]> {
  const seen: Array<[string, boolean]> = [];
  page.on("request", (r) => {
    const body = r.postData() ?? "";
    if (!body.includes('"method":"AnswerConfirm"')) return;
    try {
      const args = JSON.parse(body).args as [string, boolean];
      seen.push([args[0], args[1]]);
    } catch {
      /* not JSON */
    }
  });
  return seen;
}

test.describe("Confirm before clear app data", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  test.beforeEach(async ({ page }) => {
    await captureEvents(page);
    await openStudio(page);
  });
  const overlay = (page: Page) => page.locator("#confirm-overlay");

  test("cd-01 the dialog names the app and the device, Cancel has the focus, and the wipe button is not the default", async ({ page }) => {
    await fire(page, "confirm:request", request());
    await expect(overlay(page)).toBeVisible();
    await expect(page.locator("#confirm-title")).toHaveText("Clear app data?");
    await expect(page.locator("#confirm-app")).toHaveText("com.example.app");
    await expect(page.locator("#confirm-device")).toHaveText("Pixel 8");
    await expect(page.locator("#confirm-warning")).toContainText("cannot be undone");
    await expect(page.locator("#btn-confirm-cancel")).toBeFocused();
    await expect(page.locator("#btn-confirm-danger")).toHaveText("Clear data");
    await expect(page.locator(".confirm-card")).toHaveAttribute("role", "alertdialog");
    await expect(page.locator(".confirm-card")).toHaveAttribute("aria-modal", "true");
  });

  test("cd-02 Escape and Cancel say no; nothing says yes by itself", async ({ page }) => {
    const seen = answers(page);
    await fire(page, "confirm:request", request("a"));
    await page.keyboard.press("Escape");
    await expect(overlay(page)).toBeHidden();
    await expect.poll(() => seen.length).toBe(1);
    expect(seen[0]).toEqual(["a", false]);
    await fire(page, "confirm:request", request("b"));
    await page.click("#btn-confirm-cancel");
    await expect(overlay(page)).toBeHidden();
    await expect.poll(() => seen.length).toBe(2);
    expect(seen[1]).toEqual(["b", false]);
    await page.waitForTimeout(300);
    expect(seen.some(([, allow]) => allow)).toBe(false);
  });

  test("cd-03 Enter on Cancel says no, and Enter on Clear data does nothing: only a click or Space says yes", async ({ page }) => {
    const seen = answers(page);
    await fire(page, "confirm:request", request("c"));
    await page.keyboard.press("Tab"); // Cancel -> Clear data
    await expect(page.locator("#btn-confirm-danger")).toBeFocused();
    await page.keyboard.press("Enter");
    await page.waitForTimeout(300);
    await expect(overlay(page)).toBeVisible();
    expect(seen).toEqual([]); // nothing was answered
    await page.keyboard.press("Tab"); // Tab stays in the dialog: back to Cancel
    await expect(page.locator("#btn-confirm-cancel")).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(page.locator("#btn-confirm-danger")).toBeFocused();
    await page.keyboard.press(" "); // a keyboard user can say yes on purpose
    await expect.poll(() => seen.length).toBe(1);
    expect(seen[0]).toEqual(["c", true]);
    await expect(overlay(page)).toBeHidden();
    await fire(page, "confirm:request", request("d"));
    await page.click("#btn-confirm-danger");
    await expect.poll(() => seen.length).toBe(2);
    expect(seen[1]).toEqual(["d", true]);
  });

  test("cd-04 a question that Go closes (timeout, a cancelled run, another window) closes without an answer, and a note is a toast", async ({ page }) => {
    const seen = answers(page);
    await fire(page, "confirm:request", request("e"));
    await fire(page, "confirm:close", "other-id");
    await expect(overlay(page)).toBeVisible(); // a close for another id leaves this one
    await fire(page, "confirm:close", "e");
    await expect(overlay(page)).toBeHidden();
    await fire(page, "confirm:note", "Clear app data was not run: nothing answered in the window");
    await expect(page.locator("#toast-container .toast", { hasText: "Clear app data was not run" })).toBeVisible();
    await page.waitForTimeout(300);
    expect(seen).toEqual([]);
  });

  test("cd-05 the names are text: markup in an app id is not interpreted; a kind the window does not know is refused and not shown", async ({ page }) => {
    const seen = answers(page);
    await fire(page, "confirm:request", request("f", "<img src=x onerror=alert(1)>", "<b>dev</b>"));
    await expect(page.locator("#confirm-app")).toHaveText("<img src=x onerror=alert(1)>");
    expect(await page.locator(".confirm-card img, .confirm-card b").count()).toBe(0);
    await page.keyboard.press("Escape");
    await fire(page, "confirm:request", { id: "g", kind: "format-disk", appId: "a", deviceName: "d" });
    await expect(overlay(page)).toBeHidden();
    await expect.poll(() => seen.some(([id, allow]) => id === "g" && allow === false)).toBe(true);
  });

  test("cd-06 a click on the dark area around the card is not an answer", async ({ page }) => {
    const seen = answers(page);
    await fire(page, "confirm:request", request("h"));
    await page.mouse.click(5, 5);
    await page.waitForTimeout(300);
    await expect(overlay(page)).toBeVisible();
    expect(seen).toEqual([]);
  });
});
