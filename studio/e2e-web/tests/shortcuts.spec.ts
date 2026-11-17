import { expect, Page, test } from "@playwright/test";
import { openPath, openStudio, workspace } from "./helpers";

// The global keyboard shortcuts of the page (Ctrl or Cmd: the page reads either), checked without a device. Go calls that would change something
// are answered by hand and recorded, so a shortcut is shown to make the call it should, or none: Save writes the open dirty file (the write is
// stubbed, so the shared workspace is never changed), Run and Record do nothing without a connection, Connect does nothing without a device,
// Open workspace asks to pick a folder (and the pick is cancelled), Refresh lists devices, Shift+A toggles the chat pane.

const mods = ["Control", "Meta"] as const;

type Call = { method: string; params: unknown[] }; // params are the call's arguments (the bridge sends them as args)

/** Records every Go call the page makes; PickWorkspace, ListDevices and WriteFile are answered here, the rest goes through. */
async function recordCalls(page: Page): Promise<Call[]> {
  const calls: Call[] = [];
  await page.route("**/__headless/call", async (route) => {
    let m: { method?: string; args?: unknown[] } = {};
    try {
      m = JSON.parse(route.request().postData() ?? "{}");
    } catch {
      /* not a call */
    }
    calls.push({ method: m.method ?? "", params: m.args ?? [] });
    if (m.method === "PickWorkspace") return route.fulfill({ json: { result: "" } }); // the person cancels the dialog
    if (m.method === "ListDevices") return route.fulfill({ json: { result: [] } });
    if (m.method === "WriteFile") return route.fulfill({ json: { result: null } });
    if (m.method === "RunFileScoped" || m.method === "StartRecording" || m.method === "StopRecording" || m.method === "Disconnect") return route.fulfill({ json: { result: null } });
    await route.continue();
  });
  return calls;
}
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
const count = (calls: Call[], method: string) => calls.filter((c) => c.method === method).length;

