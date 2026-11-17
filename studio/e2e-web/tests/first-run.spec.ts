import { expect, Page, test } from "@playwright/test";
import { ChildProcess, spawn } from "node:child_process";
import * as crypto from "node:crypto";
import * as fs from "node:fs";
import * as net from "node:net";
import * as os from "node:os";
import * as path from "node:path";

// First-run guidance, with no device: a Studio that starts with no workspace, with an empty one, and (in the shared server) with the
// fixture workspace. Each case starts its own headless Studio with a fresh HOME, so nothing a person saved is there. The hints say what to do
// next, every disabled primary button says why (as a title and as a description the button points to), nothing is said while the
// workspace is still loading, and a hint goes away once its step is done.

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

async function startStudio(workspaceDir: string): Promise<{ url: string; token: string; stop: () => void }> {
  const token = crypto.randomBytes(32).toString("hex"); // this server's own token: the page is given it as a header, never by Studio
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "studio-first-run-"));
  const port = await freePort();
  const child: ChildProcess = spawn(bin, [], {
    env: {
      ...process.env,
      HOME: home,
      PROBE_STUDIO_HEADLESS: "1",
      PROBE_STUDIO_HEADLESS_PORT: String(port),
      PROBE_STUDIO_HEADLESS_TOKEN: token,
      PROBE_STUDIO_WORKSPACE: workspaceDir,
      PROBE_STUDIO_KEYCHAIN: "memory",
      PROBE_STUDIO_SEED_RUN: "",
      PROBE_STUDIO_SEED_EXCHANGES: "",
    },
    stdio: "ignore",
  });
  const url = `http://127.0.0.1:${port}/`;
  for (let i = 0; i < 100; i++) {
    try {
      const r = await fetch(url);
      if (r.ok) break;
    } catch {
      /* not up yet */
    }
    await new Promise((resolve) => setTimeout(resolve, 150));
  }
  return { url, token, stop: () => child.kill() };
}

const strip = (page: Page) => page.locator("#next-step");
const primary = ["btn-run", "btn-record", "btn-connect"] as const;

/** Every disabled primary button has a reason: its title and the description it points to say the same thing, in words. */
async function expectReasons(page: Page, buttons: readonly string[]) {
  for (const id of buttons) {
    const btn = page.locator(`#${id}`);
    await expect(btn).toBeDisabled();
    const reasonId = await btn.getAttribute("aria-describedby");
    expect(reasonId, `${id} has no description`).toBeTruthy();
    const reason = (await page.locator(`#${reasonId}`).textContent()) ?? "";
    expect(reason.length, `${id} has an empty reason`).toBeGreaterThan(5);
    await expect(btn).toHaveAttribute("title", reason);
  }
}

