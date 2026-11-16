import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { openStudio, workspace } from "./helpers";

// The Problems panel and the lint markers: the project's lint rules (a recipe or data file that is not there, a test with no steps) for the
// open file, from the editor's text, saved or not. It waits 400 ms after typing, drops an answer for text that has changed, never blocks anything,
// and shows every finding as text.

const tests = () => path.join(workspace, "probe-tests");
const withProblems = 'use "recipes/missing.probe"\n\ntest "empty"\n';
const clean = 'test "t"\n  log "x"\n';

async function openFile(page: Page, name: string) {
  await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
  await page.locator("#file-list li", { hasText: name }).first().click();
  await expect(page.locator("#editor-filename")).toContainText(name);
}

const rows = (page: Page) => page.locator("#problems-list li");
const replaceText = async (page: Page, text: string) => {
  await page.locator("#editor .view-line").first().click();
  await page.keyboard.press("ControlOrMeta+a");
  await page.keyboard.insertText(text);
};

test.describe("Problems panel", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  const file = () => path.join(tests(), "lp-fixture.probe");

  test.beforeEach(() => fs.writeFileSync(file(), withProblems));
  test.afterEach(() => fs.rmSync(file(), { force: true }));

  test("lp-01 the open file's findings are rows with their lines, the tab counts them, and the editor has markers on those lines", async ({ page }) => {
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await expect(page.locator("#tab-problems")).toHaveText("Problems (2)");
    await page.click("#tab-problems");
    await expect(rows(page)).toHaveCount(2);
    await expect(rows(page).nth(0)).toContainText("line 1: use \"recipes/missing.probe\"");
    await expect(rows(page).nth(1)).toContainText('line 3: test "empty" has no steps');
    await expect(page.locator("#editor .squiggly-warning").first()).toBeVisible();
    await page.click("#tab-results");
    await expect(page.locator("#problems-panel")).toBeHidden();
  });

  test("lp-02 a click on a row goes to its line, and Enter does too", async ({ page }) => {
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await page.click("#tab-problems");
    await rows(page).nth(1).click();
    await expect(page.locator("#editor .active-line-number")).toHaveText("3");
    await rows(page).nth(0).focus();
    await page.keyboard.press("Enter");
    await expect(page.locator("#editor .active-line-number")).toHaveText("1");
    await expect(page.locator("#editor-dirty")).not.toContainText("unsaved"); // the Enter that jumped did not also type a new line
    expect(await page.locator("#editor .view-lines").innerText()).not.toContain("\n\n\n\n");
  });

  test("lp-03 it checks the text in the editor, not the file: the file on disk is unchanged", async ({ page }) => {
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await expect(page.locator("#tab-problems")).toHaveText("Problems (2)");
    await replaceText(page, clean);
    await expect(page.locator("#tab-problems")).toHaveText("Problems", { timeout: 5000 });
    await expect(page.locator("#editor .squiggly-warning")).toHaveCount(0);
    expect(fs.readFileSync(file(), "utf8")).toBe(withProblems); // nothing was saved
  });

  test("lp-04 it waits for a pause in typing: one call for a burst, 400 ms after the last key", async ({ page }) => {
    let calls = 0;
    await page.route("**/__headless/call", async (route) => {
      if ((route.request().postData() ?? "").includes('"method":"LintProblems"')) calls++;
      await route.continue();
    });
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await expect(page.locator("#tab-problems")).toHaveText("Problems (2)");
    const before = calls;
    await page.locator("#editor .view-line").first().click();
    await page.keyboard.press("End");
    for (const ch of "abcdef") await page.keyboard.type(ch, { delay: 30 }); // a burst, each key well inside 400 ms
    await page.waitForTimeout(900);
    expect(calls - before).toBe(1);
  });

  test("lp-05 an answer for old text is dropped: it paints nothing over the new text", async ({ page }) => {
    let release!: () => void;
    const held = new Promise<void>((resolve) => (release = resolve));
    let first = true;
    await page.route("**/__headless/call", async (route) => {
      if ((route.request().postData() ?? "").includes('"method":"LintProblems"') && first) {
        first = false;
        await held; // the answer for the text of the file as opened (two findings) is late
      }
      await route.continue();
    });
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await replaceText(page, clean); // the next call answers at once: no findings
    await expect(page.locator("#tab-problems")).toHaveText("Problems", { timeout: 5000 });
    release();
    await page.waitForTimeout(700);
    await expect(page.locator("#tab-problems")).toHaveText("Problems"); // the late answer did not bring its two findings back
    await expect(page.locator("#editor .squiggly-warning")).toHaveCount(0);
    await page.click("#tab-problems");
    await expect(rows(page)).toHaveCount(0);
  });

  test("lp-06 a lint that fails is one line in the panel; the editor keeps working and nothing is blocked", async ({ page }) => {
    await page.route("**/__headless/call", async (route) => {
      if ((route.request().postData() ?? "").includes('"method":"LintProblems"')) return route.abort();
      await route.continue();
    });
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await page.click("#tab-problems");
    await expect(page.locator("#problems-status")).toContainText("Lint unavailable");
    await replaceText(page, clean);
    await expect(page.locator("#editor .view-lines")).toContainText('log "x"'); // still editable
    await expect(page.locator("#editor-dirty")).toContainText("unsaved");
  });

  test("lp-07 text that does not parse yet says it was not checked, in one line", async ({ page }) => {
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await replaceText(page, 'test "t"\n  tap "unterminated\n');
    await page.click("#tab-problems");
    await expect(page.locator("#problems-status")).toContainText("Not checked:", { timeout: 5000 });
    await expect(rows(page)).toHaveCount(0);
  });

  test("lp-09 the three tabs are a tablist: each tab controls its panel, only the shown one is selected, and each panel is labelled by its tab", async ({ page }) => {
    await openStudio(page);
    const tabs = [["tab-results", "results-panel"], ["tab-network", "network-panel"], ["tab-problems", "problems-panel"]] as const;
    await expect(page.locator(".pane-tabs")).toHaveAttribute("role", "tablist");
    for (const [tab, panel] of tabs) {
      await expect(page.locator(`#${tab}`)).toHaveAttribute("aria-controls", panel);
      await expect(page.locator(`#${panel}`)).toHaveAttribute("role", "tabpanel");
      await expect(page.locator(`#${panel}`)).toHaveAttribute("aria-labelledby", tab);
    }
    for (const [shown] of tabs) {
      await page.click(`#${shown}`);
      for (const [tab, panel] of tabs) {
        await expect(page.locator(`#${tab}`)).toHaveAttribute("aria-selected", String(tab === shown));
        if (tab === shown) await expect(page.locator(`#${panel}`)).toBeVisible();
        else await expect(page.locator(`#${panel}`)).toBeHidden();
      }
    }
  });

  test("lp-10 an answer marked stale (a newer request replaced it) is not drawn: no rows, no error marker", async ({ page }) => {
    let calls = 0;
    await page.route("**/__headless/call", async (route) => {
      if ((route.request().postData() ?? "").includes('"method":"LintProblems"')) {
        calls++;
        return route.fulfill({ json: { result: { problems: [{ line: 1, warning: false, message: "SHOULD NOT BE DRAWN", source: "reference" }], stale: true } } });
      }
      await route.continue();
    });
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await expect.poll(() => calls).toBeGreaterThan(0);
    await page.waitForTimeout(500);
    await expect(page.locator("#tab-problems")).toHaveText("Problems");
    await page.click("#tab-problems");
    await expect(rows(page)).toHaveCount(0);
    await expect(page.locator("#editor .squiggly-error")).toHaveCount(0); // the stale finding was an error; the old parse lint only warns
  });

  test("lp-11 a ${NAME} that Studio's environment does not have is a warning with the name and the way out; a set one and a comment say nothing", async ({ page }) => {
    fs.writeFileSync(file(), 'test "t"\n  type "${LP_SURELY_UNSET_VARIABLE_42}" into the "email" field\n  type "${HOME}" into the "home" field\n  # tap "${LP_IN_A_COMMENT}"\n  log "x"\n');
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await expect(page.locator("#tab-problems")).toHaveText("Problems (1)");
    await page.click("#tab-problems");
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("line 2: ${LP_SURELY_UNSET_VARIABLE_42} is not set in Studio's environment");
    await expect(rows(page).first()).toContainText("start Studio from a terminal where LP_SURELY_UNSET_VARIABLE_42 is set");
    await expect(rows(page).first()).not.toContainText("LP_IN_A_COMMENT");
    await expect(rows(page).first()).toContainText("warning");
    await expect(page.locator("#editor .squiggly-warning").first()).toBeVisible();
  });

  test("lp-08 findings are text: a recipe name that looks like markup is not interpreted", async ({ page }) => {
    fs.writeFileSync(file(), 'use "recipes/<img src=x onerror=alert(1)>.probe"\n\ntest "t"\n  log "x"\n');
    await openStudio(page);
    await openFile(page, "lp-fixture.probe");
    await page.click("#tab-problems");
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("<img src=x");
    expect(await page.locator("#problems-list img").count()).toBe(0);
    expect(await rows(page).first().evaluate((el) => el.querySelectorAll("*").length)).toBe(2); // the kind and the text spans only
  });
});
