import { expect, Page, test } from "@playwright/test";
import { app, openStudio, workspace } from "./helpers";
import { netEmail, netHome, netSecret, netToken } from "./explain-seed";

// The Network tab, against the request log Studio starts with (PROBE_STUDIO_SEED_EXCHANGES, headless only). No app is connected and no
// request leaves the machine. The seeded traffic holds a secret in every position, on purpose.

const rows = (page: Page) => page.locator("#net-list li[data-seq]");

async function openNetwork(page: Page) {
  await openStudio(page);
  await page.click("#tab-network");
  await expect(rows(page)).toHaveCount(7);
}

test.describe("Network tab (seeded request log)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test("nw-01 the tab lists the requests, oldest first, with the query values hidden", async ({ page }) => {
    await openStudio(page);
    await expect(page.locator("#network-panel")).toBeHidden();
    await page.click("#tab-network");
    await expect(page.locator("#network-panel")).toBeVisible();
    await expect(page.locator("#results-panel")).toBeHidden();
    await expect(rows(page)).toHaveCount(7);
    const seqs = await rows(page).evaluateAll((els) => els.map((e) => (e as HTMLElement).dataset.seq));
    expect(seqs).toEqual(["1", "2", "3", "4", "5", "6", "7"]);
    const first = rows(page).first();
    await expect(first).toContainText("GET");
    await expect(first).toContainText("200");
    await expect(first).toContainText("41 ms");
    await expect(first).toContainText("https://api.example.test/v1/me?token=[redacted]&id=[redacted]");
    await expect(rows(page).nth(1)).toContainText("401");
    await expect(rows(page).nth(4)).toContainText("error");
    await expect(rows(page).nth(6)).toContainText("mocked");
    await expect(page.locator("#net-info")).toHaveText("7");
    expect(await page.locator("#network-panel").innerText()).not.toContain(netSecret);
  });

  test("nw-02 the filters work on what is shown: a parameter name is found, its value is not", async ({ page }) => {
    await openNetwork(page);
    await page.selectOption("#net-method", "POST");
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("/v1/login");
    await page.selectOption("#net-method", "");
    await page.selectOption("#net-status", "4xx");
    await expect(rows(page)).toHaveCount(1);
    await page.selectOption("#net-status", "error");
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("down.example.test");
    await page.selectOption("#net-status", "");
    await page.fill("#net-text", "token");
    await expect(rows(page)).toHaveCount(1); // the parameter name is shown, so it is found
    await page.fill("#net-text", netSecret);
    await expect(page.locator("#net-list .empty")).toContainText("No request matches the filter");
    await page.fill("#net-text", netSecret.slice(0, 5)); // not even the start of the value
    await expect(page.locator("#net-list .empty")).toBeVisible();
    await page.fill("#net-text", "");
    await expect(rows(page)).toHaveCount(7);
  });

  test("nw-03 a detail shows the header names and hides their values, and the body without credentials or personal text", async ({ page }) => {
    await openNetwork(page);
    await rows(page).first().click();
    const detail = page.locator("#net-detail");
    await expect(detail.locator(".net-hname", { hasText: /^X-Note$/ }).locator("xpath=following-sibling::span[1]")).not.toContainText("ghp_");
    await expect(detail).toContainText("Authorization");
    await expect(detail.locator(".net-hname", { hasText: /^Authorization$/ }).locator("xpath=following-sibling::span[1]")).toHaveText("[redacted]");
    await expect(detail.locator(".net-hname", { hasText: /^Cookie$/ }).locator("xpath=following-sibling::span[1]")).toHaveText("[redacted]");
    await expect(detail.locator(".net-hname", { hasText: /^X_Api_Key$/ }).locator("xpath=following-sibling::span[1]")).toHaveText("[redacted]");
    await expect(detail.locator(".net-hname", { hasText: /^Content-Type$/ }).first().locator("xpath=following-sibling::span[1]")).toHaveText("application/json");
    await expect(detail).toContainText('"password": "[redacted]"');
    await expect(detail).toContainText("[email]");
    const all = await page.locator("body").innerText();
    for (const bad of [netSecret, netEmail, "jane.doe", "alice", netHome]) expect(all, bad).not.toContain(bad);
    await expect(rows(page).first()).toHaveClass(/selected/);
  });

  test("nw-04 hostile bodies stay text, and binary, large and many-header exchanges are summarised", async ({ page }) => {
    await openNetwork(page);
    await rows(page).nth(1).click();
    await expect(page.locator("#net-detail pre").last()).toContainText("<img src=x");
    await expect(page.locator("#net-detail img, #net-detail script")).toHaveCount(0);
    expect(await page.evaluate(() => (window as any).__netXss)).toBeUndefined();
    expect(await page.locator("#net-detail").innerText()).not.toContain(netToken.slice(0, 12));
    for (const bad of [netToken.slice(4, 20), "jane.doe", "alice", "example.com"]) expect(await page.locator("#net-detail").innerText(), bad).not.toContain(bad); // split by hidden and glyphless characters
    await rows(page).nth(2).click();
    await expect(page.locator("#net-detail")).toContainText("binary, 4.0 KB, not shown");
    await expect(page.locator("#net-detail pre")).toHaveCount(0);
    await rows(page).nth(3).click();
    await expect(page.locator("#net-detail")).toContainText("over 1 MB");
    await rows(page).nth(4).click();
    await expect(page.locator("#net-detail")).toContainText("connection refused");
    expect(await page.locator("#net-detail").innerText()).not.toContain(netSecret);
    await rows(page).nth(5).click();
    await expect(page.locator("#net-detail")).toContainText("more not shown");
    expect(await page.locator("#net-detail .net-hname").count()).toBeLessThanOrEqual(100 + 2);
  });

  test("nw-05 nothing is stored in the browser and nothing but Studio is asked", async ({ page }) => {
    const hosts = new Set<string>();
    page.on("request", (r) => {
      if (/^https?:/.test(r.url())) hosts.add(new URL(r.url()).host); // page requests only: data: and blob: URLs have no host
    });
    await openNetwork(page);
    await rows(page).first().click();
    await rows(page).nth(1).click();
    await page.click("#btn-net-refresh");
    await page.waitForTimeout(300);
    const base = new URL(page.url()).host;
    expect([...hosts].filter((h) => h !== base)).toEqual([]);
    const stored = await page.evaluate(() => JSON.stringify({ l: { ...localStorage }, s: { ...sessionStorage } }));
    for (const bad of [netSecret, "jane.doe", "api.example.test", "net-list", "x-many"]) expect(stored, bad).not.toContain(bad);
  });

  test("nw-06 the Results tab comes back as it was, and the Network tab asks again when it is shown", async ({ page }) => {
    await openNetwork(page);
    await page.click("#tab-results");
    await expect(page.locator("#results-panel")).toBeVisible();
    await expect(page.locator("#network-panel")).toBeHidden();
    await expect(page.locator("#tab-results")).toHaveClass(/active/);
    await expect(page.locator("#tab-network")).not.toHaveClass(/active/);
    await page.click("#tab-network");
    await expect(rows(page)).toHaveCount(7);
  });

  test("nw-07 a detail for another epoch is refused by Studio, and an unknown request is not found", async ({ page }) => {
    await openStudio(page);
    await expect(app(page, "HTTPExchange", "another-epoch", 1)).rejects.toThrow(/restarted/);
    await expect(app(page, "HTTPExchange", "e2e-epoch", 99)).rejects.toThrow(/no longer in the log/);
    await expect(app(page, "HTTPExchange", "", 1)).rejects.toThrow(/restart marker/);
    const ok: any = await app(page, "HTTPExchange", "e2e-epoch", 1);
    expect(JSON.stringify(ok)).not.toContain(netSecret);
  });
});