test.describe("First-run guidance", () => {
  test.skip(!bin, "STUDIO_BIN is not set (run scripts/studio-e2e-web.sh)");

  test("fr-01 with no workspace: the next step, a button for it, and a reason on every disabled button", async ({ page }) => {
    const studio = await startStudio("");
    try {
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      await expect(page.locator("#topbar")).toBeVisible();
      await expect(strip(page)).toContainText("Open your project folder");
      await expect(page.locator("#file-list .pane-empty")).toContainText("No workspace open");
      await expect(page.locator("#btn-workspace-hint")).toBeVisible();
      await expectReasons(page, primary);
      await expect(page.locator("#reason-run")).toHaveText("Open a .probe file first.");
      await expect(page.locator("#reason-record")).toHaveText("Connect a device first.");
      // the placeholder in the editor does not send the person to a list that is empty
      await expect(page.locator("#editor .view-lines")).toContainText("scratch buffer");
      await expect(page.locator("#editor .view-lines")).not.toContainText("from the left");
    } finally {
      studio.stop();
    }
  });

  test("fr-02 the button in the empty pane is reachable from the keyboard and does what the folder button does", async ({ page }) => {
    const studio = await startStudio("");
    try {
      const calls: string[] = [];
      page.on("request", (r) => {
        const m = /"method":"(\w+)"/.exec(r.postData() ?? "");
        if (m) calls.push(m[1]);
      });
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      const hint = page.locator("#btn-workspace-hint");
      await expect(hint).toBeVisible();
      await hint.focus();
      await expect(hint).toBeFocused();
      await page.keyboard.press("Enter");
      await expect.poll(() => calls.filter((c) => c === "PickWorkspace").length).toBe(1); // the same call as the folder button
    } finally {
      studio.stop();
    }
  });

  test("fr-03 nothing is said while the workspace is loading, and the line comes once, after it", async ({ page }) => {
    const empty = fs.mkdtempSync(path.join(os.tmpdir(), "studio-empty-ws-"));
    const studio = await startStudio(empty);
    try {
      // Hold the first directory listing back, and record every text that appears in the pane and the strip until it is let go.
      let release!: () => void;
      const held = new Promise<void>((resolve) => (release = resolve));
      await page.route("**/__headless/call", async (route) => {
        if ((route.request().postData() ?? "").includes('"method":"ListDir"')) await held;
        await route.continue();
      });
      await page.addInitScript(() => {
        (window as any).__said = [] as string[];
        new MutationObserver(() => {
          const pane = document.querySelector("#file-list .pane-empty")?.textContent;
          const step = (document.getElementById("next-step") as HTMLElement | null);
          if (pane) (window as any).__said.push("pane:" + pane);
          if (step && !step.hidden && step.textContent) (window as any).__said.push("strip:" + step.textContent);
        }).observe(document, { subtree: true, childList: true, characterData: true, attributes: true });
      });
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      await expect(page.locator("#topbar")).toBeVisible();
      await page.waitForTimeout(800); // long enough for a flash
      await expect(strip(page)).toBeHidden(); // the strip itself is not shown empty while loading
      expect(await page.evaluate(() => (window as any).__said)).toEqual([]); // nothing, while loading
      await expect(page.locator("#file-list .pane-empty")).toHaveCount(0);
      release();
      await expect(page.locator("#file-list .pane-empty")).toContainText("No .probe files here");
      await expect(page.locator("#file-list .pane-empty")).toHaveCount(1); // once, not once per refresh
    } finally {
      studio.stop();
      fs.rmSync(empty, { recursive: true, force: true });
    }
  });

  test("fr-04 with a workspace that has files: no pane hint, the next step is to open a file, and it goes when one is open", async ({ page }) => {
    const ws = fs.mkdtempSync(path.join(os.tmpdir(), "studio-ws-"));
    fs.writeFileSync(path.join(ws, "a.probe"), 'test "a"\n  see "x"\n');
    const studio = await startStudio(ws);
    try {
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      await expect(page.locator("#file-list li", { hasText: "a.probe" })).toBeVisible();
      await expect(page.locator("#file-list .pane-empty")).toHaveCount(0);
      await expect(strip(page)).toContainText("Open a .probe file");
      await expect(page.locator("#reason-run")).toHaveText("Open a .probe file first.");
      await page.locator("#file-list li", { hasText: "a.probe" }).click();
      await expect(page.locator("#editor-filename")).toContainText("a.probe");
      // the file step is done: the strip no longer asks for it, and Run's reason is now the connection
      await expect(strip(page)).not.toContainText("Open a .probe file");
      await expect(page.locator("#reason-run")).toHaveText("Connect a device first.");
      await expect(page.locator("#btn-run")).toHaveAttribute("aria-describedby", "reason-run");
      // Save: nothing to save, and then a reason is no longer given once there are changes
      await expect(page.locator("#reason-save")).toHaveText("There are no unsaved changes.");
      await page.locator("#editor .view-line").first().click();
      await page.keyboard.press("End");
      await page.keyboard.type("!");
      await expect(page.locator("#btn-save")).toBeEnabled();
      await expect(page.locator("#btn-save")).not.toHaveAttribute("aria-describedby", /.+/);
      await expect(page.locator("#reason-save")).toHaveText("");
    } finally {
      studio.stop();
      fs.rmSync(ws, { recursive: true, force: true });
    }
  });

  test("fr-06 a remembered workspace that is gone or empty: the pane and the strip name the folder, not a file", async ({ page }) => {
    const empty = fs.mkdtempSync(path.join(os.tmpdir(), "studio-empty-ws-"));
    const gone = path.join(os.tmpdir(), "studio-gone-ws-does-not-exist");
    const studio = await startStudio("");
    try {
      // the window remembers the last workspace; the folder has since gone
      await page.addInitScript((p) => localStorage.setItem("fp.studio.workspace", p), gone);
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      await expect(page.locator("#file-list .pane-empty")).toContainText(/cannot be read|No \.probe files/);
      await expect(strip(page)).not.toContainText("Open a .probe file from the Workspace pane");
      await expect(strip(page)).toContainText("open another folder");
      await expect(page.locator("#btn-workspace-hint")).toBeVisible();
      // a remembered folder that exists but is empty
      const page2 = await page.context().newPage();
      await page2.addInitScript((p) => localStorage.setItem("fp.studio.workspace", p), empty);
      await page2.goto(studio.url);
      await expect(page2.locator("#file-list .pane-empty")).toContainText("No .probe files here");
      await expect(strip(page2)).toContainText("no .probe files in this folder");
      await expectReasons(page2, ["btn-run", "btn-record", "btn-connect"]);
      // the folder goes while Studio is open: a refresh says so (this path is after the start-up, where the pane line is drawn at once)
      fs.rmSync(empty, { recursive: true, force: true });
      await page2.click("#btn-files-refresh");
      await expect(page2.locator("#file-list .pane-empty")).toContainText("cannot be read");
      await expect(strip(page2)).toContainText("cannot be read");
    } finally {
      studio.stop();
      fs.rmSync(empty, { recursive: true, force: true });
    }
  });

  test("fr-05 the guidance is text only: no element inside the strip or the reasons", async ({ page }) => {
    const ws = fs.mkdtempSync(path.join(os.tmpdir(), "studio-ws-"));
    fs.writeFileSync(path.join(ws, "x.probe"), 'test "a"\n  see "x"\n');
    const studio = await startStudio(ws);
    try {
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      await expect(page.locator("#next-step")).toContainText("Open a .probe file");
      expect(await page.locator("#next-step").evaluate((el) => el.children.length)).toBe(0); // text only, no element inside
      for (const id of ["reason-run", "reason-record", "reason-connect", "reason-save"]) {
        expect(await page.locator(`#${id}`).evaluate((el) => el.children.length)).toBe(0);
      }
    } finally {
      studio.stop();
      fs.rmSync(ws, { recursive: true, force: true });
    }
  });

  test("fr-07 a workspace can be closed: the guidance of a first start comes back, the remembered folder is forgotten, and unsaved changes are asked about", async ({ page }) => {
    const ws = fs.mkdtempSync(path.join(os.tmpdir(), "studio-close-ws-"));
    fs.mkdirSync(path.join(ws, "tests"));
    fs.writeFileSync(path.join(ws, "tests", "a.probe"), 'test "a"\n  open the app\n');
    const studio = await startStudio(ws);
    try {
      await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
      await page.goto(studio.url);
      const close = page.locator("#btn-close-workspace");
      await expect(close).toBeVisible(); // a workspace is open
      await page.locator("#file-list li", { hasText: "tests" }).first().click();
      await page.locator("#file-list li", { hasText: "a.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("a.probe");
      // unsaved changes: the question in the page; Keep editing leaves everything as it was
      await page.locator("#editor .view-line").first().click();
      await page.keyboard.type("!!");
      await expect(page.locator("#editor-dirty")).toContainText("unsaved");
      await close.click();
      await expect(page.locator("#discard-overlay")).toBeVisible();
      await page.click("#btn-discard-cancel");
      await expect(page.locator("#discard-overlay")).toBeHidden();
      await expect(close).toBeVisible();
      await expect(page.locator("#editor-filename")).toContainText("a.probe");
      // Discard changes closes it
      await close.click();
      await expect(page.locator("#btn-discard-ok")).toBeEnabled(); // armed after half a second
      await page.click("#btn-discard-ok");
      await expect(page.locator("#file-list .pane-empty")).toContainText("No workspace open");
      await expect(strip(page)).toContainText("Open your project folder");
      await expect(close).toBeHidden();
      await expect(page.locator("#editor-filename")).toHaveText("untitled.probe");
      await expectReasons(page, ["btn-run"]);
      expect(await page.evaluate(() => Object.keys(localStorage).filter((k) => /workspace/i.test(k)).map((k) => localStorage.getItem(k)))).toEqual([]);
    } finally {
      studio.stop();
    }
  });
});
