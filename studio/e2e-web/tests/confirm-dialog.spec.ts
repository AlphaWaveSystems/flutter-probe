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
    await expect(page.locator("#btn-confirm-danger")).toBeEnabled(); // armed after half a second
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

  test("cd-07 the dialog keeps the keys: after a click on the dark area (or on the page behind) Tab stays inside, Escape still says no, and typed keys do not reach the editor", async ({ page }) => {
    const seen = answers(page);
    await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
    await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
    await expect(page.locator("#editor .view-lines")).toContainText("counter increments");
    const before = await page.locator("#editor .view-lines").innerText();
    await fire(page, "confirm:request", request("i"));
    await page.mouse.click(5, 5); // the dark area
    await page.keyboard.press("Tab");
    expect(await page.evaluate(() => !!document.activeElement?.closest(".confirm-card"))).toBe(true);
    await page.locator("#editor .view-line").first().click({ force: true }); // a click aimed at the editor behind the dialog
    expect(await page.evaluate(() => !!document.activeElement?.closest(".confirm-card"))).toBe(true);
    await page.evaluate(() => (document.querySelector("#editor textarea") as HTMLElement | null)?.focus()); // focus pushed to the editor by script
    expect(await page.evaluate(() => !!document.activeElement?.closest(".confirm-card"))).toBe(true);
    await page.keyboard.type("zzz"); // Enter is left out: on Cancel it is a no, which cd-03 shows
    expect(await page.locator("#editor .view-lines").innerText()).toBe(before); // nothing reached the editor
    await expect(page.locator("#editor-dirty")).not.toContainText("unsaved");
    expect(seen).toEqual([]);
    await page.mouse.click(5, 5);
    await page.keyboard.press("Escape");
    await expect.poll(() => seen.length).toBe(1);
    expect(seen[0]).toEqual(["i", false]);
    await expect(overlay(page)).toBeHidden();
  });

  test("cd-08 Clear data cannot be pressed for the first half second: a click or a key that was already on its way does not wipe", async ({ page }) => {
    const seen = answers(page);
    await fire(page, "confirm:request", request("j"));
    await expect(page.locator("#btn-confirm-danger")).toBeDisabled();
    await page.locator("#btn-confirm-danger").click({ force: true }); // a click inside the first moments
    await page.keyboard.press(" ");
    await page.waitForTimeout(150);
    expect(seen).toEqual([]);
    await expect(overlay(page)).toBeVisible();
    await expect(page.locator("#btn-confirm-danger")).toBeEnabled({ timeout: 3000 });
    await page.click("#btn-confirm-danger");
    await expect.poll(() => seen.length).toBe(1);
    expect(seen[0]).toEqual(["j", true]);
  });

  test("cd-10 every dialog opens with the focus on Cancel, also a second one right after the first was answered with a click on Clear data, with the pointer still over it", async ({ page }) => {
    const seen = answers(page);
    await fire(page, "confirm:request", request("first"));
    await expect(page.locator("#btn-confirm-danger")).toBeEnabled({ timeout: 3000 });
    await page.click("#btn-confirm-danger");
    await expect.poll(() => seen.length).toBe(1);
    expect(seen[0]).toEqual(["first", true]);
    await expect(overlay(page)).toBeHidden();
    // the pointer stays where Clear data was; the next dialog opens under it
    await fire(page, "confirm:request", request("second"));
    await expect(overlay(page)).toBeVisible();
    await expect(page.locator("#btn-confirm-cancel")).toBeFocused();
    expect(await page.evaluate(() => document.activeElement?.id)).toBe("btn-confirm-cancel");
    expect(await page.locator("#btn-confirm-danger").evaluate((el) => el.matches(":focus"))).toBe(false);
    await page.keyboard.press("Enter"); // Enter on the focused Cancel is a no
    await expect.poll(() => seen.length).toBe(2);
    expect(seen[1]).toEqual(["second", false]);
  });

  test("cd-09 the app is labelled as coming from probe.yaml", async ({ page }) => {
    await fire(page, "confirm:request", request("k"));
    await expect(page.locator("#confirm-app-label")).toHaveText("App (from probe.yaml)");
    await page.keyboard.press("Escape");
  });

  test("cd-06 a click on the dark area around the card is not an answer", async ({ page }) => {
    const seen = answers(page);
    await fire(page, "confirm:request", request("h"));
    await page.mouse.click(5, 5);
    await page.waitForTimeout(300);
    await expect(overlay(page)).toBeVisible();
    expect(seen).toEqual([]);
  });
  test("cd-13 while the question is open, the page behind it cannot be clicked: the Connect/Disconnect button is covered, a click on it is not an answer and does not disconnect", async ({ page }) => {
    const seen = answers(page);
    const calls: string[] = [];
    page.on("request", (r) => {
      const m = /"method":"(\w+)"/.exec(r.postData() ?? "");
      if (m) calls.push(m[1]);
    });
    await fire(page, "confirm:request", request("m"));
    await expect(overlay(page)).toBeVisible();
    // what is at the middle of the button is the dialog's overlay, not the button
    const covered = await page.evaluate(() => {
      const b = document.getElementById("btn-connect") as HTMLElement;
      const r = b.getBoundingClientRect();
      const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      return !!top && document.getElementById("confirm-overlay")!.contains(top) && top !== b;
    });
    expect(covered).toBe(true);
    const box = await page.locator("#btn-connect").boundingBox();
    await page.mouse.click(box!.x + box!.width / 2, box!.y + box!.height / 2);
    await page.waitForTimeout(300);
    await expect(overlay(page)).toBeVisible();
    expect(seen).toEqual([]);
    expect(calls).not.toContain("Disconnect");
    // the way out is the dialog's own: Escape says no
    await page.keyboard.press("Escape");
    await expect.poll(() => seen.length).toBe(1);
    expect(seen[0]).toEqual(["m", false]);
  });
});
