import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { fakeProvider } from "./ai-fake";
import { openStudio, workspace } from "./helpers";

// The Settings AI section against a fake provider on 127.0.0.1. No hosted provider is called, no key is used, and the keychain
// is not touched.

const yamlPath = path.join(workspace, "probe.yaml");
let original = "";
const readYaml = () => fs.readFileSync(yamlPath, "utf8");

/** Opens Settings and waits until the form has loaded: the load fills the fields and would overwrite an early change. */
async function openSettings(page: Page) {
  await openStudio(page);
  await page.click("#btn-settings");
  await expect(page.locator("#settings-overlay")).toBeVisible();
  await expect(page.locator("#settings-port")).toHaveValue(/^\d+$/);
}

test.describe("Settings AI section (fake provider on 127.0.0.1)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test.beforeEach(() => {
    original = readYaml(); // the fixture has no ai: block
    if (!original.endsWith("\n")) original += "\n";
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));

  test("ai-s01 Test connection with AI off says so and makes no request", async ({ page }) => {
    const f = await fakeProvider();
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: none\n  endpoint: ${f.url}/v1\n  model: fake-model\n`);
      await openSettings(page);
      await page.click("#btn-ai-test");
      await expect(page.locator("#settings-ai-result")).toContainText("Cannot test: AI is off: ai.provider is none");
      expect(f.paths()).toEqual([]);
    } finally {
      await f.close();
    }
  });

  test("ai-s02 Test connection with a local provider shows the checks and the summary", async ({ page }) => {
    const f = await fakeProvider("pong");
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: fake-model\n`);
      await openSettings(page);
      await page.click("#btn-ai-test");
      const result = page.locator("#settings-ai-result");
      await expect(result).toContainText("Text works.");
      for (const line of ["✓ config", "✓ endpoint", "✓ text"]) await expect(result).toContainText(line);
      // It reached the endpoint the form names: the model listing and a chat completion, nothing else.
      expect(f.paths()).toContain("GET /v1/models");
      expect(f.paths()).toContain("POST /v1/chat/completions");
    } finally {
      await f.close();
    }
  });

  test("ai-s03 List models fills the model suggestions from the endpoint", async ({ page }) => {
    const f = await fakeProvider();
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: fake-model\n`);
      await openSettings(page);
      await page.click("#btn-ai-list-models");
      await expect(page.locator("#toast-container")).toContainText("1 models listed");
      await expect(page.locator("#settings-ai-models option")).toHaveAttribute("value", "fake-model");
    } finally {
      await f.close();
    }
  });

  test("ai-s04 a plain key in probe.yaml shows the note and survives a Save", async ({ page }) => {
    const f = await fakeProvider();
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: fake-model\n  api_key: plain-key-in-file-e2e\n`);
      await openSettings(page);
      const note = page.locator("#settings-ai-plain-key");
      await expect(note).toBeVisible();
      await expect(note).toContainText("does not use and does not change");
      await page.fill("#settings-port", "48801"); // change something that has nothing to do with the key
      await page.click("#btn-settings-save");
      await expect(page.locator("#toast-container")).toContainText("Saved probe.yaml");
      expect(readYaml()).toContain("api_key: plain-key-in-file-e2e");
      expect(readYaml()).toMatch(/port:\s*48801/);
    } finally {
      await f.close();
    }
  });

  test("ai-s05 Save writes the form into the ai block, keeps the other keys, and reloads", async ({ page }) => {
    const f = await fakeProvider();
    try {
      await openSettings(page);
      await page.selectOption("#settings-ai-provider", "local");
      await page.fill("#settings-ai-endpoint", `${f.url}/v1`);
      await page.fill("#settings-ai-model", "fake-model");
      await page.click("#btn-settings-save");
      await expect(page.locator("#toast-container")).toContainText("Saved probe.yaml");
      const written = readYaml();
      expect(written).toContain("provider: local");
      expect(written).toMatch(/endpoint:\s*"?http:\/\/127\.0\.0\.1:\d+\/v1"?/);
      expect(written).toContain("model: fake-model");
      const appId = /\bid:\s*(\S+)/.exec(original)?.[1]; // a key the form does not manage, read from the file as it was
      expect(appId).toBeTruthy();
      expect(written).toContain(appId!); // it is still there
      expect(written).not.toMatch(/api_key/); // Settings never writes a key

      await page.reload();
      await openSettings(page);
      await expect(page.locator("#settings-ai-provider")).toHaveValue("local");
      await expect(page.locator("#settings-ai-model")).toHaveValue("fake-model");
    } finally {
      await f.close();
    }
  });

  test("ai-s06 Off writes provider none", async ({ page }) => {
    await openSettings(page);
    await page.selectOption("#settings-ai-provider", "none");
    await page.click("#btn-settings-save");
    await expect(page.locator("#toast-container")).toContainText("Saved probe.yaml");
    expect(readYaml()).toContain("provider: none");
  });

  test("ai-s07 a form that would not run is refused with the reason and not written", async ({ page }) => {
    await openSettings(page);
    await page.selectOption("#settings-ai-provider", "openai"); // the key source is the keychain, which Studio never sends to OpenAI
    await page.click("#btn-settings-save");
    await expect(page.locator("#settings-ai-error")).toContainText("Environment variable");
    await expect(page.locator("#settings-overlay")).toBeVisible(); // the form stays open
    expect(readYaml()).not.toContain("openai"); // the refused AI form was not written
  });
});
