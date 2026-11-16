import { expect, Page, test } from "@playwright/test";
import { ChildProcess, spawn } from "node:child_process";
import * as fs from "node:fs";
import * as net from "node:net";
import * as os from "node:os";
import * as path from "node:path";

// The note in the Workspace pane about probe.yaml: what will trip a first run (no file, a file that cannot be read, no project.app), in plain
// text, hidden when there is nothing to say. Each case starts its own headless Studio on its own folder.

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

async function startStudio(workspaceDir: string): Promise<{ url: string; stop: () => void }> {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "studio-wn-home-"));
  const port = await freePort();
  const child: ChildProcess = spawn(bin, [], {
    env: { ...process.env, HOME: home, PROBE_STUDIO_HEADLESS: "1", PROBE_STUDIO_HEADLESS_PORT: String(port), PROBE_STUDIO_WORKSPACE: workspaceDir, PROBE_STUDIO_KEYCHAIN: "memory", PROBE_STUDIO_SEED_RUN: "", PROBE_STUDIO_SEED_EXCHANGES: "" },
    stdio: "ignore",
  });
  const url = `http://127.0.0.1:${port}/`;
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(url)).ok) break;
    } catch {
      /* not up yet */
    }
    await new Promise((resolve) => setTimeout(resolve, 150));
  }
  return { url, stop: () => child.kill() };
}

function workspaceWith(yaml: string | null): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "studio-wn-ws-"));
  fs.mkdirSync(path.join(dir, "tests"));
  fs.writeFileSync(path.join(dir, "tests", "a.probe"), 'test "a"\n  log "x"\n');
  if (yaml !== null) fs.writeFileSync(path.join(dir, "probe.yaml"), yaml);
  return dir;
}

const note = (page: Page) => page.locator("#workspace-note");

test.describe("Workspace note about probe.yaml", () => {
  test.skip(!bin, "STUDIO_BIN is not set (run scripts/studio-e2e-web.sh)");

  async function withStudio(dir: string, body: (page: Page, url: string) => Promise<void>, page: Page) {
    const studio = await startStudio(dir);
    try {
      await page.goto(studio.url);
      await expect(page.locator("#file-list li", { hasText: "tests" })).toBeVisible();
      await body(page, studio.url);
    } finally {
      studio.stop();
      fs.rmSync(dir, { recursive: true, force: true });
    }
  }

  test("wn-01 no probe.yaml: the note says runs use the defaults and no app id", async ({ page }) => {
    await withStudio(workspaceWith(null), async () => {
      await expect(note(page)).toBeVisible();
      await expect(note(page)).toContainText("No probe.yaml in this folder");
      await expect(note(page)).toContainText("project.app");
    }, page);
  });

  test("wn-02 probe.yaml without project.app: the note says what will fail", async ({ page }) => {
    await withStudio(workspaceWith("agent:\n  port: 48690\n"), async () => {
      await expect(note(page)).toContainText("has no project.app");
      await expect(note(page)).toContainText("restart the app");
    }, page);
  });

  test("wn-03 a good probe.yaml: nothing is said", async ({ page }) => {
    await withStudio(workspaceWith("project:\n  app: com.example.app\n"), async () => {
      await page.waitForTimeout(500);
      await expect(note(page)).toBeHidden();
      await expect(note(page)).toHaveText("");
    }, page);
  });

  test("wn-04 a probe.yaml that cannot be read is named, with no path and one line", async ({ page }) => {
    const dir = workspaceWith("project: [unclosed\n");
    await withStudio(dir, async () => {
      await expect(note(page)).toContainText("probe.yaml cannot be read, so Studio is not using it");
      const text = (await note(page).textContent()) ?? "";
      expect(text).not.toContain(dir);
      expect(text).not.toContain("\n");
    }, page);
  });

  test("wn-05 the refresh button reads probe.yaml again: a file edited outside Studio updates the note", async ({ page }) => {
    const dir = workspaceWith("agent:\n  port: 48690\n");
    await withStudio(dir, async () => {
      await expect(note(page)).toContainText("has no project.app");
      fs.writeFileSync(path.join(dir, "probe.yaml"), "project:\n  app: com.example.app\n");
      await page.click("#btn-files-refresh");
      await expect(note(page)).toBeHidden();
      fs.rmSync(path.join(dir, "probe.yaml"));
      await page.click("#btn-files-refresh");
      await expect(note(page)).toContainText("No probe.yaml in this folder");
    }, page);
  });

  test("wn-06 the note is plain text: markup in an error is not interpreted", async ({ page }) => {
    await withStudio(workspaceWith("project:\n  app: \"<img src=x onerror=alert(1)>\"\n"), async () => {
      await expect(note(page)).toContainText("cannot be read");
      expect(await note(page).evaluate((el) => el.children.length)).toBe(0);
    }, page);
  });
});
