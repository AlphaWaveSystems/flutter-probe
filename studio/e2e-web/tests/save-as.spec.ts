import { expect, Page, test } from "@playwright/test";
import { ChildProcess, spawn } from "node:child_process";
import * as crypto from "node:crypto";
import * as fs from "node:fs";
import * as net from "node:net";
import * as os from "node:os";
import * as path from "node:path";

// Save as: a buffer with no file (a recording, a scratch buffer) is saved as a new file in the workspace. The name is asked in the page, a file that
// exists is never replaced, a name with a path or a hidden character is refused in the dialog, and the new file becomes the open one. Each case starts
// its own headless Studio on a fresh workspace, so nothing is written into the shared fixture.

const bin = process.env.STUDIO_BIN ?? "";

async function freePort(): Promise<number> {
  return new Promise((resolve) => {
    const s = net.createServer();
    s.listen(0, "127.0.0.1", () => {
      const port = (s.address() as net.AddressInfo).port;
      s.close(() => resolve(port));
    });
  });
}

async function startStudio(files: Record<string, string> = {}): Promise<{ url: string; token: string; ws: string; stop: () => void }> {
  const token = crypto.randomBytes(32).toString("hex");
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "studio-saveas-home-"));
  const ws = fs.mkdtempSync(path.join(os.tmpdir(), "studio-saveas-ws-"));
  for (const [name, text] of Object.entries(files)) fs.writeFileSync(path.join(ws, name), text);
  const port = await freePort();
  const child: ChildProcess = spawn(bin, [], {
    env: { ...process.env, HOME: home, PROBE_STUDIO_HEADLESS: "1", PROBE_STUDIO_HEADLESS_PORT: String(port), PROBE_STUDIO_HEADLESS_TOKEN: token, PROBE_STUDIO_WORKSPACE: ws, PROBE_STUDIO_KEYCHAIN: "memory", PROBE_STUDIO_SEED_RUN: "", PROBE_STUDIO_SEED_EXCHANGES: "" },
    stdio: "ignore",
  });
  const url = `http://127.0.0.1:${port}/`;
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(url)).ok) break;
    } catch {
      /* not up yet */
    }
    await new Promise((r) => setTimeout(r, 150));
  }
  return { url, token, ws, stop: () => child.kill() };
}

async function typeScratch(page: Page, text: string) {
  await page.locator("#editor .view-lines").click();
  await page.keyboard.type(text);
  await expect(page.locator("#editor-dirty")).toContainText("unsaved");
}

const overlay = (page: Page) => page.locator("#save-as-overlay");
const nameField = (page: Page) => page.locator("#save-as-name");

