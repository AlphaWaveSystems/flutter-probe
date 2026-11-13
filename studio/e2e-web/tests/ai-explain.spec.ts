import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as http from "node:http";
import * as net from "node:net";
import * as path from "node:path";
import { openStudio, workspace } from "./helpers";
import { seedEmail, seedFailed, seedToken } from "./explain-seed";

// Explain, against fake providers on 127.0.0.1 and the failed run Studio starts with (PROBE_STUDIO_SEED_RUN). No hosted provider is
// called and no key is used. The seeded error text is hostile on purpose.

interface Fake {
  url: string;
  chats: () => number;
  bodies: () => string[];
  close: () => Promise<void>;
}

/** A fake OpenAI-compatible server: it records every chat request body and answers each with status and reply. */
async function fakeProvider(reply: string, status = 200): Promise<Fake> {
  const bodies: string[] = [];
  const server = http.createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      res.setHeader("Content-Type", "application/json");
      if (req.method === "POST" && (req.url ?? "").endsWith("/chat/completions")) {
        bodies.push(Buffer.concat(chunks).toString("utf8"));
        res.statusCode = status;
        res.end(JSON.stringify({ choices: [{ message: { content: reply } }] }));
        return;
      }
      res.end(JSON.stringify({ data: [{ id: "fake-model" }] }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as net.AddressInfo).port;
  return { url: `http://127.0.0.1:${port}`, chats: () => bodies.length, bodies: () => bodies, close: () => new Promise((resolve) => server.close(() => resolve())) };
}

const yamlPath = path.join(workspace, "probe.yaml");
let original = "";

function useProvider(f: Fake, model = "fake-model") {
  fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: ${model}\n`);
}

async function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address() as net.AddressInfo;
      server.close(() => resolve(port));
    });
  });
}

/** Rewrites the destination in Explain's answers, so the page believes it is a hosted one while the request still goes to the fake. */
async function pretendHosted(page: Page) {
  await page.route("**/__headless/call", async (route) => {
    const body = JSON.parse(route.request().postData() || "{}");
    if (body.method !== "PreviewExplain" && body.method !== "ExplainPreviewed") return route.continue();
    const resp = await route.fetch();
    const json = await resp.json();
    if (json.result) json.result.hosted = true;
    await route.fulfill({ response: resp, contentType: "application/json", body: JSON.stringify(json) });
  });
}

const overlay = (page: Page) => page.locator("#explain-preview-overlay");
const card = (page: Page) => page.locator("#explain-card");

test.describe("Explain (fake providers on 127.0.0.1, a seeded run)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test.beforeEach(() => {
    original = fs.readFileSync(yamlPath, "utf8"); // the fixture has no ai: block
    if (!original.endsWith("\n")) original += "\n";
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));

  test("ex-01 with AI off there is no Explain button, though the run has failed tests", async ({ page }) => {
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: none\n`);
    await openStudio(page);
    await expect(page.locator("#btn-explain")).toBeHidden();
    await expect(page.locator("#explain-bar")).toBeHidden();
    await expect(card(page)).toBeHidden();
  });

  test("ex-02 with AI on the preview comes first, nothing is sent until Send, and the card is advisory", async ({ page }) => {
    const f = await fakeProvider("Probably the label changed.");
    try {
      useProvider(f);
      await openStudio(page);
      await expect(page.locator("#btn-explain")).toBeVisible();
      const listBefore = await page.locator("#results-list").innerHTML();
      const summaryBefore = await page.locator("#results-summary").innerText();

      await page.click("#btn-explain");
      await expect(overlay(page)).toBeVisible();
      const text = await page.locator("#explain-preview-text").innerText();
      expect(text.split("\n")[0]).toBe("Sent to: fake-model, on this machine"); // the provider and model, never an endpoint
      expect(text).not.toContain("127.0.0.1");
      expect(f.chats()).toBe(0);

      await page.click("#btn-explain-preview-cancel");
      await expect(overlay(page)).toBeHidden();
      await expect(card(page)).toBeHidden();
      expect(f.chats()).toBe(0);

      await page.click("#btn-explain");
      await expect(overlay(page)).toBeVisible();
      expect(f.chats()).toBe(0);
      await page.click("#btn-explain-preview-send");
      await expect(card(page)).toBeVisible();
      await expect(card(page).locator(".explain-card-title")).toHaveText("Advisory");
      await expect(card(page).locator(".explain-card-dest")).toHaveText("fake-model, on this machine");
      await expect(card(page).locator(".explain-place")).toHaveText("local");
      await expect(card(page)).toContainText("Probably the label changed.");
      await expect(card(page)).toContainText("It does not change any result.");
      expect(f.chats()).toBe(seedFailed); // one request for each failed test with error text, none for the passed or skipped one

      // The card never touches the results: the rows and the counts are as the run left them.
      expect(await page.locator("#results-list").innerHTML()).toBe(listBefore);
      expect(await page.locator("#results-summary").innerText()).toBe(summaryBefore);
      expect(await card(page).evaluate((el) => !!el.closest("#results-list"))).toBe(false);
    } finally {
      await f.close();
    }
  });

  test("ex-03 a destination on this machine is shown once in the session, and again after a settings change", async ({ page }) => {
    const f = await fakeProvider("advice");
    try {
      useProvider(f);
      await openStudio(page);
      await page.click("#btn-explain");
      await page.click("#btn-explain-preview-send");
      await expect(card(page)).toBeVisible();
      expect(f.chats()).toBe(seedFailed);

      await page.click("#btn-explain"); // the same destination: no second preview
      await expect(overlay(page)).toBeHidden();
      await expect(page.locator("#btn-explain")).toBeEnabled();
      await expect.poll(() => f.chats()).toBe(seedFailed * 2);

      useProvider(f, "other-model"); // the file changes under the open window: another model is another destination
      await page.click("#btn-explain");
      await expect(overlay(page)).toBeVisible();
      expect((await page.locator("#explain-preview-text").innerText()).split("\n")[0]).toBe("Sent to: other-model, on this machine");
      await page.click("#btn-explain-preview-cancel");
      expect(f.chats()).toBe(seedFailed * 2);

      // Back to the destination that was confirmed: it goes without a second preview, because the cancelled one never counted.
      useProvider(f);
      await page.click("#btn-explain");
      await expect(overlay(page)).toBeHidden();
      await expect.poll(() => f.chats()).toBe(seedFailed * 3);

      // Saving a change to the provider settings shows the preview again for the same destination.
      await page.click("#btn-settings");
      await expect(page.locator("#settings-port")).toHaveValue(/^\d+$/);
      await page.fill("#settings-ai-timeout", "90s"); // any change to the provider settings
      await page.click("#btn-settings-save");
      await expect(page.locator("#toast-container")).toContainText("Saved probe.yaml");
      await expect(page.locator("#settings-overlay")).toBeHidden();
      await page.click("#btn-explain");
      await expect(overlay(page)).toBeVisible();
      await page.click("#btn-explain-preview-cancel");
    } finally {
      await f.close();
    }
  });

  test("ex-04 a hosted or remote destination shows the preview on every Explain, and the card says hosted", async ({ page }) => {
    const f = await fakeProvider("advice");
    try {
      useProvider(f);
      await pretendHosted(page);
      await openStudio(page);
      for (let i = 1; i <= 3; i++) {
        await page.click("#btn-explain");
        await expect(overlay(page), `Explain ${i}`).toBeVisible();
        await expect(page.locator("#explain-preview-note")).toContainText("before every Explain");
        expect(f.chats()).toBe((i - 1) * seedFailed);
        await page.click("#btn-explain-preview-send");
        await expect(card(page).locator(".explain-place")).toHaveText("hosted");
        expect(f.chats()).toBe(i * seedFailed);
      }
    } finally {
      await f.close();
    }
  });

  test("ex-05 the seeded text is hostile and stays data: markup in the card is text, credentials and personal text never leave", async ({ page }) => {
    const f = await fakeProvider('Try <img src=x onerror="window.__explainXss=2"> and <script>window.__explainXss=3</script>​ done');
    try {
      useProvider(f);
      await openStudio(page);
      await page.click("#btn-explain");
      await page.click("#btn-explain-preview-send");
      await expect(card(page)).toContainText("Try <img src=x");
      await expect(card(page).locator("img, script, b")).toHaveCount(0); // text, not elements
      await expect(card(page)).toContainText("<b>sign out</b>"); // a test name with markup is shown as text
      expect(await page.evaluate(() => (window as any).__explainXss)).toBeUndefined();
      expect(await card(page).innerText()).not.toContain("​");

      const preview = await page.evaluate(async () => (window as any).go.main.App.PreviewExplain());
      const sent = f.bodies().join("\n") + "\n" + preview.text;
      for (const bad of ["ghp_", seedToken.slice(0, 12), "jane", "example.com", "alice", "\\u001b", "skipped-error-text-must-not-be-sent", seedEmail]) {
        expect(sent, bad).not.toContain(bad);
      }
      expect(sent).toContain("[redacted]");
    } finally {
      await f.close();
    }
  });

  test("ex-06 a setting that changes between the preview and Send sends nothing", async ({ page }) => {
    const f = await fakeProvider("advice");
    try {
      useProvider(f);
      await openStudio(page);
      await page.click("#btn-explain");
      await expect(overlay(page)).toBeVisible();
      useProvider(f, "switched-model"); // the destination changes while the preview is open
      await page.click("#btn-explain-preview-send");
      await expect(page.locator("#explain-status")).toContainText("destination changed");
      await expect(card(page)).toBeHidden();
      expect(f.chats()).toBe(0);
    } finally {
      await f.close();
    }
  });

  test("ex-07 a provider that fails gives a fixed sentence, never its own message", async ({ page }) => {
    const f = await fakeProvider("secret detail http://127.0.0.1:1/internal", 500);
    try {
      useProvider(f);
      await openStudio(page);
      await page.click("#btn-explain");
      await page.click("#btn-explain-preview-send");
      await expect(card(page)).toContainText("The AI provider could not answer");
      expect(await card(page).innerText()).not.toContain("secret detail");
      expect(await card(page).innerText()).not.toContain("127.0.0.1");
      expect(f.chats()).toBe(1); // it stops after the first error
    } finally {
      await f.close();
    }
  });

  test("ex-08 turning AI off hides the button, and a double click sends one preview", async ({ page }) => {
    const f = await fakeProvider("advice");
    try {
      useProvider(f);
      await openStudio(page);
      await page.locator("#btn-explain").dblclick();
      await expect(overlay(page)).toBeVisible();
      await page.click("#btn-explain-preview-cancel");
      expect(f.chats()).toBe(0);

      await page.click("#btn-settings");
      await expect(page.locator("#settings-port")).toHaveValue(/^\d+$/);
      await page.selectOption("#settings-ai-provider", "none"); // Off
      await page.click("#btn-settings-save");
      await expect(page.locator("#settings-overlay")).toBeHidden();
      await expect(page.locator("#btn-explain")).toBeHidden();
    } finally {
      await f.close();
    }
  });
});
