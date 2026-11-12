import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as http from "node:http";
import * as net from "node:net";
import * as path from "node:path";
import { openStudio, workspace } from "./helpers";

// The first-send preview and the chat, against fake providers on 127.0.0.1. No hosted provider is called and no key is used.

interface Fake {
  url: string;
  chats: () => number;
  close: () => Promise<void>;
}

/** A fake OpenAI-compatible server: it counts chat requests and answers every one with reply. */
async function fakeProvider(reply: string): Promise<Fake> {
  let chats = 0;
  const server = http.createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      res.setHeader("Content-Type", "application/json");
      if (req.method === "POST" && (req.url ?? "").endsWith("/chat/completions")) {
        chats++;
        res.end(JSON.stringify({ choices: [{ message: { content: reply } }] }));
        return;
      }
      res.end(JSON.stringify({ data: [{ id: "fake-model" }] }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as { port: number }).port;
  return { url: `http://127.0.0.1:${port}`, chats: () => chats, close: () => new Promise((resolve) => server.close(() => resolve())) };
}

const yamlPath = path.join(workspace, "probe.yaml");
let original = "";

function useProvider(f: Fake) {
  fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: fake-model\n`);
}

/** A port the OS says is free. The specs write it as the agent port, so no port number is a literal in this file. */
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

/** Opens Settings and waits until the form has loaded, because the load fills the fields and would overwrite an early change. */
async function openSettings(page: Page) {
  await page.click("#btn-settings");
  await expect(page.locator("#settings-overlay")).toBeVisible();
  await expect(page.locator("#settings-port")).toHaveValue(/^\d+$/); // the fixture's agent port, set by the load
}

async function ask(page: Page, text: string) {
  if (await page.locator("#chat-pane").isHidden()) await page.click("#btn-ai-toggle");
  await page.fill("#chat-input", text);
  await page.press("#chat-input", "Enter");
}

test.describe("AI chat preview (fake providers on 127.0.0.1)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test.beforeEach(() => {
    original = fs.readFileSync(yamlPath, "utf8"); // the fixture has no ai: block
    if (!original.endsWith("\n")) original += "\n";
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));

  test("ai-01 the first message names where it goes, and nothing is sent until Send", async ({ page }) => {
    const f = await fakeProvider("answer one");
    try {
      useProvider(f);
      await openStudio(page);
      await ask(page, "hello there");
      await expect(page.locator("#ai-preview-overlay")).toBeVisible();
      // The line the person relies on: the destination and the model.
      expect((await page.locator("#ai-preview-text").innerText()).split("\n")[0]).toBe(`Sent to: local endpoint ${f.url}/v1, model fake-model`);
      expect(f.chats()).toBe(0);

      await page.click("#btn-ai-preview-cancel");
      await expect(page.locator("#ai-preview-overlay")).toBeHidden();
      await expect(page.locator("#chat-input")).toHaveValue("hello there"); // Cancel keeps the text
      expect(f.chats()).toBe(0);

      await page.press("#chat-input", "Enter");
      await page.click("#btn-ai-preview-send");
      await expect(page.locator(".chat-msg-assistant")).toContainText("answer one");
      expect(f.chats()).toBe(1);
    } finally {
      await f.close();
    }
  });

  test("ai-02 a second message to the same destination goes without a preview", async ({ page }) => {
    const f = await fakeProvider("answer");
    try {
      useProvider(f);
      await openStudio(page);
      await ask(page, "first");
      await page.click("#btn-ai-preview-send");
      await expect(page.locator(".chat-msg-assistant")).toHaveCount(1);
      await ask(page, "second");
      await expect(page.locator(".chat-msg-assistant")).toHaveCount(2);
      await expect(page.locator("#ai-preview-overlay")).toBeHidden();
      expect(f.chats()).toBe(2);
    } finally {
      await f.close();
    }
  });

  test("ai-03 a changed destination is shown again, without any Settings save", async ({ page }) => {
    const a = await fakeProvider("from a");
    const b = await fakeProvider("from b");
    try {
      useProvider(a);
      await openStudio(page);
      await ask(page, "to a");
      await page.click("#btn-ai-preview-send");
      await expect(page.locator(".chat-msg-assistant")).toContainText("from a");

      useProvider(b); // the file changes under the open window: the label the preview returns is now different
      await ask(page, "to b");
      await expect(page.locator("#ai-preview-overlay")).toBeVisible();
      expect((await page.locator("#ai-preview-text").innerText()).split("\n")[0]).toBe(`Sent to: local endpoint ${b.url}/v1, model fake-model`);
      expect(b.chats()).toBe(0);
      await page.click("#btn-ai-preview-send");
      await expect(page.locator(".chat-msg-assistant").last()).toContainText("from b");
      expect([a.chats(), b.chats()]).toEqual([1, 1]);
    } finally {
      await a.close();
      await b.close();
    }
  });

  test("ai-04 a second Enter while the preview is open does not send twice", async ({ page }) => {
    const f = await fakeProvider("only one");
    try {
      useProvider(f);
      await openStudio(page);
      await ask(page, "double");
      await expect(page.locator("#ai-preview-overlay")).toBeVisible();
      await expect(page.locator("#chat-input")).toBeDisabled(); // the input is locked while the preview is open
      await page.locator("#chat-input").dispatchEvent("keydown", { key: "Enter" }); // a second Enter reaches the page anyway
      await page.click("#btn-chat-send", { force: true });
      await page.click("#btn-ai-preview-send");
      await expect(page.locator(".chat-msg-assistant")).toHaveCount(1);
      await page.waitForTimeout(500);
      expect(f.chats()).toBe(1);
      await expect(page.locator(".chat-msg-user")).toHaveCount(1);
    } finally {
      await f.close();
    }
  });

  test("ai-05 the preview is shown again after the provider settings are saved", async ({ page }) => {
    const f = await fakeProvider("answer");
    try {
      useProvider(f);
      await openStudio(page);
      await ask(page, "first");
      await page.click("#btn-ai-preview-send");
      await expect(page.locator(".chat-msg-assistant")).toHaveCount(1);

      await openSettings(page);
      await page.fill("#settings-ai-timeout", "90s"); // any change to the provider settings
      await page.click("#btn-settings-save");
      await expect(page.locator("#toast-container")).toContainText("Saved probe.yaml");

      await ask(page, "second");
      await expect(page.locator("#ai-preview-overlay")).toBeVisible();
      await page.click("#btn-ai-preview-cancel");
      expect(f.chats()).toBe(1);
    } finally {
      await f.close();
    }
  });

  test("ai-06 the preview is shown again after the saved key changes", async ({ page }) => {
    const f = await fakeProvider("answer");
    try {
      useProvider(f);
      await openStudio(page);
      await ask(page, "first");
      await page.click("#btn-ai-preview-send");
      await expect(page.locator(".chat-msg-assistant")).toHaveCount(1);

      await page.click("#btn-chat-key");
      await page.fill("#api-key-input", "sk" + "-ant-" + "e2e-memory-keychain-000"); // kept in memory: PROBE_STUDIO_KEYCHAIN=memory
      await page.click("#btn-api-key-save");
      await expect(page.locator("#toast-container")).toContainText("API key saved");

      await ask(page, "second");
      await expect(page.locator("#ai-preview-overlay")).toBeVisible();
      await page.click("#btn-ai-preview-cancel");
      expect(f.chats()).toBe(1);
    } finally {
      await f.close();
    }
  });

  test("ai-07 Save with an untouched AI form succeeds, for OpenAI with a plain key", async ({ page }) => {
    // A setup the CLI accepts: OpenAI with a plain key. The AI form is not touched, so Save must write the rest.
    const aiBlock = "ai:\n  provider: openai\n  model: m\n  api_key: plain-key-in-file-e2e\n";
    fs.writeFileSync(yamlPath, `${original}${aiBlock}`);
    await openStudio(page);
    await openSettings(page);
    await expect(page.locator("#settings-ai-plain-key")).toContainText("Name an environment variable here"); // advice for OpenAI
    const port = await freePort();
    await page.fill("#settings-port", String(port));
    await page.click("#btn-settings-save");
    await expect(page.locator("#settings-overlay")).toBeHidden(); // a clean save closes the form
    await expect(page.locator("#toast-container")).not.toContainText("except");
    const written = fs.readFileSync(yamlPath, "utf8");
    expect(written).toMatch(new RegExp(`port:\\s*${port}`));
    expect(written).toContain("provider: openai");
    expect(written).toContain("api_key: plain-key-in-file-e2e");
  });

  test("ai-09 Save with an untouched AI form succeeds, for OpenAI with no key", async ({ page }) => {
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: openai\n  model: m\n`);
    await openStudio(page);
    await openSettings(page);
    const port = await freePort();
    await page.fill("#settings-port", String(port));
    await page.click("#btn-settings-save");
    await expect(page.locator("#settings-overlay")).toBeHidden();
    await expect(page.locator("#toast-container")).not.toContainText("except");
    const written = fs.readFileSync(yamlPath, "utf8");
    expect(written).toMatch(new RegExp(`port:\\s*${port}`));
    expect(written).toContain("provider: openai");
  });

  test("ai-08 a refused AI form shows its reason and the rest of Settings still saves", async ({ page }) => {
    await openStudio(page);
    await openSettings(page);
    await page.selectOption("#settings-ai-provider", "openai"); // keychain source: refused, Studio never sends it to OpenAI
    const port = await freePort();
    await page.fill("#settings-port", String(port));
    await page.click("#btn-settings-save");
    await expect(page.locator("#settings-ai-error")).toContainText("Environment variable");
    await expect(page.locator("#settings-overlay")).toBeVisible(); // the form stays open on the section that needs a change
    const written = fs.readFileSync(yamlPath, "utf8");
    expect(written).toMatch(new RegExp(`port:\\s*${port}`)); // the rest was saved
    expect(written).not.toContain("openai"); // the refused AI form was not
  });

  test("ai-10 changing the model while a plain key is in the file still saves, and keeps the key", async ({ page }) => {
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: openai\n  model: m\n  api_key: plain-key-in-file-e2e\n`);
    await openStudio(page);
    await openSettings(page);
    await page.fill("#settings-ai-model", "m2"); // a change to the AI form, so it is validated and written
    await page.click("#btn-settings-save");
    await expect(page.locator("#settings-overlay")).toBeHidden();
    const written = fs.readFileSync(yamlPath, "utf8");
    expect(written).toContain("model: m2");
    expect(written).toContain("api_key: plain-key-in-file-e2e");
  });
});
