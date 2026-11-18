import { expect, Page } from "@playwright/test";

export const workspace = process.env.STUDIO_WORKSPACE ?? "";
export const udid = process.env.STUDIO_E2E_UDID ?? "";
export const simName = process.env.STUDIO_E2E_SIM_NAME ?? "StudioE2EWeb-iPhone17";

/** Opens Studio and waits until the workspace is listed. */
export async function openStudio(page: Page) {
  await page.goto("/");
  await expect(page.locator("#file-list li").first()).toBeVisible();
}

/** Clicks a folder or file in the file list, one path segment at a time. */
export async function openPath(page: Page, rel: string) {
  for (const part of rel.split("/")) {
    await page.locator("#file-list li", { hasText: part }).first().click();
    await page.waitForTimeout(300);
  }
}

/** Calls a bound App method directly (the same call the frontend makes). */
export async function app<T = unknown>(page: Page, method: string, ...args: unknown[]): Promise<T> {
  return page.evaluate(([m, a]) => (window as any).go.main.App[m as string](...(a as unknown[])), [method, args]) as Promise<T>;
}

/** Selects the named device in the picker and connects; waits until connected. */
export async function connect(page: Page) {
  await page.click("#btn-refresh");
  const picker = page.locator("#device-picker");
  await expect(picker.locator("option", { hasText: simName })).toHaveCount(1, { timeout: 30_000 });
  await picker.selectOption({ label: (await picker.locator("option", { hasText: simName }).first().textContent())! });
  await expect(page.locator("#btn-connect")).toBeEnabled();
  await page.click("#btn-connect");
  await expect(page.locator("#status-text")).toContainText(simName, { timeout: 60_000 });
}

export async function disconnect(page: Page) {
  await app(page, "CancelRun").catch(() => undefined);
  await app(page, "Disconnect").catch(() => undefined);
}

/** Counts the Go calls the page makes, by method name (the body of the bridge's request names it). Read it with expectNoCall for "this was not called". */
export function recordCalls(page: Page): Record<string, number> {
  const seen: Record<string, number> = {};
  page.on("request", (r) => {
    const m = /"method":"(\w+)"/.exec(r.postData() ?? "");
    if (m) seen[m[1]] = (seen[m[1]] ?? 0) + 1;
  });
  return seen;
}

const provenRecorders = new WeakSet<object>();

/**
 * Asserts that none of the named Go methods was called, after proving that the recorder sees a call in this very test: it makes one harmless call
 * itself (InitialWorkspace, a read of an environment variable) and waits until the recorder has counted it. A recorder that was never attached, or
 * stopped counting, fails here instead of letting every "nothing happened" assertion pass by default.
 */
export async function expectNoCall(page: Page, calls: Record<string, number>, ...methods: string[]) {
  if (!provenRecorders.has(calls)) {
    const before = calls.InitialWorkspace ?? 0;
    await page.evaluate(() => (window as any).go.main.App.InitialWorkspace());
    await expect.poll(() => calls.InitialWorkspace ?? 0, { message: "the recorder did not count a call it was shown" }).toBeGreaterThan(before);
    provenRecorders.add(calls);
  }
  for (const m of methods) expect(calls[m] ?? 0, `${m} was called`).toBe(0);
}
