import { expect, Page, test } from "@playwright/test";
import { openStudio, workspace } from "./helpers";

// The line under the Device title about the build of the app under test: shown only when the app reported it, text only, gone with the connection.
// The Go calls are answered by hand so the spec does not depend on an agent that sends the facts.

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

async function connectWith(page: Page, appBuild?: string) {
  await page.route("**/__headless/call", async (route) => {
    const body = route.request().postData() ?? "";
    if (body.includes('"method":"ListDevices"')) return route.fulfill({ json: { result: [{ id: "emu-1", name: "Pixel Test", platform: "android", kind: "emulator", state: "device", booted: true }] } });
    if (body.includes('"method":"DeviceListNote"')) return route.fulfill({ json: { result: "" } });
    if (body.includes('"method":"Connect"')) return route.fulfill({ json: { result: { connected: true, deviceId: "emu-1", deviceName: "Pixel Test", platform: "android", ...(appBuild === undefined ? {} : { appBuild }) } } });
    await route.continue();
  });
  await openStudio(page);
  await page.click("#btn-refresh");
  await page.locator("#device-picker").selectOption({ label: "Pixel Test · android" });
  await page.click("#btn-connect");
  await expect(page.locator("#toast-container .toast", { hasText: "Connected to Pixel Test" })).toBeVisible();
}

test.describe("The build of the app under test", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  test.beforeEach(async ({ page }) => {
    await captureEvents(page);
  });
  const line = (page: Page) => page.locator("#app-build");

  test("bv-01 a connection whose app reported its build shows the line, with the caveat on hover", async ({ page }) => {
    await connectWith(page, "App built with Flutter 3.44.8 (stable), Dart 3.12.2 · framework revision 1a2b3c4d5e");
    await expect(line(page)).toBeVisible();
    await expect(line(page)).toHaveText("App built with Flutter 3.44.8 (stable), Dart 3.12.2 · framework revision 1a2b3c4d5e");
    await expect(line(page)).toHaveAttribute("title", /Only the parts the app reported are shown.*a fork or a custom embedder/);
  });

  test("bv-02 a connection whose app reported nothing shows nothing: no placeholder", async ({ page }) => {
    await connectWith(page, undefined);
    await expect(line(page)).toBeHidden();
    await expect(line(page)).toHaveText("");
    await expect(page.locator("#device-pane")).not.toContainText("Flutter");
  });

  test("bv-03 the line is text, never markup", async ({ page }) => {
    await connectWith(page, "<img src=x onerror=\"window.__xss=1\"> App built with Flutter 1");
    await expect(line(page)).toContainText("<img src=x");
    await expect(line(page).locator("img")).toHaveCount(0);
    expect(await page.evaluate(() => (window as any).__xss)).toBeUndefined();
  });

  test("bv-04 the line goes with the connection", async ({ page }) => {
    await connectWith(page, "App built with Dart 3.12.2");
    await expect(line(page)).toBeVisible();
    await fire(page, "connection:changed", { connected: false, deviceId: "", deviceName: "", platform: "" });
    await expect(line(page)).toBeHidden();
    await fire(page, "connection:changed", { connected: true, deviceId: "emu-1", deviceName: "Pixel Test", platform: "android", appBuild: "App built with Flutter 3.44.8" });
    await expect(line(page)).toHaveText("App built with Flutter 3.44.8");
  });

  test("bv-05 a long line stays inside the Device pane: it wraps, and does not reach the editor", async ({ page }) => {
    const long = "App built with Flutter 3.44.8 (stable), Dart 3.12.2 · framework revision " + "1a2b3c4d5e".repeat(12) + " · engine " + "f".repeat(80);
    await connectWith(page, long);
    await expect(line(page)).toBeVisible();
    const boxes = await page.evaluate(() => {
      const r = (id: string) => document.getElementById(id)!.getBoundingClientRect();
      const l = document.getElementById("app-build")!;
      return { line: r("app-build"), pane: r("device-pane"), editor: r("editor-pane"), overflowX: l.scrollWidth > l.clientWidth };
    });
    expect(boxes.overflowX).toBe(false);
    expect(boxes.line.left).toBeGreaterThanOrEqual(boxes.pane.left - 0.5);
    expect(boxes.line.right).toBeLessThanOrEqual(boxes.pane.right + 0.5);
    expect(boxes.line.left).toBeGreaterThanOrEqual(boxes.editor.right - 0.5); // not over the editor
  });
});