test.describe("Save as", () => {
  test.skip(!bin, "STUDIO_BIN is not set (run scripts/studio-e2e-web.sh)");

  test("sa-01 a scratch buffer is saved under the name asked for, and the new file is the open one", async ({ page }) => {
    const studio = await startStudio({ "keep.probe": 'test "keep"\n  open the app\n' });
    try {
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      await expect(page.locator("#btn-save")).toBeDisabled(); // nothing to save yet
      await typeScratch(page, 'test "new"');
      await expect(page.locator("#btn-save")).toBeEnabled(); // a buffer with no file can be saved
      await page.click("#btn-save");
      await expect(overlay(page)).toBeVisible();
      await expect(nameField(page)).toBeFocused();
      await expect(nameField(page)).toHaveValue("untitled.probe");
      await nameField(page).fill("my flow");
      await page.keyboard.press("Enter");
      await expect(overlay(page)).toBeHidden();
      const file = path.join(studio.ws, "my flow.probe");
      expect(fs.readFileSync(file, "utf8")).toContain('test "new"');
      await expect(page.locator("#editor-filename")).toContainText("my flow.probe");
      await expect(page.locator("#editor-dirty")).toBeEmpty();
      await expect(page.locator("#btn-save")).toBeDisabled(); // saved: no unsaved changes
      await expect(page.locator("#file-list li", { hasText: "my flow.probe" })).toHaveCount(1);
      expect(fs.readFileSync(path.join(studio.ws, "keep.probe"), "utf8")).toBe('test "keep"\n  open the app\n');
    } finally {
      studio.stop();
    }
  });

  test("sa-02 a name that exists is refused in the dialog and nothing is replaced", async ({ page }) => {
    const original = 'test "keep"\n  open the app\n';
    const studio = await startStudio({ "keep.probe": original });
    try {
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      await typeScratch(page, 'test "other"');
      await page.click("#btn-save");
      await nameField(page).fill("KEEP"); // compared without case
      await page.click("#btn-save-as-ok");
      await expect(page.locator("#save-as-error")).toContainText("already exists");
      await expect(overlay(page)).toBeVisible();
      expect(fs.readFileSync(path.join(studio.ws, "keep.probe"), "utf8")).toBe(original);
      await nameField(page).fill("other");
      await page.click("#btn-save-as-ok");
      await expect(overlay(page)).toBeHidden();
      expect(fs.existsSync(path.join(studio.ws, "other.probe"))).toBe(true);
    } finally {
      studio.stop();
    }
  });

  test("sa-03 a path, a hidden file, another extension or a hidden character is refused, and no file is written", async ({ page }) => {
    const studio = await startStudio();
    try {
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      await typeScratch(page, 'test "x"');
      await page.click("#btn-save");
      for (const bad of ["../escape.probe", "a/b.probe", ".hidden.probe", "x.txt", "a​b.probe", ""]) {
        await nameField(page).fill(bad);
        await page.click("#btn-save-as-ok");
        await expect(page.locator("#save-as-error")).not.toBeEmpty();
        await expect(overlay(page)).toBeVisible();
      }
      expect(fs.readdirSync(studio.ws)).toEqual([]);
      expect(fs.existsSync(path.join(path.dirname(studio.ws), "escape.probe"))).toBe(false);
    } finally {
      studio.stop();
    }
  });

  test("sa-04 Escape and Cancel write nothing and leave the buffer unsaved", async ({ page }) => {
    const studio = await startStudio();
    try {
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      await typeScratch(page, 'test "x"');
      await page.click("#btn-save");
      await expect(overlay(page)).toBeVisible(); // it opens after the folder has been listed
      await page.keyboard.press("Escape");
      await expect(overlay(page)).toBeHidden();
      await page.click("#btn-save");
      await expect(overlay(page)).toBeVisible();
      await page.click("#btn-save-as-cancel");
      await expect(overlay(page)).toBeHidden();
      await page.click("#btn-save");
      await expect(overlay(page)).toBeVisible();
      await page.mouse.click(5, 5); // the dark area is not an answer either
      await page.waitForTimeout(300);
      await expect(overlay(page)).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(overlay(page)).toBeHidden();
      await expect(page.locator("#editor-dirty")).toContainText("unsaved");
      expect(fs.readdirSync(studio.ws)).toEqual([]);
    } finally {
      studio.stop();
    }
  });

  test("sa-05 when Studio refuses the write because the file appeared since the listing, the buffer stays unsaved and the toast says why", async ({ page }) => {
    const studio = await startStudio();
    try {
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      // Go's own refusal is tested in Go; here the page is shown what it does with it
      await page.route("**/__headless/call", async (route) => {
        if ((route.request().postData() ?? "").includes('"method":"CreateFile"')) return route.fulfill({ json: { error: "taken.probe already exists: Save never replaces a file" } });
        await route.continue();
      });
      await page.goto(studio.url);
      await typeScratch(page, 'test "x"');
      await page.click("#btn-save");
      await nameField(page).fill("taken");
      await page.click("#btn-save-as-ok");
      await expect(page.locator("#toast-container")).toContainText("already exists: Save never replaces a file");
      await expect(page.locator("#editor-dirty")).toContainText("unsaved");
      await expect(page.locator("#editor-filename")).not.toContainText("taken.probe");
      expect(fs.readdirSync(studio.ws)).toEqual([]);
    } finally {
      studio.stop();
    }
  });
});
