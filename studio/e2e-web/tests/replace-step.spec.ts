import { expect, Page, test } from "@playwright/test";
import { ChildProcess, spawn } from "node:child_process";
import * as crypto from "node:crypto";
import * as fs from "node:fs";
import * as net from "node:net";
import * as os from "node:os";
import * as path from "node:path";

// "Use the text on screen": the On screen row under a failed see or tap offers the same step with the text the screen showed. Each case starts its own headless Studio whose run is seeded
// (a failed step with the rule's fix, and the text the run read), and the page is shown the same step by firing the run events. Studio must agree
// before the editor changes, the edit is one undo step, nothing is saved, and every refusal says why.

const bin = process.env.STUDIO_BIN ?? "";
const base = 'test "login"\n  open the app\n  tap "Sign in"\n  see "Welcome"\n';

async function freePort(): Promise<number> {
  return new Promise((resolve) => {
    const s = net.createServer();
    s.listen(0, "127.0.0.1", () => {
      const port = (s.address() as net.AddressInfo).port;
      s.close(() => resolve(port));
    });
  });
}

type Studio = { url: string; token: string; ws: string; stop: () => void };

/** A Studio on a fresh workspace holding tests/login.probe (the given text), seeded with one failed step at the given line. */
async function startStudio(loginText: string, failedLine = 3, description = 'tap "Sign in"', replacement = "Sign In"): Promise<Studio> {
  const token = crypto.randomBytes(32).toString("hex");
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "studio-replace-home-"));
  const ws = fs.mkdtempSync(path.join(os.tmpdir(), "studio-replace-ws-"));
  fs.mkdirSync(path.join(ws, "tests"));
  fs.writeFileSync(path.join(ws, "tests", "login.probe"), loginText);
  fs.writeFileSync(path.join(ws, "tests", "other.probe"), 'test "other"\n  open the app\n');
  fs.writeFileSync(path.join(ws, "tests", "dup.probe"), loginText); // a twin of login.probe: the same text in another file
  const seed = {
    results: [{ name: "login", file: "tests/login.probe", passed: false, skipped: false, durationMs: 9, error: "element not found" }],
    steps: [
      {
        file: "tests/login.probe", test: "login", line: failedLine, description, status: "failed", durationMs: 5, attempt: 0, depth: 0, error: "element not found",
        triage: { replacement, reason: `the screen shows "${replacement}"` },
      },
    ],
  };
  const seedPath = path.join(ws, "seed.json");
  fs.writeFileSync(seedPath, JSON.stringify(seed));
  const port = await freePort();
  const child: ChildProcess = spawn(bin, [], {
    env: {
      ...process.env, HOME: home, PROBE_STUDIO_HEADLESS: "1", PROBE_STUDIO_HEADLESS_PORT: String(port), PROBE_STUDIO_HEADLESS_TOKEN: token,
      PROBE_STUDIO_WORKSPACE: ws, PROBE_STUDIO_KEYCHAIN: "memory", PROBE_STUDIO_SEED_RUN: seedPath, PROBE_STUDIO_SEED_EXCHANGES: "",
    },
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

/** Fires the events of a run whose step at the line failed with the rule's fix (the same step the seed holds), as the page would get them. */
async function fireFailedStep(page: Page, line = 3, depth = 0, description = 'tap "Sign in"', replacement = "Sign In") {
  await page.evaluate(([l, d, desc, rep]) => {
    const fire = (name: string, ...args: unknown[]) => (((window as any).__handlers[name]) || []).forEach((cb: (...a: unknown[]) => void) => cb(...args));
    const base = { file: "tests/login.probe", test: "login", line: l, description: desc, durationMs: 5, attempt: 0, depth: d };
    fire("run:started", "tests/login.probe");
    fire("run:step", { ...base, status: "started" });
    fire("run:step", { ...base, status: "failed", error: "element not found", triage: { replacement: rep, reason: `the screen shows "${rep}"` } });
    fire("run:finished", []);
  }, [line, depth, description, replacement] as const);
}

async function openLogin(page: Page, studio: Studio) {
  await page.context().setExtraHTTPHeaders({ "X-Headless-Token": studio.token });
  await captureEvents(page);
  await page.goto(studio.url);
  await expect(page.locator("#topbar")).toBeVisible();
  await page.locator("#file-list li", { hasText: "tests" }).first().click();
  await page.locator("#file-list li", { hasText: "login.probe" }).first().click();
  await expect(page.locator("#editor-filename")).toContainText("login.probe");
}

/** Counts the Go calls the page makes, and proves in this test that it sees one (InitialWorkspace, a harmless read) before a zero is asserted on it. */
function recordCalls(page: Page) {
  const seen: Record<string, number> = {};
  page.on("request", (r) => {
    const m = /"method":"(\w+)"/.exec(r.postData() ?? "");
    if (m) seen[m[1]] = (seen[m[1]] ?? 0) + 1;
  });
  return seen;
}
async function expectNoCall(page: Page, seen: Record<string, number>, ...methods: string[]) {
  const before = seen.InitialWorkspace ?? 0;
  await page.evaluate(() => (window as any).go.main.App.InitialWorkspace());
  await expect.poll(() => seen.InitialWorkspace ?? 0).toBeGreaterThan(before);
  for (const m of methods) expect(seen[m] ?? 0, `${m} was called`).toBe(0);
}

const useButton = (page: Page) => page.locator("#results-list li.step-triage button.triage-replace");
const editorText = async (page: Page) => (await page.locator("#editor .view-lines").innerText()).replace(/\u00a0/g, " "); // the editor draws spaces as no-break spaces
const toasts = (page: Page) => page.locator("#toast-container");
const mod = process.platform === "darwin" ? "Meta" : "Control";

test.describe("Use the text on screen", () => {
  test.skip(!bin, "STUDIO_BIN is not set (run scripts/studio-e2e-web.sh)");

  test("rs-01 one press replaces the text of the step, one undo restores it and the button works again, a second press is refused", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      const calls = recordCalls(page);
      await fireFailedStep(page);
      await expect(useButton(page)).toHaveCount(1);
      await expect(useButton(page)).toHaveText('Use "Sign In"');
      await expect(page.locator("#results-list li.step-triage button.triage-insert")).toHaveCount(0); // the wait is not offered: the text is not late, it is spelled otherwise
      const before = (await editorText(page)).split("\n").length;
      await useButton(page).click();
      await expect(toasts(page)).toContainText("Line 3 now uses the text the screen showed");
      await expect(page.locator("#editor .view-lines")).toContainText('tap "Sign In"');
      await expect(page.locator("#editor .view-lines")).not.toContainText('tap "Sign in"');
      expect((await editorText(page)).split("\n").length).toBe(before); // the line count did not change
      expect(fs.readFileSync(path.join(studio.ws, "tests", "login.probe"), "utf8")).toBe(base); // nothing was saved
      await expectNoCall(page, calls, "WriteFile", "RunFile", "RunFileScoped");
      // a second press: the file now differs from the run's text
      await useButton(page).click();
      await expect(toasts(page)).toContainText("changed since the run");
      // one undo restores the line and nothing else, and then the text is the run's again
      await page.locator("#editor .view-line").first().click();
      await page.keyboard.press(`${mod}+z`);
      await expect(page.locator("#editor .view-lines")).toContainText('tap "Sign in"');
      await expect(page.locator("#editor .view-lines")).not.toContainText('tap "Sign In"');
      await useButton(page).click();
      await expect(page.locator("#editor .view-lines")).toContainText('tap "Sign In"');
    } finally {
      studio.stop();
    }
  });

  test("rs-02 an edit anywhere first is refused with the reason, and the editor is unchanged", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      await page.locator("#editor .view-line").first().click();
      await page.keyboard.press("End");
      await page.keyboard.type(" # note");
      await useButton(page).click();
      await expect(toasts(page)).toContainText("changed since the run");
      await expect(page.locator("#editor .view-lines")).not.toContainText('tap "Sign In"');
    } finally {
      studio.stop();
    }
  });

  test("rs-03 another open file is refused", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      await page.locator("#file-list li", { hasText: "other.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("other.probe");
      await useButton(page).click();
      await expect(toasts(page)).toContainText("not the file the failed step ran from");
      await expect(page.locator("#editor .view-lines")).not.toContainText('tap "Sign In"');
    } finally {
      studio.stop();
    }
  });

  test("rs-04 a step inside a recipe has no button", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page, 3, 1);
      await expect(page.locator("#results-list li.step-triage")).toHaveCount(2); // the On screen row and the reason are there
      await expect(useButton(page)).toHaveCount(0);
    } finally {
      studio.stop();
    }
  });

  test("rs-05 the button is off while a run is going and on again when it has finished", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      await expect(useButton(page)).toBeEnabled();
      await page.evaluate(() => (((window as any).__handlers["run:started"]) || []).forEach((cb: (...a: unknown[]) => void) => cb("tests/login.probe")));
      await expect(useButton(page)).toHaveCount(0); // the new run clears the rows of the last one
      await fireFailedStep(page); // this fires run:started, the step and run:finished again: the button is back
      await expect(useButton(page)).toBeEnabled();
    } finally {
      studio.stop();
    }
  });

  test("rs-06 a hostile on-screen text is shown as text, and a text a step line would read otherwise is refused", async ({ page }) => {
    const studio = await startStudio(base, 3, 'tap "Sign in"', 'Sign <img src=x onerror="window.__rsXss=1"> In');
    try {
      await openLogin(page, studio);
      await fireFailedStep(page, 3, 0, 'tap "Sign in"', 'Sign <img src=x onerror="window.__rsXss=1"> In');
      await expect(useButton(page)).toContainText("<img src=x");
      await expect(page.locator("#results-list li.step-triage img")).toHaveCount(0);
      expect(await page.evaluate(() => (window as any).__rsXss)).toBeUndefined();
      await useButton(page).click();
      await expect(toasts(page)).toContainText("Not replaced");
      await expect(page.locator("#editor .view-lines")).toContainText('tap "Sign in"');
    } finally {
      studio.stop();
    }
  });

  test("rs-07 a file with CRLF line endings is replaced the same way", async ({ page }) => {
    const studio = await startStudio(base.replace(/\n/g, "\r\n"));
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      await useButton(page).click();
      await expect(page.locator("#editor .view-lines")).toContainText('tap "Sign In"');
      await expect(page.locator("#editor .view-lines")).toContainText('see "Welcome"'); // the lines after it are as they were
    } finally {
      studio.stop();
    }
  });
});
