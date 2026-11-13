import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as http from "node:http";
import * as path from "node:path";
import { app, openStudio, workspace } from "./helpers";

// The chat header names where the next message would go, against a fake provider on 127.0.0.1. No hosted provider is called, and
// keys live in memory (PROBE_STUDIO_KEYCHAIN=memory), so no spec touches the real keychain.

const chatModel = "claude-sonnet-4-6"; // the Anthropic model Studio uses when probe.yaml names none

interface Fake {
  url: string;
  close: () => Promise<void>;
}

/** A fake OpenAI-compatible provider on 127.0.0.1 that answers every chat with "pong". */
async function fakeProvider(): Promise<Fake> {
  const server = http.createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      res.setHeader("Content-Type", "application/json");
      if (req.method === "POST" && (req.url ?? "").endsWith("/chat/completions")) {
        res.end(JSON.stringify({ choices: [{ message: { content: "pong" } }] }));
        return;
      }
      res.end(JSON.stringify({ data: [{ id: "fake-model" }] }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as { port: number }).port;
  return { url: `http://127.0.0.1:${port}`, close: () => new Promise((resolve) => server.close(() => resolve())) };
}

const yamlPath = path.join(workspace, "probe.yaml");
let original = "";

/** Opens Settings and waits until the form has loaded, because the load fills the fields and would overwrite an early change. */
async function openSettings(page: Page) {
  await page.click("#btn-settings");
  await expect(page.locator("#settings-overlay")).toBeVisible();
  await expect(page.locator("#settings-port")).toHaveValue(/^\d+$/);
}

/** Opens the chat pane. The header is filled when the pane opens and kept fresh while it is open, and not before. */
async function openPane(page: Page) {
  if (await page.locator("#chat-pane").isHidden()) await page.click("#btn-ai-toggle");
  await expect(page.locator("#chat-pane")).toBeVisible();
}

test.describe("Chat header (fake provider on 127.0.0.1)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test.beforeEach(() => {
    original = fs.readFileSync(yamlPath, "utf8"); // the fixture has no ai: block
    if (!original.endsWith("\n")) original += "\n";
  });
  test.afterEach(async ({ page }) => {
    fs.writeFileSync(yamlPath, original);
    await app(page, "DeleteAPIKey").catch(() => undefined); // a key saved in memory by a spec does not outlive it
  });

  test("ai-h01 AI off says so", async ({ page }) => {
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: none\n`);
    await openStudio(page);
    await openPane(page);
    await expect(page.locator("#chat-provider")).toHaveText("AI is off");
  });

  test("ai-h02 a local provider shows its model and that it is on this machine, and no dollar figure", async ({ page }) => {
    const f = await fakeProvider();
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: fake-model\n`);
      await openStudio(page);
      await openPane(page);
      await expect(page.locator("#chat-provider")).toHaveText("fake-model, on this machine");
      await expect(page.locator("#chat-provider")).not.toContainText("127.0.0.1"); // never the endpoint

      await page.fill("#chat-input", "hello");
      await page.press("#chat-input", "Enter");
      // The preview names the same destination the header does.
      await expect(page.locator("#ai-preview-overlay")).toBeVisible();
      await expect(page.locator("#ai-preview-text")).toContainText("Sent to: local endpoint");
      await expect(page.locator("#ai-preview-text")).toContainText("model fake-model");
      await page.click("#btn-ai-preview-send");
      await expect(page.locator(".chat-msg-assistant")).toContainText("pong");
      await expect(page.locator("#chat-cost")).toHaveText(""); // a local model has no dollar cost to show
    } finally {
      await f.close();
    }
  });

  test("ai-h03 the header follows a Settings save, with no reload", async ({ page }) => {
    const f = await fakeProvider();
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: fake-model\n`);
      await openStudio(page);
      await openPane(page);
      await expect(page.locator("#chat-provider")).toHaveText("fake-model, on this machine");
      await openSettings(page);
      await page.selectOption("#settings-ai-provider", "none");
      await page.click("#btn-settings-save");
      await expect(page.locator("#settings-overlay")).toBeHidden();
      await expect(page.locator("#chat-provider")).toHaveText("AI is off");
    } finally {
      await f.close();
    }
  });

  test("ai-h04 the header follows a saved key, with no reload", async ({ page }) => {
    await openStudio(page);
    await app(page, "DeleteAPIKey").catch(() => undefined); // an earlier spec may have left a key in memory
    await page.reload();
    await openPane(page);
    await expect(page.locator("#chat-provider")).toHaveText("AI is off");
    await page.click("#btn-chat-key");
    await page.fill("#api-key-input", "sk" + "-ant-" + "e2e-label-keychain-000"); // kept in memory
    await page.click("#btn-api-key-save");
    await expect(page.locator("#toast-container")).toContainText("API key saved");
    await expect(page.locator("#chat-provider")).toHaveText(`Anthropic, ${chatModel}`);
  });

  test("ai-h05 a model id with a token and a tag is gated and shown as text", async ({ page }) => {
    const f = await fakeProvider();
    try {
      const token = "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789";
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: "<b>bold</b>-${token}"\n`);
      await openStudio(page);
      await openPane(page);
      const header = page.locator("#chat-provider");
      await expect(header).toContainText("<b>bold</b>"); // the tag is text
      await expect(header).not.toContainText("ghp_"); // the token is not shown
      await expect(header.locator("b")).toHaveCount(0); // and no element was made from it
      await expect(header).toHaveAttribute("title", /^<b>bold<\/b>.*, on this machine$/); // the title holds the same text, set as an attribute
    } finally {
      await f.close();
    }
  });

  test("ai-h06 a remote endpoint says so when it is confirmed, and the header asks for a fix when it is not", async ({ page }) => {
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: fake-model\nstudio:\n  ai_allow_remote_endpoint: true\n`);
    await openStudio(page);
    await openPane(page);
    await expect(page.locator("#chat-provider")).toHaveText("fake-model, remote endpoint");
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: fake-model\n`);
    await page.reload();
    await openPane(page);
    await expect(page.locator("#chat-provider")).toHaveText("AI needs a fix, see Settings");
  });

  test("ai-h07 a key removed through the dialog changes the header", async ({ page }) => {
    await openStudio(page);
    await app(page, "DeleteAPIKey").catch(() => undefined);
    await page.reload();
    await openPane(page);
    await page.click("#btn-chat-key");
    await page.fill("#api-key-input", "sk" + "-ant-" + "e2e-label-delete-000"); // kept in memory
    await page.click("#btn-api-key-save");
    await expect(page.locator("#chat-provider")).toHaveText(`Anthropic, ${chatModel}`);
    await page.click("#btn-chat-key");
    await page.click("#btn-api-key-delete");
    await expect(page.locator("#toast-container")).toContainText("removed");
    await expect(page.locator("#chat-provider")).toHaveText("AI is off");
  });

  test("ai-h08 the header is not asked for before the chat pane opens", async ({ page }) => {
    const f = await fakeProvider();
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: fake-model\n`);
      await openStudio(page);
      await page.waitForTimeout(400); // long enough for an eager refresh at launch to fill it
      await expect(page.locator("#chat-pane")).toBeHidden();
      await expect(page.locator("#chat-provider")).toHaveText(""); // nothing was asked: no keychain read for a pane nobody opened
      await openPane(page);
      await expect(page.locator("#chat-provider")).toHaveText("fake-model, on this machine");
    } finally {
      await f.close();
    }
  });

  test("ai-h09 a model id cannot spoof the header: direction marks are replaced and the header is isolated", async ({ page }) => {
    const f = await fakeProvider();
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: "a\\u202Eb\\u200Bc, remote endpoint"\n`);
      await openStudio(page);
      await openPane(page);
      const header = page.locator("#chat-provider");
      await expect(header).toHaveText("a?b?c? remote endpoint, on this machine"); // the override, the zero-width character and the comma are shown as ?
      expect(await header.evaluate((el) => el.textContent)).not.toMatch(/[\u202A-\u202E\u2066-\u2069\u200B-\u200F]/);
      expect(await header.evaluate((el) => getComputedStyle(el).unicodeBidi)).toBe("isolate"); // whatever is left cannot reorder the page
    } finally {
      await f.close();
    }
  });

  test("ai-h10 a very long model id is shortened with an ellipsis and the place stays in view", async ({ page }) => {
    const longModel = "x".repeat(190);
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: "${longModel}"\nstudio:\n  ai_allow_remote_endpoint: true\n`);
    await openStudio(page);
    await openPane(page);
    const header = page.locator("#chat-provider");
    await expect(header).toHaveText(`${longModel}, remote endpoint`); // the whole line is the text
    await expect(header).toHaveAttribute("title", `${longModel}, remote endpoint`);
    const model = header.locator(".chat-provider-model");
    expect(await model.evaluate((el) => el.scrollWidth > el.clientWidth)).toBe(true); // the model is cut with an ellipsis
    expect(await model.evaluate((el) => getComputedStyle(el).textOverflow)).toBe("ellipsis");
    // The place after it is on screen: inside the pane and inside the window.
    const place = header.locator(".chat-provider-place");
    await expect(place).toBeVisible();
    await expect(place).toHaveText(", remote endpoint");
    const pane = (await page.locator("#chat-pane").boundingBox())!;
    const box = (await place.boundingBox())!;
    const viewport = page.viewportSize()!;
    expect(box.x + box.width).toBeLessThanOrEqual(pane.x + pane.width + 1);
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);
    expect(box.x).toBeGreaterThanOrEqual(0);
  });
});
