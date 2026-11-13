import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as http from "node:http";
import * as path from "node:path";
import { openStudio, workspace } from "./helpers";

// Find local models against a fake model server on a free loopback port. The suite points the default target at a port that
// refuses at once (PROBE_STUDIO_LOCAL_ENDPOINT), so no spec asks a real Ollama. No hosted provider, no key, no keychain.

interface Fake {
  url: string; // the endpoint, with /v1
  requests: () => string[];
  close: () => Promise<void>;
}

/** A fake model server that lists ids and records "METHOD path" for every request. */
async function fakeOllama(ids: string[]): Promise<Fake> {
  const seen: string[] = [];
  const server = http.createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      seen.push(`${req.method} ${req.url}`);
      res.setHeader("Content-Type", "application/json");
      res.end(JSON.stringify({ data: ids.map((id) => ({ id })) }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as { port: number }).port;
  return { url: `http://127.0.0.1:${port}/v1`, requests: () => [...seen], close: () => new Promise((resolve) => server.close(() => resolve())) };
}

const yamlPath = path.join(workspace, "probe.yaml");
let original = "";
const readYaml = () => fs.readFileSync(yamlPath, "utf8");

/** Opens Settings and waits until the form has loaded, because the load fills the fields and would overwrite an early change. */
async function openSettings(page: Page) {
  await openStudio(page);
  await page.click("#btn-settings");
  await expect(page.locator("#settings-overlay")).toBeVisible();
  await expect(page.locator("#settings-port")).toHaveValue(/^\d+$/);
}

test.describe("Find local models (fake server on a free loopback port)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test.beforeEach(() => {
    original = readYaml(); // the fixture has no ai: block
    if (!original.endsWith("\n")) original += "\n";
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));

  test("ai-d01 nothing is asked before the click, and the click asks for the model list only", async ({ page }) => {
    const f = await fakeOllama(["fake-model"]);
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}\n  model: fake-model\n`);
      await openSettings(page);
      await page.waitForTimeout(500); // long enough for a request on load or on opening Settings to arrive
      expect(f.requests()).toEqual([]);
      await page.click("#btn-ai-find-models");
      await expect(page.locator("#settings-ai-find-status")).toContainText("Found 1 model");
      expect(f.requests()).toEqual(["GET /v1/models"]);
    } finally {
      await f.close();
    }
  });

  test("ai-d02 one model fills the empty fields, says what it filled, and saves nothing until Save", async ({ page }) => {
    const f = await fakeOllama(["only-model"]);
    try {
      await openSettings(page);
      await page.fill("#settings-ai-endpoint", f.url); // the provider is Not set and the model is empty
      await page.click("#btn-ai-find-models");
      const status = page.locator("#settings-ai-find-status");
      await expect(status).toContainText("Filled: provider, model.");
      await expect(status).toContainText("Nothing is saved until you press Save.");
      await expect(page.locator("#settings-ai-provider")).toHaveValue("local");
      await expect(page.locator("#settings-ai-endpoint")).toHaveValue(f.url); // already had a value: not overwritten
      await expect(page.locator("#settings-ai-model")).toHaveValue("only-model");
      expect(readYaml()).toBe(original); // the click saved nothing

      await page.click("#btn-settings-save");
      await expect(page.locator("#settings-overlay")).toBeHidden();
      expect(readYaml()).toContain("model: only-model");
    } finally {
      await f.close();
    }
  });

  test("ai-d03 several models are offered as suggestions, and the model is not picked for the person", async ({ page }) => {
    const f = await fakeOllama(["qwen3-coder:30b", "gemma4:31b", "other:7b"]);
    try {
      await openSettings(page);
      await page.fill("#settings-ai-endpoint", f.url);
      await page.click("#btn-ai-find-models");
      const status = page.locator("#settings-ai-find-status");
      await expect(status).toContainText("Found 3 models");
      await expect(status).toContainText("Filled: provider.");
      await expect(status).toContainText("Pick one in the Model field.");
      await expect(page.locator("#settings-ai-model")).toHaveValue(""); // not picked silently
      await expect(page.locator("#settings-ai-models option")).toHaveCount(3);
    } finally {
      await f.close();
    }
  });

  test("ai-d04 a field that already has a value is not overwritten", async ({ page }) => {
    const f = await fakeOllama(["other-model"]);
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: none\n  endpoint: ${f.url}\n  model: mine\n`);
      await openSettings(page);
      await page.click("#btn-ai-find-models");
      await expect(page.locator("#settings-ai-find-status")).toContainText("Filled nothing");
      await expect(page.locator("#settings-ai-provider")).toHaveValue("none"); // Off is a choice, not an empty field
      await expect(page.locator("#settings-ai-model")).toHaveValue("mine");
    } finally {
      await f.close();
    }
  });

  test("ai-d04b a model the person already chose is not replaced, even when one is listed", async ({ page }) => {
    const f = await fakeOllama(["other-model"]);
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}\n  model: mine\n`);
      await openSettings(page);
      await page.click("#btn-ai-find-models");
      await expect(page.locator("#settings-ai-find-status")).toContainText("Filled nothing");
      await expect(page.locator("#settings-ai-model")).toHaveValue("mine");
      await expect(page.locator("#settings-ai-provider")).toHaveValue("local");
    } finally {
      await f.close();
    }
  });

  test("ai-d05 when nothing answers it says so and what to start", async ({ page }) => {
    const f = await fakeOllama(["x"]);
    const closed = f.url;
    await f.close();
    fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${closed}\n  model: m\n`);
    await openSettings(page);
    await page.click("#btn-ai-find-models");
    const status = page.locator("#settings-ai-find-status");
    await expect(status).toContainText("no answer");
    await expect(status).toContainText("Start your local model server");
  });

  test("ai-d06 what a server sends is shown as text, never as markup", async ({ page }) => {
    const token = "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789";
    const f = await fakeOllama(['"><img src=x onerror=window.__pwned=1>', "<b>bold</b>", `a\u0000b-${token}`]);
    try {
      await openSettings(page);
      await page.fill("#settings-ai-endpoint", f.url);
      await page.click("#btn-ai-find-models");
      await expect(page.locator("#settings-ai-find-status")).toContainText("Found 3 models");
      const values = await page.locator("#settings-ai-models option").evaluateAll((os) => os.map((o) => (o as HTMLOptionElement).value));
      expect(values).toContain('"><img src=x onerror=window.__pwned=1>'); // kept as text
      expect(values).toContain("<b>bold</b>");
      expect(values.join("|")).not.toContain("ghp_"); // the token is redacted
      expect(values.join("|")).not.toContain("\u0000");
      await expect(page.locator("img[src='x']")).toHaveCount(0); // no element was made from an id
      expect(await page.evaluate(() => (window as unknown as { __pwned?: number }).__pwned)).toBeUndefined();
    } finally {
      await f.close();
    }
  });
});