test.describe("Keyboard shortcuts", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  for (const mod of mods) {
    test(`sc-01 ${mod}+S writes the open file: its own text when nothing changed, the edit once there is one`, async ({ page }) => {
      const calls = await recordCalls(page);
      await openStudio(page);
      await openPath(page, "probe-tests/counter.probe");
      await expect(page.locator("#editor-filename")).toContainText("counter.probe");
      await expect(page.locator("#editor-dirty")).not.toContainText("unsaved");
      await page.locator("body").press(`${mod}+s`); // nothing changed: the file is written back as it is
      await expect.poll(() => count(calls, "WriteFile")).toBe(1);
      const clean = calls.find((c) => c.method === "WriteFile")!;
      await page.locator("#editor .view-line").first().click();
      await page.keyboard.type("# ");
      await expect(page.locator("#editor-dirty")).toContainText("unsaved");
      await page.keyboard.press(`${mod}+s`);
      await expect.poll(() => count(calls, "WriteFile")).toBe(2);
      const w = calls.filter((c) => c.method === "WriteFile")[1];
      expect(String(w.params[0])).toMatch(/counter\.probe$/);
      expect(String(w.params[1]).length).toBe(String(clean.params[1]).length + 2); // the two typed characters, and nothing else
      expect(String(w.params[1])).toContain("# ");
      await expect(page.locator("#editor-dirty")).not.toContainText("unsaved");
      await expect(page.locator("#toast-container .toast", { hasText: "Saved" }).last()).toBeVisible();
    });
  }

  for (const mod of mods) {
    test(`sc-02 ${mod}+R and ${mod}+Shift+R do nothing without a connection`, async ({ page }) => {
      const calls = await recordCalls(page);
      await openStudio(page);
      await openPath(page, "probe-tests/counter.probe");
      await expect(page.locator("#editor-filename")).toContainText("counter.probe");
      const before = await page.locator("#editor-filename").textContent();
      await page.locator("body").press(`${mod}+r`);
      await page.locator("body").press(`${mod}+Shift+r`);
      await page.waitForTimeout(300);
      for (const m of ["RunFile", "RunFileScoped", "StartRecording", "StopRecording"]) expect(count(calls, m), m).toBe(0);
      await expect(page.locator("#editor-filename")).toHaveText(before!); // a recording would have renamed the buffer
      await expect(page.locator("#editor-dirty")).not.toContainText("unsaved");
    });
  }

  for (const mod of mods) {
    test(`sc-03 ${mod}+B does nothing without a device`, async ({ page }) => {
      const calls = await recordCalls(page);
      await openStudio(page);
      await page.locator("body").press(`${mod}+b`);
      await page.waitForTimeout(300);
      expect(count(calls, "Connect") + count(calls, "ConnectWiFi") + count(calls, "Disconnect")).toBe(0);
      await expect(page.locator("#status-text")).not.toContainText(/connected to/i);
    });
  }

  for (const mod of mods) {
    test(`sc-04 ${mod}+P asks to pick a workspace folder, and a cancelled pick changes nothing`, async ({ page }) => {
      const calls = await recordCalls(page);
      await openStudio(page);
      const before = await page.locator("#file-list li").allTextContents();
      const setBefore = count(calls, "SetWorkspace"); // the page sets the remembered workspace when it loads
      await page.locator("body").press(`${mod}+p`);
      await expect.poll(() => count(calls, "PickWorkspace")).toBe(1);
      await page.waitForTimeout(300);
      expect(count(calls, "SetWorkspace")).toBe(setBefore);
      expect(await page.locator("#file-list li").allTextContents()).toEqual(before);
    });
  }

  for (const mod of mods) {
    test(`sc-05 ${mod}+K refreshes the device list`, async ({ page }) => {
      const calls = await recordCalls(page);
      await openStudio(page);
      const before = count(calls, "ListDevices");
      await page.locator("body").press(`${mod}+k`);
      await expect.poll(() => count(calls, "ListDevices")).toBe(before + 1);
    });
  }

  for (const mod of mods) {
    test(`sc-06 ${mod}+Shift+A toggles the AI chat pane; ${mod}+A alone does not`, async ({ page }) => {
      await recordCalls(page);
      await openStudio(page);
      const pane = page.locator("#chat-pane");
      const start = await pane.isHidden();
      await page.locator("body").press(`${mod}+a`);
      expect(await pane.isHidden()).toBe(start);
      await page.locator("body").press(`${mod}+Shift+a`);
      expect(await pane.isHidden()).toBe(!start);
      await page.locator("body").press(`${mod}+Shift+a`);
      expect(await pane.isHidden()).toBe(start);
    });
  }

  for (const mod of mods) {
    test(`sc-09 with a connection, ${mod}+R runs the open file, ${mod}+Shift+R records, ${mod}+B disconnects`, async ({ page }) => {
      await captureEvents(page);
      const calls = await recordCalls(page);
      await openStudio(page);
      await openPath(page, "probe-tests/counter.probe");
      await expect(page.locator("#editor-filename")).toContainText("counter.probe");
      await fire(page, "connection:changed", { connected: true, deviceId: "emu-1", deviceName: "Pixel Test", platform: "android" });
      await expect(page.locator("#status-text")).toContainText("Pixel Test");
      await page.locator("body").press(`${mod}+r`);
      await expect.poll(() => count(calls, "RunFileScoped")).toBe(1);
      expect(count(calls, "StartRecording")).toBe(0); // the plain shortcut is Run, not Record
      await page.locator("body").press(`${mod}+Shift+r`);
      await expect.poll(() => count(calls, "StartRecording")).toBe(1);
      expect(count(calls, "RunFileScoped")).toBe(1);
      await fire(page, "connection:changed", { connected: true, deviceId: "emu-1", deviceName: "Pixel Test", platform: "android" });
      await page.locator("body").press(`${mod}+b`);
      await expect.poll(() => count(calls, "Disconnect")).toBe(1);
    });
  }

  test("sc-07 the help overlay closes on a click outside its card, not on one inside it", async ({ page }) => {
    await recordCalls(page);
    await openStudio(page);
    await page.locator("body").press("?");
    const overlay = page.locator("#help-overlay");
    await expect(overlay).toBeVisible();
    await page.locator("#help-overlay .overlay-card h2").click(); // inside the card
    await expect(overlay).toBeVisible();
    await overlay.click({ position: { x: 5, y: 5 } }); // the dimmed area around the card
    await expect(overlay).toBeHidden();
  });

  test("sc-08 a shortcut with no modifier does nothing, and ? typed in the editor is text", async ({ page }) => {
    const calls = await recordCalls(page);
    await openStudio(page);
    const before = calls.length;
    for (const k of ["s", "r", "b", "p", "k"]) await page.locator("body").press(k);
    await page.waitForTimeout(300);
    expect(calls.slice(before).map((c) => c.method).filter((m) => ["WriteFile", "RunFile", "RunFileScoped", "PickWorkspace", "Connect", "Disconnect", "ListDevices", "StartRecording"].includes(m))).toEqual([]);
    await openPath(page, "probe-tests/counter.probe");
    await page.locator("#editor .view-line").first().click();
    await page.keyboard.type("?");
    await expect(page.locator("#help-overlay")).toBeHidden();
    await expect(page.locator("#editor-dirty")).toContainText("unsaved");
  });

  for (const mod of mods) {
    test(`sc-10 ${mod}+Shift+S saves the open file too`, async ({ page }) => {
      const calls = await recordCalls(page);
      await openStudio(page);
      await openPath(page, "probe-tests/counter.probe");
      await expect(page.locator("#editor-filename")).toContainText("counter.probe");
      await page.locator("body").press(`${mod}+Shift+s`);
      await expect.poll(() => count(calls, "WriteFile")).toBe(1);
    });

    test(`sc-11 while the help card is open ${mod}+P, ${mod}+K and ${mod}+Shift+A do nothing; Escape closes the card and they work again`, async ({ page }) => {
      const calls = await recordCalls(page);
      await openStudio(page);
      const pane = page.locator("#chat-pane");
      const chatBefore = await pane.isHidden();
      await page.locator("body").press("?");
      await expect(page.locator("#help-overlay")).toBeVisible();
      const listBefore = count(calls, "ListDevices");
      await page.locator("body").press(`${mod}+p`);
      await page.locator("body").press(`${mod}+k`);
      await page.locator("body").press(`${mod}+Shift+a`);
      await page.waitForTimeout(300);
      expect(count(calls, "PickWorkspace")).toBe(0);
      expect(count(calls, "ListDevices")).toBe(listBefore);
      expect(await pane.isHidden()).toBe(chatBefore);
      await page.keyboard.press("Escape");
      await expect(page.locator("#help-overlay")).toBeHidden();
      await page.locator("body").press(`${mod}+p`);
      await expect.poll(() => count(calls, "PickWorkspace")).toBe(1);
    });
  }

  test("sc-12 the browser's own defaults (Save, Print, ...) are prevented for the shortcuts, and not for a plain key", async ({ page }) => {
    await openStudio(page);
    const prevented = (init: KeyboardEventInit) =>
      page.evaluate((i) => {
        const ev = new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...i });
        window.dispatchEvent(ev);
        return ev.defaultPrevented;
      }, init);
    for (const key of ["s", "r", "b", "p", "k"]) {
      expect(await prevented({ key, ctrlKey: true }), `Ctrl+${key}`).toBe(true);
      expect(await prevented({ key, metaKey: true }), `Meta+${key}`).toBe(true);
    }
    expect(await prevented({ key: "a", ctrlKey: true, shiftKey: true }), "Ctrl+Shift+A").toBe(true);
    expect(await prevented({ key: "a", ctrlKey: true }), "Ctrl+A alone is the browser's select all").toBe(false);
    // and with the help card open the keys are still taken (the browser's Save or Print must not run under the card), though they do nothing
    await page.locator("body").press("?");
    await expect(page.locator("#help-overlay")).toBeVisible();
    for (const key of ["s", "p", "r"]) expect(await prevented({ key, ctrlKey: true }), `Ctrl+${key} with the help card open`).toBe(true);
    expect(await prevented({ key: "a", ctrlKey: true }), "Ctrl+A with the help card open").toBe(false);
    expect(await prevented({ key: "s" }), "a plain s").toBe(false);
  });

  for (const mod of mods) {
    test(`sc-13 with the settings or the WiFi overlay open (not only the help card) ${mod}+P, ${mod}+K and ${mod}+Shift+A do nothing; closed again they work`, async ({ page }) => {
      const calls = await recordCalls(page);
      await openStudio(page);
      const pane = page.locator("#chat-pane");
      const chatBefore = await pane.isHidden();
      for (const [opener, overlay] of [["#btn-settings", "#settings-overlay"], ["#btn-wifi", "#wifi-overlay"]] as const) {
        await page.click(opener);
        await expect(page.locator(overlay)).toBeVisible();
        const listBefore = count(calls, "ListDevices");
        await page.locator("body").press(`${mod}+p`);
        await page.locator("body").press(`${mod}+k`);
        await page.locator("body").press(`${mod}+Shift+a`);
        await page.waitForTimeout(300);
        expect(count(calls, "PickWorkspace"), `${overlay}: PickWorkspace`).toBe(0);
        expect(count(calls, "ListDevices"), `${overlay}: ListDevices`).toBe(listBefore);
        expect(await pane.isHidden(), `${overlay}: chat pane`).toBe(chatBefore);
        await page.keyboard.press("Escape");
        await expect(page.locator(overlay)).toBeHidden();
      }
      await page.locator("body").press(`${mod}+p`);
      await expect.poll(() => count(calls, "PickWorkspace")).toBe(1);
    });
  }
});
