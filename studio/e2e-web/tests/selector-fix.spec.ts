import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as http from "node:http";
import * as net from "node:net";
import * as path from "node:path";
import { openPath, openStudio, workspace } from "./helpers";
import { seedEmail, seedToken } from "./explain-seed";

// Suggest a fix, against a fake provider on 127.0.0.1 and the failed run (with its failed step) that Studio starts with. No hosted
// provider is called and no key is used. The failed step's error text and the model's answer are hostile on purpose.

interface Fake {
  url: string;
  requests: () => number;
  bodies: () => string[];
  close: () => Promise<void>;
}

async function fakeProvider(reply: string, delayMs = 0): Promise<Fake> {
  const bodies: string[] = [];
  const server = http.createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      res.setHeader("Content-Type", "application/json");
      if (req.method === "POST" && (req.url ?? "").endsWith("/chat/completions")) {
        bodies.push(Buffer.concat(chunks).toString("utf8"));
        setTimeout(() => res.end(JSON.stringify({ choices: [{ message: { content: reply } }] })), delayMs);
        return;
      }
      res.end(JSON.stringify({ data: [{ id: "fake-model" }] }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as net.AddressInfo).port;
  return { url: `http://127.0.0.1:${port}`, requests: () => bodies.length, bodies: () => bodies, close: () => new Promise((resolve) => server.close(() => resolve())) };
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

/** Counts what the page asks Studio to do, by method name. */
function countCalls(page: Page) {
  const calls: Record<string, number> = {};
  page.on("request", (r) => {
    const body = r.postData() ?? "";
    const m = /"method":"(\w+)"/.exec(body);
    if (m) calls[m[1]] = (calls[m[1]] ?? 0) + 1;
  });
  return calls;
}

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
      expect(f.requests()).toBe(0);
      expect(calls.PreviewSelectorFix).toBe(1);
      expect(calls.SelectorFixPreviewed ?? 0).toBe(0); // nothing asks Studio to send before Send
      await expect(page.locator("#btn-fix-preview-cancel")).toBeFocused(); // the safe choice has the focus
      await expect(page.locator("#fix-preview-overlay [role=dialog]")).toHaveAttribute("aria-modal", "true");
      await page.click("#btn-fix-preview-cancel");
      await expect(overlay(page)).toBeHidden();
      await expect(card(page)).toBeHidden();
      await expect(page.locator("#btn-fix")).toBeEnabled(); // settled: the click has been handled to its end
      expect(f.requests()).toBe(0);
      expect(calls.SelectorFixPreviewed ?? 0).toBe(0);

      await page.click("#btn-fix");
      await expect(overlay(page)).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(overlay(page)).toBeHidden();
      await expect(page.locator("#btn-fix")).toBeEnabled();
      expect(f.requests()).toBe(0);
      expect(calls.SelectorFixPreviewed ?? 0).toBe(0);
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
      await expect(card(page).locator("button")).toHaveCount(1);
      await card(page).locator("button").click();
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
        await expect(card(page).locator(".explain-place")).toHaveText("hosted");
        expect(f.requests()).toBe(i);
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
      expect(f.requests()).toBe(0);
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
      expect(f.requests()).toBe(0);
      expect(calls.SelectorFixPreviewed ?? 0).toBe(0);
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
});
