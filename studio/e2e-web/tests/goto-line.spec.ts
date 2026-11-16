import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { openStudio, workspace } from "./helpers";

// Open the failing line: a click on a failing row in Results opens the file the result names and puts the cursor on the line. Rows are
// built by firing the run events a real run emits (the handlers the page registered), so no device is needed. Only a click moves the
// editor, and a name that cannot be opened (a .., a path outside the workspace, a link out of it, a file that is gone) is refused with
// a sentence and leaves the editor as it was.

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

type Step = { file: string; test: string; line: number; description: string };

/** Fires the events of a finished run with one failed step (and, optionally, the failed test's verdict). */
async function fireFailedRun(page: Page, step: Step, opts: { verdictFile?: string; noStep?: boolean } = {}) {
  await page.evaluate(
    ([s, o]) => {
      const fire = (name: string, ...args: unknown[]) => (((window as any).__handlers[name]) || []).forEach((cb: (...a: unknown[]) => void) => cb(...args));
      const st = s as Step;
      const op = o as { verdictFile?: string; noStep?: boolean };
      fire("run:started", st.file);
      if (!op.noStep) {
        const base = { file: st.file, test: st.test, line: st.line, description: st.description, durationMs: 5, attempt: 0, depth: 0 };
        fire("run:step", { ...base, status: "started" });
        fire("run:step", { ...base, status: "failed", error: "no widget" });
      }
      fire("run:result", { name: st.test, file: op.verdictFile ?? st.file, passed: false, skipped: false, durationMs: 9, error: "no widget" });
      fire("run:finished", []);
    },
    [step, opts] as const,
  );
}

const calls = (page: Page) => {
  const seen: Record<string, number> = {};
  page.on("request", (r) => {
    const m = /"method":"(\w+)"/.exec(r.postData() ?? "");
    if (m) seen[m[1]] = (seen[m[1]] ?? 0) + 1;
  });
  return seen;
};

const activeLine = (page: Page) => page.locator("#editor .active-line-number");
const filename = (page: Page) => page.locator("#editor-filename");
const stepRow = (page: Page, text: string) => page.locator("#results-list li.step", { hasText: text });

