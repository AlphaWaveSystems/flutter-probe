import { expect, Page, test } from "@playwright/test";
import { ChildProcess, spawn } from "node:child_process";
import * as crypto from "node:crypto";
import * as fs from "node:fs";
import * as net from "node:net";
import * as os from "node:os";
import * as path from "node:path";

// "Insert before the step": the rule row under a failed see or tap offers its wait line. Each case starts its own headless Studio whose run is seeded
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
async function startStudio(loginText: string, failedLine = 3, description = 'tap "Sign in"'): Promise<Studio> {
  const token = crypto.randomBytes(32).toString("hex");
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "studio-insert-home-"));
  const ws = fs.mkdtempSync(path.join(os.tmpdir(), "studio-insert-ws-"));
  fs.mkdirSync(path.join(ws, "tests"));
  fs.writeFileSync(path.join(ws, "tests", "login.probe"), loginText);
  fs.writeFileSync(path.join(ws, "tests", "other.probe"), 'test "other"\n  open the app\n');
  fs.writeFileSync(path.join(ws, "tests", "dup.probe"), loginText); // a twin of login.probe: the same text in another file
  const selector = /"([^"]*)"/.exec(description)![1];
  const seed = {
    results: [{ name: "login", file: "tests/login.probe", passed: false, skipped: false, durationMs: 9, error: "element not found" }],
    steps: [
      {
        file: "tests/login.probe", test: "login", line: failedLine, description, status: "failed", durationMs: 5, attempt: 0, depth: 0, error: "element not found",
        triage: { fix: `wait until "${selector}" appears`, reason: "found no element" },
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
async function fireFailedStep(page: Page, line = 3, depth = 0, description = 'tap "Sign in"') {
  await page.evaluate(([l, d, desc]) => {
    const fire = (name: string, ...args: unknown[]) => (((window as any).__handlers[name]) || []).forEach((cb: (...a: unknown[]) => void) => cb(...args));
    const sel = /"([^"]*)"/.exec(desc as string)![1];
    const base = { file: "tests/login.probe", test: "login", line: l, description: desc, durationMs: 5, attempt: 0, depth: d };
    fire("run:started", "tests/login.probe");
    fire("run:step", { ...base, status: "started" });
    fire("run:step", { ...base, status: "failed", error: "element not found", triage: { fix: `wait until "${sel}" appears`, reason: "found no element" } });
    fire("run:finished", []);
  }, [line, depth, description] as const);
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

const insertButton = (page: Page) => page.locator("#results-list li.step-triage button.triage-insert");
const editorText = async (page: Page) => (await page.locator("#editor .view-lines").innerText()).replace(/\u00a0/g, " "); // the editor draws spaces as no-break spaces
const toasts = (page: Page) => page.locator("#toast-container");
const mod = process.platform === "darwin" ? "Meta" : "Control";

test.describe("Insert before the step", () => {
  test.skip(!bin, "STUDIO_BIN is not set (run scripts/studio-e2e-web.sh)");

  test("ib-01 one press inserts the wait above the step, one undo removes it and the button works again, a second press is refused", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      await expect(insertButton(page)).toHaveCount(1); // under the suggested fix only
      await insertButton(page).click();
      await expect(toasts(page)).toContainText("Inserted above line 3");
      await expect(page.locator("#editor .view-lines")).toContainText('wait until "Sign in" appears');
      expect(fs.readFileSync(path.join(studio.ws, "tests", "login.probe"), "utf8")).toBe(base); // nothing was saved
      await expect(page.locator("#editor .active-line-number")).toHaveText("3");
      // a second press: the file now differs from the run's text
      await insertButton(page).click();
      await expect(toasts(page)).toContainText("changed since the run");
      expect((await editorText(page)).match(/wait until/g)?.length).toBe(1);
      // one undo removes the line and nothing else, and then the text is the run's again
      await page.locator("#editor .view-line").first().click();
      await page.keyboard.press(`${mod}+z`);
      await expect(page.locator("#editor .view-lines")).not.toContainText("wait until");
      await insertButton(page).click();
      await expect(page.locator("#editor .view-lines")).toContainText('wait until "Sign in" appears');
    } finally {
      studio.stop();
    }
  });

  test("ib-02 an edit first is refused with the reason, and the editor is unchanged", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      await page.locator("#editor .view-line").first().click();
      await page.keyboard.press("End");
      await page.keyboard.type(" # note");
      await insertButton(page).click();
      await expect(toasts(page)).toContainText("changed since the run");
      await expect(page.locator("#editor .view-lines")).not.toContainText("wait until");
    } finally {
      studio.stop();
    }
  });

  test("ib-03 another open file is refused", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      await page.locator("#file-list li", { hasText: "other.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("other.probe");
      await insertButton(page).click();
      await expect(toasts(page)).toContainText("not the file the failed step ran from");
      await expect(page.locator("#editor .view-lines")).not.toContainText("wait until");
    } finally {
      studio.stop();
    }
  });

  test("ib-04 a step inside a recipe has no button", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page, 3, 1);
      await expect(page.locator("#results-list li.step-triage")).toHaveCount(2); // the rows are there
      await expect(insertButton(page)).toHaveCount(0);
    } finally {
      studio.stop();
    }
  });

  test("ib-05 after an insertion the rows of that run no longer jump, and say so", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      await insertButton(page).click();
      await expect(page.locator("#editor .view-lines")).toContainText("wait until");
      await page.locator("#editor .view-line").first().click(); // caret away from line 3
      await page.locator("#results-list li.step", { hasText: 'tap "Sign in"' }).first().click();
      await expect(toasts(page)).toContainText("run again before using its rows");
      await expect(page.locator("#editor .active-line-number")).not.toHaveText("3");
    } finally {
      studio.stop();
    }
  });

  test("ib-06 a CRLF file stays CRLF and a failed step on a last line with no final newline keeps having none", async ({ page }) => {
    const crlf = base.replace(/\n/g, "\r\n");
    const a = await startStudio(crlf);
    try {
      await openLogin(page, a);
      await fireFailedStep(page);
      await insertButton(page).click();
      await expect(page.locator("#editor .view-lines")).toContainText("wait until");
      await page.click("#btn-save");
      await expect(toasts(page)).toContainText("Saved");
      expect(fs.readFileSync(path.join(a.ws, "tests", "login.probe"), "utf8")).toBe('test "login"\r\n  open the app\r\n  wait until "Sign in" appears\r\n  tap "Sign in"\r\n  see "Welcome"\r\n');
    } finally {
      a.stop();
    }
    const last = 'test "login"\n  open the app\n  tap "Sign in"'; // the failed step is the last line, with no final newline
    const b = await startStudio(last);
    try {
      await openLogin(page, b);
      await fireFailedStep(page);
      await insertButton(page).click();
      await expect(page.locator("#editor .view-lines")).toContainText("wait until");
      await page.click("#btn-save");
      await expect(toasts(page)).toContainText("Saved");
      expect(fs.readFileSync(path.join(b.ws, "tests", "login.probe"), "utf8")).toBe('test "login"\n  open the app\n  wait until "Sign in" appears\n  tap "Sign in"');
    } finally {
      b.stop();
    }
  });

  test("ib-07 the answer is for the file of the click: switching to a twin file while Studio checks inserts nothing in either", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      // hold Studio's answer back, so there is time to open the twin (same text, other file) before it arrives
      await page.route("**/__headless/call", async (route) => {
        if ((route.request().postData() ?? "").includes('"method":"CheckInsertBefore"')) await new Promise((r) => setTimeout(r, 1200));
        await route.continue();
      });
      await insertButton(page).click();
      await page.locator("#file-list li", { hasText: "dup.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("dup.probe");
      await expect(toasts(page)).toContainText("another file was opened or the text changed");
      await expect(page.locator("#editor .view-lines")).not.toContainText("wait until"); // the twin has no wait
      await page.locator("#file-list li", { hasText: "login.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("login.probe");
      await expect(page.locator("#editor .view-lines")).not.toContainText("wait until"); // and neither has the file of the click
      expect(fs.readFileSync(path.join(studio.ws, "tests", "dup.probe"), "utf8")).toBe(base);
    } finally {
      studio.stop();
    }
  });

  test("ib-08 the button is off while a run is going, and on again when it has finished", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await page.evaluate(() => {
        const fire = (name: string, ...args: unknown[]) => (((window as any).__handlers[name]) || []).forEach((cb: (...a: unknown[]) => void) => cb(...args));
        const base = { file: "tests/login.probe", test: "login", line: 3, description: 'tap "Sign in"', durationMs: 5, attempt: 0, depth: 0 };
        fire("run:started", "tests/login.probe");
        fire("run:step", { ...base, status: "started" });
        fire("run:step", { ...base, status: "failed", error: "element not found", triage: { fix: 'wait until "Sign in" appears', reason: "found no element" } });
        // no run:finished yet: the run is still going (its later steps are still to come)
      });
      await expect(insertButton(page)).toBeDisabled();
      await page.evaluate(() => (((window as any).__handlers["run:finished"]) || []).forEach((cb: (...a: unknown[]) => void) => cb([])));
      await expect(insertButton(page)).toBeEnabled();
      await insertButton(page).click();
      await expect(toasts(page)).toContainText("Inserted above line 3");
    } finally {
      studio.stop();
    }
  });

  test("ib-09 a run that starts while Studio is checking stops the insertion (the run events say so), and nothing is inserted", async ({ page }) => {
    const studio = await startStudio(base);
    try {
      await openLogin(page, studio);
      await fireFailedStep(page);
      // hold Studio's answer back, and start a run (the event a run started elsewhere sends) before it arrives
      await page.route("**/__headless/call", async (route) => {
        if ((route.request().postData() ?? "").includes('"method":"CheckInsertBefore"')) await new Promise((r) => setTimeout(r, 1200));
        await route.continue();
      });
      await insertButton(page).click();
      await page.evaluate(() => (((window as any).__handlers["run:started"]) || []).forEach((cb: (...a: unknown[]) => void) => cb("tests/login.probe")));
      await expect(toasts(page)).toContainText("a run started while Studio was checking");
      await expect(page.locator("#editor .view-lines")).not.toContainText("wait until");
      expect(fs.readFileSync(path.join(studio.ws, "tests", "login.probe"), "utf8")).toBe(base);
    } finally {
      studio.stop();
    }
  });
});
