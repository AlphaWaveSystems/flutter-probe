import { expect, Page, test } from "@playwright/test";
import * as fs from "node:fs";
import * as path from "node:path";
import { fakeProvider } from "./ai-fake";
import { openStudio, workspace } from "./helpers";

// The AI form is a snapshot of probe.yaml: Save writes only what the person changed, takes the rest from the file as it is now, refuses a change
// to a field that the file changed under the form, and the form shows what the file holds after a save. The page is the real Studio window code
// over the headless bridge; probe.yaml is edited from outside the page, as an editor would.

const yamlPath = path.join(workspace, "probe.yaml");
let original = "";
const readYaml = () => fs.readFileSync(yamlPath, "utf8");

async function openSettings(page: Page) {
  await openStudio(page);
  await page.click("#btn-settings");
  await expect(page.locator("#settings-overlay")).toBeVisible();
  await expect(page.locator("#settings-port")).toHaveValue(/^\d+$/);
  await expect(page.locator("#settings-ai-model")).toHaveValue(/.+/); // the AI form has loaded
}

test.describe("Settings AI form snapshot", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  test.beforeEach(() => {
    original = readYaml();
    if (!original.endsWith("\n")) original += "\n";
  });
  test.afterEach(() => fs.writeFileSync(yamlPath, original));

  test("ab-01 after a save the form shows what the file holds, including an edit made under it, and the next save is not a conflict", async ({ page }) => {
    const f = await fakeProvider();
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: m1\n  timeout: 45s\n`);
      await openSettings(page);
      fs.writeFileSync(yamlPath, readYaml().replace("timeout: 45s", "timeout: 46s")); // an editor changes the timeout under the form
      await page.fill("#settings-ai-model", "m2");
      await page.fill("#settings-retry", "11"); // the other settings are refused, so the form stays open after the AI section was saved
      await page.click("#btn-settings-save");
      await expect(page.locator("#toast-container .toast", { hasText: "outside 0 to 10" })).toBeVisible();
      await expect(page.locator("#settings-overlay")).toBeVisible();
      expect(readYaml()).toContain("model: m2");
      expect(readYaml()).toContain("timeout: 46s"); // the edit made elsewhere was kept
      await expect(page.locator("#settings-ai-timeout")).toHaveValue("46s"); // and the form shows it
      await page.fill("#settings-retry", "");
      await page.fill("#settings-ai-model", "m3"); // a second save, from the refreshed base
      await page.click("#btn-settings-save");
      await expect(page.locator("#settings-overlay")).toBeHidden();
      expect(readYaml()).toContain("model: m3");
      expect(readYaml()).toContain("timeout: 46s");
    } finally {
      await f.close();
    }
  });

  test("ab-02 a change to a field the file changed is refused, stays refused until the form is reopened, and then goes through", async ({ page }) => {
    const f = await fakeProvider();
    try {
      fs.writeFileSync(yamlPath, `${original}ai:\n  provider: local\n  endpoint: ${f.url}/v1\n  model: m1\n`);
      await openSettings(page);
      fs.writeFileSync(yamlPath, readYaml().replace("model: m1", "model: from-editor"));
      await page.fill("#settings-ai-model", "mine");
      await page.click("#btn-settings-save");
      const error = page.locator("#settings-ai-error");
      await expect(error).toContainText("changed on disk");
      await expect(error).toContainText("the model");
      await expect(error).not.toContainText("from-editor"); // the field is named, no value
      expect(readYaml()).toContain("model: from-editor");
      await page.click("#btn-settings-save"); // still refused: the form was not reloaded
      await expect(error).toContainText("changed on disk");
      expect(readYaml()).toContain("model: from-editor");
      await page.click("#btn-settings-close");
      await page.click("#btn-settings");
      await expect(page.locator("#settings-ai-model")).toHaveValue("from-editor"); // reopening reloads the file
      await page.fill("#settings-ai-model", "mine");
      await page.click("#btn-settings-save");
      await expect(page.locator("#settings-overlay")).toBeHidden();
      expect(readYaml()).toContain("model: mine");
    } finally {
      await f.close();
    }
  });
});