test.describe("Open the failing line", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  const tests = path.join(workspace, "probe-tests");

  test.beforeEach(async ({ page }) => {
    await captureEvents(page);
    await openStudio(page);
    await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
  });

  async function open(page: Page, name: string) {
    await page.locator("#file-list li", { hasText: name }).first().click();
    await expect(filename(page)).toContainText(name);
  }

  test("gl-01 a click on a failing step of the open file puts the cursor on its line, and nothing moves before the click", async ({ page }) => {
    await open(page, "counter.probe");
    const seen = calls(page);
    await fireFailedRun(page, { file: path.join(tests, "counter.probe"), test: "t", line: 5, description: 'tap "x"' });
    await page.waitForTimeout(300);
    await expect(activeLine(page)).not.toHaveText("5"); // the run failed; the editor did not move by itself
    await stepRow(page, 'tap "x"').click();
    await expect(activeLine(page)).toHaveText("5");
    expect(seen.ReadFile ?? 0).toBe(0); // the same file: nothing was read again
    expect(seen.WriteFile ?? 0).toBe(0);
  });

  test("gl-02 the same click on a row of another file opens that file first, on the right line (a relative name too)", async ({ page }) => {
    await open(page, "counter.probe");
    const seen = calls(page);
    await fireFailedRun(page, { file: path.join(tests, "form.probe"), test: "t", line: 3, description: 'tap "y"' });
    await stepRow(page, 'tap "y"').click();
    await expect(filename(page)).toContainText("form.probe");
    await expect(activeLine(page)).toHaveText("3");
    expect(seen.ReadFile).toBe(1);
    expect(seen.WriteFile ?? 0).toBe(0);
    // a relative name, as a seeded run writes it
    await open(page, "counter.probe");
    await fireFailedRun(page, { file: "probe-tests/form.probe", test: "t", line: 2, description: 'tap "z"' });
    await stepRow(page, 'tap "z"').click();
    await expect(filename(page)).toContainText("form.probe");
    await expect(activeLine(page)).toHaveText("2");
  });

  test("gl-03 unsaved edits are asked about once: Cancel leaves the editor as it is, OK opens the file; nothing is written", async ({ page }) => {
    await open(page, "counter.probe");
    const seen = calls(page);
    await page.locator("#editor .view-line").first().click();
    await page.keyboard.press("End");
    await page.keyboard.type("!!");
    await expect(page.locator("#editor-dirty")).toContainText("unsaved");
    await expect(page.locator("#editor .view-lines")).toContainText("!!"); // rendered
    const before = await page.locator("#editor .view-lines").innerText();
    await fireFailedRun(page, { file: path.join(tests, "form.probe"), test: "t", line: 3, description: 'tap "q"' });
    let asked = 0;
    page.once("dialog", (d) => {
      asked++;
      void d.dismiss();
    });
    await stepRow(page, 'tap "q"').click();
    await expect.poll(() => asked).toBe(1);
    await page.waitForTimeout(300);
    await expect(filename(page)).toContainText("counter.probe");
    expect(await page.locator("#editor .view-lines").innerText()).toBe(before);
    expect(seen.ReadFile ?? 0).toBe(0);
    page.once("dialog", (d) => void d.accept());
    await stepRow(page, 'tap "q"').click();
    await expect(filename(page)).toContainText("form.probe");
    await expect(activeLine(page)).toHaveText("3");
    expect(seen.WriteFile ?? 0).toBe(0);
  });

  test("gl-04 a name with .., outside the workspace, a link out of it or a file that is gone is refused, and the editor is untouched", async ({ page }) => {
    await open(page, "counter.probe");
    const outside = fs.mkdtempSync(path.join(os.tmpdir(), "studio-gl-"));
    fs.writeFileSync(path.join(outside, "out.probe"), 'test "o"\n  tap "o"\n');
    const link = path.join(tests, "link-out.probe");
    fs.rmSync(link, { force: true });
    fs.symlinkSync(path.join(outside, "out.probe"), link);
    const gone = path.join(tests, "deleted-since-run.probe");
    const seen = calls(page);
    try {
      await expect(page.locator("#editor .view-lines")).toContainText("counter increments"); // rendered
      const before = await page.locator("#editor .view-lines").innerText();
      await page.locator("#editor .view-line").nth(2).click(); // the cursor is on line 3 and must stay there
      await expect(activeLine(page)).toHaveText("3");
      const refusals: Array<[string, string]> = [
        ["probe-tests/../../x.probe", "Not opened"], // a .. in the name
        [`${tests}/sub/../counter.probe`, "Not opened"], // a .. that comes back
        [path.join(outside, "out.probe"), "Not opened"], // an absolute path outside the workspace
        ["/etc/passwd.probe", "Not opened"],
        [link, "Cannot open"], // inside the workspace by name, out of it by link: Studio refuses it
        [gone, "Cannot open"], // a file that is gone: Studio cannot read it
      ];
      for (const [file, message] of refusals) {
        await fireFailedRun(page, { file, test: "t", line: 2, description: `tap "${path.basename(file)}"` });
        await stepRow(page, `tap "${path.basename(file)}"`).click();
        await expect(page.locator("#toast-container .toast", { hasText: message }).last()).toBeVisible();
        await expect(filename(page)).toContainText("counter.probe");
        expect(await page.locator("#editor .view-lines").innerText()).toBe(before);
        await expect(activeLine(page)).toHaveText("3"); // and the cursor did not move
        await page.evaluate(() => document.getElementById("toast-container")?.replaceChildren());
      }
      expect(seen.WriteFile ?? 0).toBe(0);
    } finally {
      fs.rmSync(link, { force: true });
      fs.rmSync(outside, { recursive: true, force: true });
    }
  });

  test("gl-05 a line past the end shows the last line, with a sentence", async ({ page }) => {
    await open(page, "counter.probe");
    await fireFailedRun(page, { file: path.join(tests, "counter.probe"), test: "t", line: 9999, description: 'tap "far"' });
    await stepRow(page, 'tap "far"').click();
    await expect(page.locator("#toast-container .toast", { hasText: "showing the last one" })).toBeVisible();
    const lastLine = fs.readFileSync(path.join(tests, "counter.probe"), "utf8").split("\n").length;
    await expect(activeLine(page)).toHaveText(String(lastLine));
  });

  test("gl-06 the failed test row opens its first failed step's line, or the file when no step is known", async ({ page }) => {
    await open(page, "counter.probe");
    await fireFailedRun(page, { file: path.join(tests, "form.probe"), test: "form fails", line: 3, description: 'tap "w"' });
    await page.locator("#results-list li.goto", { hasText: "form fails" }).click();
    await expect(filename(page)).toContainText("form.probe");
    await expect(activeLine(page)).toHaveText("3");
    // keyboard: Enter on the focused row
    await open(page, "counter.probe");
    await page.locator("#results-list li.goto", { hasText: "form fails" }).focus();
    await page.keyboard.press("Enter");
    await expect(filename(page)).toContainText("form.probe");
    // no failed step: the file opens, and no line is invented
    await open(page, "counter.probe");
    await fireFailedRun(page, { file: path.join(tests, "form.probe"), test: "no step", line: 0, description: "" }, { noStep: true });
    await page.locator("#results-list li.goto", { hasText: "no step" }).click();
    await expect(filename(page)).toContainText("form.probe");
  });

  test("gl-08 Enter on a failed test row of the open file goes to its line and does not also type a new line into it", async ({ page }) => {
    await open(page, "counter.probe");
    const before = await page.locator("#editor .view-lines").innerText();
    await fireFailedRun(page, { file: path.join(tests, "counter.probe"), test: "counter fails", line: 5, description: 'tap "x"' });
    await page.locator("#results-list li.goto", { hasText: "counter fails" }).focus();
    await page.keyboard.press("Enter");
    await expect(page.locator("#editor .active-line-number")).toHaveText("5");
    await expect(page.locator("#editor-dirty")).not.toContainText("unsaved");
    expect(await page.locator("#editor .view-lines").innerText()).toBe(before);
  });

  test("gl-07 a passing test row is not a jump", async ({ page }) => {
    await open(page, "counter.probe");
    await page.evaluate(() => {
      const fire = (name: string, ...args: unknown[]) => (((window as any).__handlers[name]) || []).forEach((cb: (...a: unknown[]) => void) => cb(...args));
      fire("run:started", "x");
      fire("run:result", { name: "all good", file: "probe-tests/form.probe", passed: true, skipped: false, durationMs: 1 });
      fire("run:finished", []);
    });
    await expect(page.locator("#results-list li.pass", { hasText: "all good" })).toBeVisible();
    await expect(page.locator("#results-list li.goto")).toHaveCount(0);
  });
});
