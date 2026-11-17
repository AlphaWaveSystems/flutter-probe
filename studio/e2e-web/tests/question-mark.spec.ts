import { expect, test } from "@playwright/test";
import { openPath, openStudio, workspace } from "./helpers";

// A `?` is the key that opens the help card, and also a character a person types. In a field it is text: the card stays shut and the character is typed
// (it used to open the card and be swallowed: "why?" in the chat input left "why"). Outside a field the card opens, as before.

test.describe("The ? key", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test("qm-01 a ? typed in the chat input is typed, and the help card stays shut", async ({ page }) => {
    await openStudio(page);
    await page.locator("body").press("ControlOrMeta+Shift+a");
    await expect(page.locator("#chat-pane")).toBeVisible();
    await page.click("#chat-input");
    await page.keyboard.type("why?");
    await expect(page.locator("#chat-input")).toHaveValue("why?");
    await expect(page.locator("#help-overlay")).toBeHidden();
  });

  test("qm-02 a ? typed in the send panel's fields is typed too", async ({ page }) => {
    await openStudio(page);
    await page.click("#tab-network");
    await page.click("#btn-net-send");
    for (const id of ["#send-url", "#send-headers", "#send-body"]) {
      await page.click(id);
      await page.keyboard.type("a?b");
      await expect(page.locator(id)).toHaveValue("a?b");
      await expect(page.locator("#help-overlay")).toBeHidden();
    }
  });

  test("qm-03 a ? typed in the editor is typed (as before)", async ({ page }) => {
    await openStudio(page);
    await openPath(page, "probe-tests/counter.probe");
    await page.locator("#editor .view-line").first().click();
    await page.keyboard.type("?");
    await expect(page.locator("#help-overlay")).toBeHidden();
    await expect(page.locator("#editor-dirty")).toContainText("unsaved");
  });

  test("qm-04 a ? with nothing to type into opens the help card and types nothing, and with a button focused too", async ({ page }) => {
    await openStudio(page);
    await page.locator("body").press("?");
    await expect(page.locator("#help-overlay")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.locator("#help-overlay")).toBeHidden();
    await page.focus("#btn-refresh");
    await page.keyboard.press("?");
    await expect(page.locator("#help-overlay")).toBeVisible();
  });
});
