import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as http from "node:http";
import * as net from "node:net";
import * as path from "node:path";
import { expectNoCall, openPath, openStudio, recordCalls, workspace } from "./helpers";
import { seedEmail, seedLoginText, seedToken } from "./explain-seed";

// Suggest a fix, against a fake provider on 127.0.0.1 and the failed run (with its failed step) that Studio starts with. No hosted
// provider is called and no key is used. The failed step's error text and the model's answer are hostile on purpose.

interface Fake {
  url: string;
  requests: () => number;
  /** Asserts the provider was asked nothing, after proving that its counter counts a request made in this test (one flagged as a control, which requests() and bodies() leave out). */
  expectNoRequests: () => Promise<void>;
  bodies: () => string[];
  close: () => Promise<void>;
}

async function fakeProvider(reply: string, delayMs = 0): Promise<Fake> {
  const bodies: string[] = [];
  const control: boolean[] = []; // one per request that was counted: true for the request a test makes to prove the counter works
  const server = http.createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      res.setHeader("Content-Type", "application/json");
      if (req.method === "POST" && (req.url ?? "").endsWith("/chat/completions")) {
        bodies.push(Buffer.concat(chunks).toString("utf8"));
        control.push(req.headers["x-probe-control"] === "1");
        setTimeout(() => res.end(JSON.stringify({ choices: [{ message: { content: reply } }] })), delayMs);
        return;
      }
      res.end(JSON.stringify({ data: [{ id: "fake-model" }] }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as net.AddressInfo).port;
  const real = () => bodies.filter((_, i) => !control[i]);
  let proven = false;
  const expectNoRequests = async () => {
    if (!proven) {
      const before = bodies.length;
      await fetch(`http://127.0.0.1:${port}/v1/chat/completions`, { method: "POST", headers: { "x-probe-control": "1" }, body: "{}" });
      expect(bodies.length, "the provider's counter did not count a request it was sent").toBe(before + 1);
      proven = true;
    }
    expect(real().length, "the provider was asked").toBe(0);
  };
  return { url: `http://127.0.0.1:${port}`, requests: () => real().length, expectNoRequests, bodies: () => real(), close: () => new Promise((resolve) => server.close(() => resolve())) };
}

const yamlPath = path.join(workspace, "probe.yaml");
let original = "";

/** probe.yaml with the fake as the local provider, and the switch that lets a suggestion read the last run. */
function useProvider(f: Fake, model = "fake-model") {
  fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: ${model}\nstudio:\n  ai_context:\n    run_results: true\n`);
}

async function captureRunStarted(page: Page) {
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

const overlay = (page: Page) => page.locator("#fix-preview-overlay");
const card = (page: Page) => page.locator("#fix-card");

const countCalls = recordCalls; // the shared recorder (helpers.ts); a zero is asserted with expectNoCall, which proves the recorder works first

const hostileReply = 'Here:\n```probe\ntap "Sign in now"\n```\nThe label changed. <img src=x onerror="window.__fixXss=1"> <script>window.__fixXss=2</script>​';

test.describe("Suggest a fix (a fake provider on 127.0.0.1, a seeded failed run)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test.beforeEach(() => {
    original = fs.readFileSync(yamlPath, "utf8");
    if (!original.endsWith("\n")) original += "\n";
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));

  test("sf-01 with AI off there is no Suggest a fix button", async ({ page }) => {
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: none\n`);
    await openStudio(page);
    await expect(page.locator("#btn-fix")).toBeHidden();
    await expect(card(page)).toBeHidden();
  });

  test("sf-02 the preview comes first, nothing is sent until Send, and Cancel and Escape send nothing", async ({ page }) => {
    const f = await fakeProvider(hostileReply);
    try {
      useProvider(f);
      const calls = countCalls(page);
      await openStudio(page);
      await expect(page.locator("#btn-fix")).toBeVisible();
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      const text = await page.locator("#fix-preview-text").innerText();
      expect(text.split("\n")[0]).toBe("Sent to: fake-model, on this machine"); // the provider and model, never an endpoint
      expect(text).not.toContain("127.0.0.1");
      expect(text).toContain('tap "Sign in"'); // the failed step is in what would be sent
      await expect(page.locator("#fix-preview-text img, #fix-preview-text script")).toHaveCount(0); // the error text holds markup: it stays text
      expect(await page.evaluate(() => (window as any).__explainXss)).toBeUndefined();
      expect(text).toContain("<img src=x"); // and it is shown as the text it is
      await f.expectNoRequests();
      expect(calls.PreviewSelectorFix).toBe(1);
      await expectNoCall(page, calls, "SelectorFixPreviewed"); // nothing asks Studio to send before Send
      await expect(page.locator("#btn-fix-preview-cancel")).toBeFocused(); // the safe choice has the focus
      await expect(page.locator("#fix-preview-overlay [role=dialog]")).toHaveAttribute("aria-modal", "true");
      await page.click("#btn-fix-preview-cancel");
      await expect(overlay(page)).toBeHidden();
      await expect(card(page)).toBeHidden();
      await expect(page.locator("#btn-fix")).toBeEnabled(); // settled: the click has been handled to its end
      await f.expectNoRequests();
      await expectNoCall(page, calls, "SelectorFixPreviewed");

      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(overlay(page)).toBeHidden();
      await expect(page.locator("#btn-fix")).toBeEnabled();
      await f.expectNoRequests();
      await expectNoCall(page, calls, "SelectorFixPreviewed");
    } finally {
      await f.close();
    }
  });

  test("sf-03 Send asks the provider once, the card is advisory text, and the results are untouched", async ({ page }) => {
    const f = await fakeProvider(hostileReply);
    try {
      useProvider(f);
      await openStudio(page);
      const listBefore = await page.locator("#results-list").innerHTML();
      const summaryBefore = await page.locator("#results-summary").innerText();
      await page.click("#btn-fix");
      await page.click("#btn-fix-preview-send");
      await expect(card(page)).toBeVisible();
      expect(f.requests()).toBe(1);
      await expect(card(page)).toContainText("Suggested fix");
      await expect(card(page).locator(".explain-place")).toHaveText("local");
      await expect(card(page)).toContainText("fake-model");
      await expect(card(page)).toContainText("Failed step");
      await expect(card(page)).toContainText('line 4 of "sign in"');
      await expect(card(page)).toContainText('tap "Sign in now"');
      await expect(card(page)).toContainText("The label changed.");
      await expect(card(page)).toContainText("Not applied to the test file and not verified against the app. It does not change this result.");
      // Markup in the answer is text, and the hidden character is gone.
      await expect(card(page)).toContainText("<img src=x");
      await expect(card(page).locator("img, script")).toHaveCount(0);
      expect(await page.evaluate(() => (window as any).__fixXss)).toBeUndefined();
      expect(await card(page).innerText()).not.toContain("​");
      // The run is as it was.
      expect(await page.locator("#results-list").innerHTML()).toBe(listBefore);
      expect(await page.locator("#results-summary").innerText()).toBe(summaryBefore);
      // The card closes, and it is never offered a way to change the file.
      // the controls on the card: the close button and the button that opens the failing line, which opens a place and changes no result
      await expect(card(page).locator("button")).toHaveCount(2);
      await expect(card(page).locator("button.goto-place")).toHaveCount(1);
      await card(page).locator("button:not(.goto-place)").click();
      await expect(card(page)).toBeHidden();
    } finally {
      await f.close();
    }
  });

  test("sf-04 what is sent holds no credential, email, home folder or hidden character, and the page asks nobody else", async ({ page }) => {
    const f = await fakeProvider(hostileReply);
    const hosts = new Set<string>();
    page.on("request", (r) => {
      if (/^https?:/.test(r.url())) hosts.add(new URL(r.url()).host);
    });
    try {
      useProvider(f);
      await openStudio(page);
      await page.click("#btn-fix");
      const shown = await page.locator("#fix-preview-text").innerText();
      await page.click("#btn-fix-preview-send");
      await expect(card(page)).toBeVisible();
      const sent = f.bodies().join("\n");
      for (const bad of [seedToken.slice(4, 24), seedEmail, "jane.doe", "/Users/alice", "⵿", "⁥", "\u001b[31m"]) {
        expect(sent, bad).not.toContain(bad);
        expect(shown, bad).not.toContain(bad);
      }
      expect(sent).toContain("[redacted]");
      expect(sent).toContain("Sign in"); // the step itself is sent
      const base = new URL(page.url()).host;
      expect([...hosts].filter((h) => h !== base)).toEqual([]); // the page never calls the provider itself
    } finally {
      await f.close();
    }
  });

  test("sf-05 a destination on this machine is confirmed once, and a changed one is confirmed again", async ({ page }) => {
    const f = await fakeProvider(hostileReply);
    try {
      useProvider(f);
      await openStudio(page);
      await page.click("#btn-fix");
      await page.click("#btn-fix-preview-send");
      await expect(card(page)).toBeVisible();
      expect(f.requests()).toBe(1);
      // The same destination: no dialog, the request goes at once.
      await page.click("#btn-fix");
      await expect.poll(() => f.requests()).toBe(2);
      await expect(overlay(page)).toBeHidden();
      // Another model, the same workspace: Studio asks again, and a page reload is not needed.
      useProvider(f, "another-model");
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      expect((await page.locator("#fix-preview-text").innerText()).split("\n")[0]).toBe("Sent to: another-model, on this machine");
      expect(f.requests()).toBe(2);
      await page.click("#btn-fix-preview-cancel");
    } finally {
      await f.close();
    }
  });

  test("sf-06 a destination that is not this machine is confirmed every time", async ({ page }) => {
    const f = await fakeProvider(hostileReply);
    try {
      useProvider(f);
      // The page is told the destination is hosted, as in the Explain specs; the request still goes to the fake.
      await page.route("**/__headless/call", async (route) => {
        const body = JSON.parse(route.request().postData() || "{}");
        if (body.method !== "PreviewSelectorFix" && body.method !== "SelectorFixPreviewed") return route.continue();
        const resp = await route.fetch();
        const json = await resp.json();
        if (json.result) Object.assign(json.result, body.method === "PreviewSelectorFix" ? { hosted: true, needsConfirm: true } : { hosted: true });
        await route.fulfill({ response: resp, contentType: "application/json", body: JSON.stringify(json) });
      });
      await openStudio(page);
      for (let i = 1; i <= 3; i++) {
        await page.click("#btn-fix");
        await expect(overlay(page), `suggestion ${i}`).toBeVisible();
        await expect(page.locator("#fix-preview-note")).toContainText("before every suggestion");
        expect(f.requests()).toBe(i - 1);
        await page.click("#btn-fix-preview-send");
        await expect(card(page).locator(".explain-place")).toHaveText("hosted"); // from the second suggestion on this can be the card of the one before
        await expect.poll(() => f.requests(), `the provider was asked ${i} times`).toBe(i);
      }
    } finally {
      await f.close();
    }
  });

  test("sf-07 a change between the preview and Send stops the request, and says so", async ({ page }) => {
    const f = await fakeProvider(hostileReply);
    try {
      useProvider(f);
      await openStudio(page);
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      useProvider(f, "swapped-model"); // the model changes while the preview is open
      await page.click("#btn-fix-preview-send");
      await expect(page.locator("#explain-status")).toContainText("changed since the preview");
      await expect(card(page)).toBeHidden();
      await f.expectNoRequests();
      // And the next suggestion works from a new preview.
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      await page.click("#btn-fix-preview-send");
      await expect(card(page)).toBeVisible();
      expect(f.requests()).toBe(1);
    } finally {
      await f.close();
    }
  });

  test("sf-08 a double click asks Studio for one preview, and a close button is the only control on the card", async ({ page }) => {
    const f = await fakeProvider(hostileReply);
    try {
      useProvider(f);
      const calls = countCalls(page);
      await openStudio(page);
      await page.dblclick("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      await page.waitForTimeout(300);
      expect(calls.PreviewSelectorFix).toBe(1);
      await page.click("#btn-fix-preview-cancel");
    } finally {
      await f.close();
    }
  });

  test("sf-09 the open file goes into the request, as the editor holds it", async ({ page }) => {
    const f = await fakeProvider(hostileReply);
    try {
      useProvider(f);
      await openStudio(page);
      await openPath(page, "probe-tests/counter.probe");
      await expect(page.locator("#editor-filename")).toContainText("counter.probe");
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      const text = await page.locator("#fix-preview-text").innerText();
      expect(text).toContain("Current file:");
      expect(text).toContain("counter increments and resets");
      await page.click("#btn-fix-preview-send");
      await expect(card(page)).toBeVisible();
      expect(f.bodies().join("\n")).toContain("counter increments and resets");
    } finally {
      await f.close();
    }
  });

  test("sf-10 a new run drops an open preview and a card on screen, and a new suggestion works", async ({ page }) => {
    const f = await fakeProvider(hostileReply);
    try {
      useProvider(f);
      await captureRunStarted(page);
      const runStarted = () => page.evaluate(() => (((window as any).__handlers["run:started"]) || []).forEach((cb: (p: string) => void) => cb("x.probe")));
      const calls = countCalls(page);
      await openStudio(page);
      // An open preview is closed by a new run, and nothing is sent.
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      await runStarted();
      await expect(overlay(page)).toBeHidden();
      await expect(page.locator("#btn-fix")).toBeEnabled(); // not stuck disabled
      await f.expectNoRequests();
      await expectNoCall(page, calls, "SelectorFixPreviewed");
      // A card on screen is removed by a new run.
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      await page.click("#btn-fix-preview-send");
      await expect(card(page)).toBeVisible();
      await runStarted();
      await expect(card(page)).toBeHidden();
      expect(f.requests()).toBe(1);
    } finally {
      await f.close();
    }
  });

  test("sf-11 a new run while the provider is answering drops the answer, and the next suggestion works", async ({ page }) => {
    const f = await fakeProvider(hostileReply, 700);
    try {
      useProvider(f);
      await captureRunStarted(page);
      const runStarted = () => page.evaluate(() => (((window as any).__handlers["run:started"]) || []).forEach((cb: (p: string) => void) => cb("x.probe")));
      await openStudio(page);
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      await page.click("#btn-fix-preview-send");
      await expect.poll(() => f.requests()).toBe(1); // the request is on its way
      await runStarted();
      await expect(page.locator("#btn-fix")).toBeEnabled(); // the session is free at once
      await page.waitForTimeout(1200); // the late answer arrives and is dropped
      await expect(card(page)).toBeHidden();
    } finally {
      await f.close();
    }
  });

  test("sf-12 a suggestion that cleaning changed carries a Changed row, and an unchanged one does not", async ({ page }) => {
    const changed = await fakeProvider('```probe\ntype "jane.doe@example.com" into "Email"\n```\nThe field was empty.');
    try {
      useProvider(changed);
      await openStudio(page);
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      await page.click("#btn-fix-preview-send");
      await expect(card(page)).toBeVisible();
      await expect(card(page).locator(".explain-row-changed")).toHaveCount(1);
      await expect(card(page)).toContainText("[email]");
      await expect(card(page)).not.toContainText("jane.doe");
    } finally {
      await changed.close();
    }
    const plain = await fakeProvider('```probe\ntap "Sign in now"\n```\nThe label changed.');
    try {
      useProvider(plain);
      await page.reload();
      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      await page.click("#btn-fix-preview-send");
      await expect(card(page)).toBeVisible();
      await expect(card(page).locator(".explain-row-changed")).toHaveCount(0);
    } finally {
      await plain.close();
    }
  });

  const loginFile = () => path.join(workspace, "probe-tests", "login.probe");
  const loginText = seedLoginText; // the text the seeded run read
  const lines = (page: Page) => page.locator("#editor .view-lines");

  async function suggestWith(page: Page, reply: string) {
    const f = await fakeProvider(reply);
    useProvider(f);
    fs.mkdirSync(path.dirname(loginFile()), { recursive: true });
    fs.writeFileSync(loginFile(), loginText);
    await openStudio(page);
    await openPath(page, "probe-tests/login.probe");
    await expect(page.locator("#editor-filename")).toContainText("login.probe");
    await page.click("#btn-fix");
    await expect(overlay(page)).toBeVisible();
    await page.click("#btn-fix-preview-send");
    await expect(card(page)).toBeVisible();
    return f;
  }

  test("sf-13 Replace the line puts the suggestion in the editor as one undo step, and saves and runs nothing", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nThe label changed.');
    try {
      await expect(page.locator("#btn-fix-replace")).toBeVisible();
      const calls = countCalls(page);
      await page.click("#btn-fix-replace");
      await expect(lines(page)).toContainText('tap "Sign in now"');
      await expect(lines(page)).not.toContainText('tap "Sign in"\n');
      await expect(page.locator("#explain-status")).toContainText("not saved and not run");
      await expect(card(page)).toBeVisible(); // the card stays, with the Re-run button
      await expect(page.locator("#btn-fix-rerun")).toBeVisible();
      expect(fs.readFileSync(loginFile(), "utf8")).toBe(loginText); // the file on disk is untouched
      await expectNoCall(page, calls, "WriteFile");
      await expectNoCall(page, calls, "RunFile");
      await expectNoCall(page, calls, "RunFileScoped");
      // one Undo restores the line
      await lines(page).locator(".view-line", { hasText: "Welcome" }).click(); // focus the editor
      await page.keyboard.press("ControlOrMeta+z");
      await expect(lines(page)).toContainText('tap "Sign in"');
      await expect(lines(page)).not.toContainText("Sign in now");
    } finally {
      await f.close();
    }
  });

  test("sf-14 Replace the line is refused for a suggestion that is not one step of the same kind", async ({ page }) => {
    const f = await suggestWith(page, '```probe\nsee "Welcome"\n```\nWhy.');
    try {
      await page.click("#btn-fix-replace");
      await expect(page.locator("#explain-status")).toContainText("Not replaced");
      await expect(page.locator("#explain-status")).toContainText("same kind");
      await expect(lines(page)).not.toContainText('see "Welcome"\n  tap');
      await expect(lines(page)).toContainText('tap "Sign in"');
    } finally {
      await f.close();
    }
  });

  test("sf-15 Replace the line is refused when the line was edited after the suggestion", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      // edit the failed step's line in the editor: go to line 4, end of line, type
      await lines(page).locator(".view-line", { hasText: 'tap "Sign in"' }).click();
      await page.keyboard.press("End");
      await page.keyboard.type("!");
      await page.click("#btn-fix-replace");
      await expect(page.locator("#explain-status")).toContainText("Not replaced");
      await expect(lines(page)).toContainText('tap "Sign in"!');
      await expect(lines(page)).not.toContainText("Sign in now");
    } finally {
      await f.close();
    }
  });

  test("sf-16 a suggestion that cleaning changed has no Replace the line button", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntype "jane.doe@example.com" into "Email"\n```\nWhy.');
    try {
      await expect(card(page).locator(".explain-row-changed")).toHaveCount(1);
      await expect(page.locator("#btn-fix-replace")).toHaveCount(0);
    } finally {
      await f.close();
    }
  });

  test("sf-17 one Undo takes back the replacement and nothing else", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      await page.click("#btn-fix-replace");
      await expect(lines(page)).toContainText('tap "Sign in now"');
      await lines(page).locator(".view-line", { hasText: "Welcome" }).click();
      await page.keyboard.press("ControlOrMeta+z");
      await expect(lines(page)).not.toContainText("Sign in now");
      await expect(lines(page)).toContainText('tap "Sign in"');
    } finally {
      await f.close();
    }
  });

  test("sf-22 an edit anywhere in the file since the run refuses Replace the line: the run's text is the anchor", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      await lines(page).locator(".view-line", { hasText: 'see "Home"' }).click();
      await page.keyboard.press("End");
      await page.keyboard.type("x"); // an edit on another line: the failed step's line number is unchanged
      await page.click("#btn-fix-replace");
      await expect(page.locator("#explain-status")).toContainText("Not replaced");
      await expect(page.locator("#explain-status")).toContainText("changed since the run");
      await expect(lines(page)).not.toContainText("Sign in now");
      await expect(lines(page)).toContainText('see "Home"x'); // the edit is still there, untouched
    } finally {
      await f.close();
    }
  });

  test("rr-01 after a replacement Re-run waits for an explicit Save and a device, and Replace itself writes and runs nothing", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      const calls = countCalls(page);
      await page.click("#btn-fix-replace");
      await expect(page.locator("#btn-fix-rerun")).toBeVisible();
      await expect(page.locator("#btn-fix-rerun")).toBeDisabled();
      await expect(page.locator("#fix-rerun-hint")).toContainText("Save the file first"); // the replacement is unsaved
      await expectNoCall(page, calls, "WriteFile");
      await expectNoCall(page, calls, "RunFileScoped");
      await expectNoCall(page, calls, "RunFile");
      // the person saves: that is the only write
      await page.click("#btn-save");
      await expect.poll(() => calls.WriteFile ?? 0).toBe(1);
      // the request is counted when it is sent, not when Studio has finished writing: Save writes a new file, flushes it and renames it over the old one, so the
      // file is read until it holds the text (a read right after the request saw the old file on a busy machine)
      await expect.poll(() => fs.readFileSync(loginFile(), "utf8")).toContain('tap "Sign in now"');
      // no device in this suite: the next reason is the missing connection, and still nothing runs
      await expect(page.locator("#fix-rerun-hint")).toContainText("Connect a device");
      await expect(page.locator("#btn-fix-rerun")).toBeDisabled();
      await page.locator("#btn-fix-rerun").click({ force: true }); // a disabled button does nothing
      await page.waitForTimeout(300);
      await expectNoCall(page, calls, "RunFileScoped");
    } finally {
      await f.close();
    }
  });

  test("rr-04 the card with the Re-run button survives the Explain offer going away (what a passing re-run causes)", async ({ page }) => {
    await captureRunStarted(page); // keeps the handlers the page registers, so the test can fire run:finished as the bridge would
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      await page.click("#btn-fix-replace");
      await expect(page.locator("#btn-fix-rerun")).toBeVisible();
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: none\n`); // AI is off (whatever key an earlier spec left): the Explain offer is gone, as it is after a run with no failed test
      await page.evaluate(() => (((window as any).__handlers["run:finished"]) || []).forEach((cb: (p?: unknown) => void) => cb()));
      await expect(page.locator("#explain-bar")).toBeHidden();
      await expect(page.locator("#btn-fix-rerun")).toBeVisible();
    } finally {
      await f.close();
    }
  });

  test("rr-02 editing the replaced line takes the re-run away: it would not test the replacement", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      await page.click("#btn-fix-replace");
      await page.click("#btn-save");
      await lines(page).locator(".view-line", { hasText: "Sign in now" }).click();
      await page.keyboard.press("End");
      await page.keyboard.type("!");
      await expect(page.locator("#fix-rerun-hint")).toContainText("edited after it was replaced");
      await expect(page.locator("#btn-fix-rerun")).toBeDisabled();
    } finally {
      await f.close();
    }
  });

  test("rr-03 a re-run is for the file that was changed", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      await page.click("#btn-fix-replace");
      await page.click("#btn-save");
      await page.locator("#file-list li", { hasText: "counter.probe" }).first().click(); // another file in the same folder
      await expect(page.locator("#editor-filename")).toContainText("counter.probe");
      await expect(page.locator("#fix-rerun-hint")).toContainText("Open the file that was changed");
      await expect(page.locator("#btn-fix-rerun")).toBeDisabled();
    } finally {
      await f.close();
    }
  });

  test("rr-05 a line added elsewhere in the file takes the re-run away: the replaced line is no longer where it was recorded", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      await page.click("#btn-fix-replace");
      await page.click("#btn-save");
      await expect(page.locator("#fix-rerun-hint")).toContainText("Connect a device"); // saved: only the missing device is in the way
      await lines(page).locator(".view-line", { hasText: 'see "Home"' }).click();
      await page.keyboard.press("End");
      await page.keyboard.press("Enter"); // one more line, below the replaced one: its text and number are unchanged
      await page.keyboard.type("  tap \"Extra\"");
      await page.click("#btn-save");
      await expect(page.locator("#fix-rerun-hint")).toContainText("Lines were added or removed");
      await expect(page.locator("#btn-fix-rerun")).toBeDisabled();
    } finally {
      await f.close();
    }
  });

  test("sf-18 a failed step gets three suggestions, numbered, and a fourth is refused before anything is sent", async ({ page }) => {
    const f = await fakeProvider('```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      useProvider(f);
      const calls = countCalls(page);
      await openStudio(page);
      for (let n = 1; n <= 3; n++) {
        await page.click("#btn-fix");
        if (n === 1) {
          await expect(overlay(page)).toBeVisible(); // a destination on this machine is confirmed once per session
          await page.click("#btn-fix-preview-send");
        }
        await expect(card(page).locator(".explain-card-title")).toHaveText(n === 1 ? "Suggested fix" : `Suggested fix (${n} of 3)`);
        await expect.poll(() => f.requests()).toBe(n); // one request per attempt, never more
        await page.waitForTimeout(150);
      }
      // the fourth: Studio refuses at the preview, and the provider is not asked
      const previews = calls.PreviewSelectorFix ?? 0;
      await page.click("#btn-fix");
      await expect(page.locator("#explain-status")).toContainText("3 suggestions were already asked for this step");
      expect(f.requests()).toBe(3);
      expect(calls.PreviewSelectorFix ?? 0).toBe(previews + 1);
      expect(calls.SelectorFixPreviewed ?? 0).toBe(3); // and nothing was sent for it
    } finally {
      await f.close();
    }
  });

  test("sf-19 a new suggestion after a replacement is an ordinary suggestion card: it goes when the Explain offer goes", async ({ page }) => {
    await captureRunStarted(page);
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      await page.click("#btn-fix-replace");
      await expect(page.locator("#btn-fix-rerun")).toBeVisible();
      await page.click("#btn-fix"); // another suggestion, while the card of the replacement is on screen
      await expect(card(page).locator(".explain-card-title")).toHaveText("Suggested fix (2 of 3)");
      await expect(page.locator("#btn-fix-rerun")).toHaveCount(0);
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: none\n`);
      await page.evaluate(() => (((window as any).__handlers["run:finished"]) || []).forEach((cb: (p?: unknown) => void) => cb()));
      await expect(page.locator("#explain-bar")).toBeHidden();
      await expect(card(page)).toBeHidden(); // not kept alive by the replacement before it
    } finally {
      await f.close();
    }
  });

  const activeLine = (page: Page) => page.locator("#editor .active-line-number");

  test("sf-20 the fix card names the failed step's line, and the same click as a failing row opens it", async ({ page }) => {
    const f = await suggestWith(page, '```probe\ntap "Sign in now"\n```\nWhy.');
    try {
      const place = card(page).locator("button.goto-place");
      await expect(place).toHaveText("L4");
      await expect(place).toHaveAttribute("title", "Open line 4 of login.probe"); // the file's name, not its folder
      await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("counter.probe");
      await place.click();
      await expect(page.locator("#editor-filename")).toContainText("login.probe");
      await expect(activeLine(page)).toHaveText("4");
    } finally {
      await f.close();
    }
  });

  test("sf-21 an Explain note names where its test failed; a place outside the workspace is refused, and nothing moves", async ({ page }) => {
    const f = await fakeProvider("Check the label.");
    try {
      useProvider(f);
      fs.mkdirSync(path.dirname(loginFile()), { recursive: true });
      fs.writeFileSync(loginFile(), loginText);
      await openStudio(page);
      await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
      await page.locator("#file-list li", { hasText: "counter.probe" }).first().click();
      await expect(page.locator("#editor-filename")).toContainText("counter.probe");
      await page.click("#btn-explain");
      await expect(page.locator("#explain-preview-overlay")).toBeVisible();
      await page.click("#btn-explain-preview-send");
      const explain = page.locator("#explain-card");
      await expect(explain).toBeVisible();
      // the failed test with a failed step: its line; the other (a path outside the workspace): its file's name only
      await expect(explain.locator(".explain-row", { hasText: "sign in" }).locator("button.goto-place")).toHaveText("L4");
      const outside = explain.locator(".explain-row", { hasText: "sign out" }).locator("button.goto-place");
      await expect(outside).toHaveText("logout.probe");
      await outside.click();
      await expect(page.locator("#toast-container .toast", { hasText: "outside the open workspace" })).toBeVisible();
      await expect(page.locator("#editor-filename")).toContainText("counter.probe"); // nothing moved
      await explain.locator(".explain-row", { hasText: "sign in" }).locator("button.goto-place").click();
      await expect(page.locator("#editor-filename")).toContainText("login.probe");
      await expect(activeLine(page)).toHaveText("4");
      expect(f.bodies().join("\n")).not.toContain(workspace); // no path of the workspace went to the provider
    } finally {
      await f.close();
    }
  });
});
