import { expect, Page, test } from "@playwright/test";
import { openStudio, workspace } from "./helpers";

// The device list is refreshed at start-up and again on every ↻. A refresh must not take the list away from under the person: a native dropdown that
// is open when the list is rebuilt loses the pick (the window check: the first selection after a launch was ignored, the second worked). The Go calls
// are answered by hand so the spec does not depend on what runs on this machine, and DeviceListNote can be held back to show what the picker does
// while a refresh is in flight.

const dev = (id: string, name: string, booted = true) => ({ id, name, platform: "android", kind: "emulator", state: "device", booted });
const two = [dev("emu-1", "Pixel One"), dev("emu-2", "Pixel Two")];

async function answer(page: Page, devices: () => unknown[], noteDelayMs = 0) {
  await page.route("**/__headless/call", async (route) => {
    const body = route.request().postData() ?? "";
    if (body.includes('"method":"ListDevices"')) return route.fulfill({ json: { result: devices() } });
    if (body.includes('"method":"DeviceListNote"')) {
      if (noteDelayMs) await new Promise((r) => setTimeout(r, noteDelayMs));
      return route.fulfill({ json: { result: "" } });
    }
    await route.continue();
  });
}

const options = (page: Page) => page.locator("#device-picker option").evaluateAll((os) => os.map((o) => (o as HTMLOptionElement).textContent));

test.describe("Device picker", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test("dp-01 while a refresh is in flight the picker keeps the list it has: it is never emptied and refilled", async ({ page }) => {
    await answer(page, () => two, 1500);
    await openStudio(page);
    await expect.poll(async () => (await options(page)).length, { timeout: 10_000 }).toBe(3);
    await page.click("#btn-refresh");
    await page.waitForTimeout(400); // the refresh is waiting for DeviceListNote
    expect((await options(page)).length).toBe(3);
    await expect.poll(async () => (await options(page)).length, { timeout: 10_000 }).toBe(3);
  });

  test("dp-02 a refresh that finds the same devices leaves the picker alone: the same options, the same pick", async ({ page }) => {
    await answer(page, () => two);
    await openStudio(page);
    await expect.poll(async () => (await options(page)).length, { timeout: 10_000 }).toBe(3);
    await page.locator("#device-picker").selectOption({ label: "Pixel Two  ·  android" });
    await page.evaluate(() => ((document.querySelectorAll("#device-picker option")[2] as any).__mark = "kept"));
    await page.click("#btn-refresh");
    await page.waitForTimeout(500);
    expect(await page.evaluate(() => (document.querySelectorAll("#device-picker option")[2] as any).__mark)).toBe("kept");
    await expect(page.locator("#device-picker")).toHaveValue("emu-2");
    await expect(page.locator("#btn-connect")).toBeEnabled();
  });

  test("dp-03 a refresh that finds other devices rebuilds the list and keeps the pick when that device is still there", async ({ page }) => {
    let devices: unknown[] = two;
    await answer(page, () => devices);
    await openStudio(page);
    await expect.poll(async () => (await options(page)).length, { timeout: 10_000 }).toBe(3);
    await page.locator("#device-picker").selectOption({ label: "Pixel Two  ·  android" });
    devices = [...two, dev("emu-3", "Pixel Three")];
    await page.click("#btn-refresh");
    await expect.poll(async () => (await options(page)).length, { timeout: 10_000 }).toBe(4);
    await expect(page.locator("#device-picker")).toHaveValue("emu-2");
    devices = [dev("emu-3", "Pixel Three")]; // the pick is gone: nothing is selected and Connect is off
    await page.click("#btn-refresh");
    await expect.poll(async () => (await options(page)).length, { timeout: 10_000 }).toBe(2);
    await expect(page.locator("#device-picker")).toHaveValue("");
    await expect(page.locator("#btn-connect")).toBeDisabled();
  });

  test("dp-04 an older refresh that finishes after a newer one does not put its list back", async ({ page }) => {
    // The first refresh (start-up) is held at DeviceListNote; a click on the refresh button starts a second one that finishes first with another list.
    // When the first one is let go, its list is out of date and the seq check after the note must drop it.
    const three = [...two, dev("emu-3", "Pixel Three")];
    let lists = 0;
    let notes = 0;
    let releaseFirstNote: () => void = () => {};
    const firstNoteHeld = new Promise<void>((r) => (releaseFirstNote = r));
    await page.route("**/__headless/call", async (route) => {
      const body = route.request().postData() ?? "";
      if (body.includes('"method":"ListDevices"')) return route.fulfill({ json: { result: lists++ === 0 ? two : three } });
      if (body.includes('"method":"DeviceListNote"')) {
        if (notes++ === 0) await firstNoteHeld;
        return route.fulfill({ json: { result: "" } });
      }
      await route.continue();
    });
    await openStudio(page);
    await expect.poll(() => notes, { timeout: 10_000 }).toBe(1); // the first refresh is now waiting for its note
    await page.click("#btn-refresh");
    await expect.poll(async () => (await options(page)).length, { timeout: 10_000 }).toBe(4); // the second one finished first
    releaseFirstNote();
    await page.waitForTimeout(600); // the first one resumes here and must not replace the newer list with its own
    expect((await options(page)).length).toBe(4);
  });
});
