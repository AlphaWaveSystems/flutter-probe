import { expect, Page, test } from "@playwright/test";
import * as http from "node:http";
import * as net from "node:net";
import { app, openStudio, workspace } from "./helpers";
import { netEmail, netHome, netSecret, netToken } from "./explain-seed";

const split = (s: string, ch: string) => [...s].join(ch);

// The Send request panel, against fake backends on 127.0.0.1 (the headless build refuses local addresses except for the host named in
// PROBE_STUDIO_TEST_ALLOW_LOCAL). No real service is called.

interface Backend {
  url: string;
  host: string;
  port: number;
  hits: () => number;
  seen: () => { method: string; url: string; auth: string; body: string }[];
  close: () => Promise<void>;
}

async function backend(other?: Backend): Promise<Backend> {
  let hits = 0;
  const seen: { method: string; url: string; auth: string; body: string }[] = [];
  const server = http.createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      hits++;
      seen.push({ method: req.method ?? "", url: req.url ?? "", auth: String(req.headers["authorization"] ?? ""), body: Buffer.concat(chunks).toString("utf8") });
      const path = (req.url ?? "").split("?")[0];
      if (path === "/ok") {
        res.setHeader("Content-Type", "application/json");
        res.setHeader("Set-Cookie", `sid=${netSecret}; HttpOnly`);
        res.setHeader("X-Request-Id", "req-9");
        res.setHeader("X-Split", split(netToken, "\u00AD")); // a header can only carry Latin-1
        res.end(JSON.stringify({ ok: true, password: netSecret, note: '<img src=x onerror="window.__sendXss=1"><script>window.__sendXss=2</script>', email: netEmail, token: netToken, soft: split(netToken, "\u00AD"), glyph: split(netEmail, "\uE000"), home: split(netHome, "\u2065") }));
      } else if (path === "/away" && other) {
        res.statusCode = 302;
        res.setHeader("Location", `${other.url}/steal?token=${netSecret}`);
        res.end();
      } else if (path === "/echo") {
        res.setHeader("X-Seen", String(req.headers["x-api-key"] ?? ""));
        res.setHeader("Content-Type", "text/plain");
        res.end(`key=${req.headers["x-api-key"]} auth=${req.headers["authorization"]} body=${Buffer.concat(chunks).toString("utf8")} soft=${split(String(req.headers["x-api-key"]), "\u2065")} kept=hello`);
      } else if (path === "/slow") {
        setTimeout(() => res.end("slow"), 900);
      } else if (path === "/hop") {
        res.statusCode = 302;
        res.setHeader("Location", "/ok");
        res.end();
      } else if (path === "/big") {
        res.setHeader("Content-Type", "text/plain");
        res.end("x ".repeat(3 * 1024 * 1024));
      } else if (path === "/bin") {
        res.setHeader("Content-Type", "image/png");
        res.end(Buffer.from([0x89, 0x50, 0x4e, 0x47, 0, 0]));
      } else {
        res.statusCode = 404;
        res.end("not found");
      }
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as net.AddressInfo).port;
  return { url: `http://127.0.0.1:${port}`, host: "127.0.0.1", port, hits: () => hits, seen: () => seen, close: () => new Promise((resolve) => server.close(() => resolve())) };
}

async function openSend(page: Page) {
  await openStudio(page);
  await page.click("#tab-network");
  await page.click("#btn-net-send");
  await expect(page.locator("#send-panel")).toBeVisible();
  await expect(page.locator("#net-body")).toBeHidden();
}

async function fill(page: Page, url: string, opts: { method?: string; headers?: string; body?: string } = {}) {
  await page.selectOption("#send-method", opts.method ?? "GET");
  await page.fill("#send-url", url);
  await page.fill("#send-headers", opts.headers ?? "");
  await page.fill("#send-body", opts.body ?? "");
}

const overlay = (page: Page) => page.locator("#send-allow-overlay");

test.describe("Send request (fake backends on 127.0.0.1)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");

  test("sr-01 the first request to a host asks, shows where it leads, and sends nothing until Allow", async ({ page }) => {
    const b = await backend();
    try {
      await openSend(page);
      await fill(page, `${b.url}/ok?token=${netSecret}&id=7`, { headers: `Authorization: Bearer ${netSecret}\nX-Note: hello` });
      await page.click("#btn-send");
      await expect(overlay(page)).toBeVisible();
      await expect(page.locator("#send-allow-title")).toHaveText(`Send requests to 127.0.0.1:${b.port}?`);
      await expect(page.locator("#send-allow-lines")).toContainText("this machine");
      await expect(page.locator("#send-allow-warning")).toBeVisible(); // the stronger warning for an address on this machine
      expect(b.hits()).toBe(0);
      await page.click("#btn-send-allow-cancel");
      await expect(overlay(page)).toBeHidden();
      await expect(page.locator("#send-status")).toContainText("Nothing was sent");
      expect(b.hits()).toBe(0);

      await page.click("#btn-send");
      await page.click("#btn-send-allow-ok");
      await expect(page.locator("#send-result")).toContainText("200");
      expect(b.hits()).toBe(1);
      expect(b.seen()[0].auth).toBe(`Bearer ${netSecret}`); // the server got what was typed

      // The second request to the same host and port goes without a dialog.
      await page.click("#btn-send");
      await expect(overlay(page)).toBeHidden();
      await expect.poll(() => b.hits()).toBe(2);
    } finally {
      await b.close();
    }
  });

  test("sr-02 the answer is redacted, the typed secrets are not echoed, and markup stays text", async ({ page }) => {
    const b = await backend();
    try {
      await openSend(page);
      await fill(page, `${b.url}/ok?token=${netSecret}&id=7`, { headers: `Authorization: Bearer ${netSecret}\nX-Note: hello` });
      await page.click("#btn-send");
      await page.click("#btn-send-allow-ok");
      const result = page.locator("#send-result");
      await expect(result).toContainText("200");
      await expect(result).toContainText("from this machine");
      await expect(result).toContainText(`GET ${b.url}/ok?token=[redacted]&id=[redacted]`);
      await expect(result.locator(".net-hname", { hasText: /^Authorization$/ }).locator("xpath=following-sibling::span[1]")).toHaveText("[redacted]");
      await expect(result.locator(".net-hname", { hasText: /^X-Note$/ }).locator("xpath=following-sibling::span[1]")).toHaveText("hello");
      await expect(result.locator(".net-hname", { hasText: /^Set-Cookie$/ }).locator("xpath=following-sibling::span[1]")).toHaveText("[redacted]");
      await expect(result).toContainText('"password": "[redacted]"');
      await expect(result).toContainText("<img src=x"); // markup is text
      await expect(result.locator("img, script")).toHaveCount(0);
      expect(await page.evaluate(() => (window as any).__sendXss)).toBeUndefined();
      const all = await page.locator("body").innerText();
      for (const bad of [netSecret, netEmail, "jane.doe", netToken.slice(4, 20), "alice"]) expect(all, bad).not.toContain(bad); // also when split by hidden or glyphless characters
    } finally {
      await b.close();
    }
  });

  test("sr-03 a redirect to another host is shown and not followed", async ({ page }) => {
    const other = await backend();
    const b = await backend(other);
    try {
      await openSend(page);
      await fill(page, `${b.url}/away`);
      await page.click("#btn-send");
      await page.click("#btn-send-allow-ok");
      await expect(page.locator("#send-result")).toContainText("was not followed");
      await expect(page.locator("#send-result")).toContainText(`127.0.0.1:${other.port}`);
      await expect(page.locator("#send-result")).toContainText("302");
      expect(other.hits()).toBe(0);
      expect(await page.locator("body").innerText()).not.toContain(netSecret);

      // Following it is a new request, and the new host is asked about.
      await fill(page, `${other.url}/ok`);
      await page.click("#btn-send");
      await expect(overlay(page)).toBeVisible();
      await expect(page.locator("#send-allow-title")).toContainText(`127.0.0.1:${other.port}`);
      await page.click("#btn-send-allow-cancel");
      expect(other.hits()).toBe(0);

      // A redirect on the same origin is followed and listed.
      await fill(page, `${b.url}/hop`);
      await page.click("#btn-send");
      await expect(page.locator("#send-result")).toContainText("Redirects followed");
      await expect(page.locator("#send-result")).toContainText("302 →");
    } finally {
      await b.close();
      await other.close();
    }
  });

  test("sr-04 a request Studio will not make says why, and sends nothing", async ({ page }) => {
    const b = await backend();
    try {
      await openSend(page);
      for (const [url, headers, want] of [
        ["file:///etc/passwd", "", /valid http or https address/],
        [`http://user:${netSecret}@127.0.0.1:${b.port}/ok`, "", /user name or password/],
        [`ftp://127.0.0.1:${b.port}/`, "", /only http and https/],
        ["not a url", "", /valid http or https address/],
      ] as const) {
        await fill(page, url, { headers });
        await page.click("#btn-send");
        await expect(page.locator("#send-status")).toContainText(want);
        await expect(overlay(page)).toBeHidden();
        expect(await page.locator("#send-status").innerText()).not.toContain(netSecret);
      }
      // A header Studio sets itself, once the host is allowed.
      await fill(page, `${b.url}/ok`, { headers: "Host: evil.example" });
      await page.click("#btn-send");
      await page.click("#btn-send-allow-ok");
      await expect(page.locator("#send-status")).toContainText("header is not allowed");
      await fill(page, `${b.url}/ok`, { headers: "this is not a header" });
      await page.click("#btn-send");
      await expect(page.locator("#send-status")).toContainText('Line 1 is not "Name: value"');
      await fill(page, `${b.url}/ok`, { method: "GET", body: "x" });
      await page.click("#btn-send");
      await expect(page.locator("#send-status")).toContainText("sends no body");
      expect(b.hits()).toBe(0);
    } finally {
      await b.close();
    }
  });

  test("sr-05 a large answer is cut and a binary one is summarised", async ({ page }) => {
    const b = await backend();
    try {
      await openSend(page);
      await fill(page, `${b.url}/big`);
      await page.click("#btn-send");
      await page.click("#btn-send-allow-ok");
      await expect(page.locator("#send-result")).toContainText("longer than 1 MB");
      expect((await page.locator("#send-result pre").innerText()).length).toBeLessThanOrEqual(4200);
      await fill(page, `${b.url}/bin`);
      await page.click("#btn-send");
      await expect(page.locator("#send-result")).toContainText("binary");
      await expect(page.locator("#send-result pre")).toHaveCount(0);
    } finally {
      await b.close();
    }
  });

  test("sr-06 Escape is Cancel, nothing is stored in the browser, and only Studio is asked", async ({ page }) => {
    const b = await backend();
    const hosts = new Set<string>();
    page.on("request", (r) => {
      if (/^https?:/.test(r.url())) hosts.add(new URL(r.url()).host);
    });
    try {
      await openSend(page);
      await fill(page, `${b.url}/ok?token=${netSecret}`, { method: "POST", headers: `Authorization: Bearer ${netSecret}`, body: netSecret });
      await page.click("#btn-send");
      await expect(overlay(page)).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(overlay(page)).toBeHidden();
      expect(b.hits()).toBe(0);
      await page.click("#btn-send");
      await page.click("#btn-send-allow-ok");
      await expect(page.locator("#send-result")).toContainText("200");
      const base = new URL(page.url()).host;
      expect([...hosts].filter((h) => h !== base)).toEqual([]); // the page itself never calls the backend
      const stored = await page.evaluate(() => JSON.stringify({ l: { ...localStorage }, s: { ...sessionStorage } }));
      for (const bad of [netSecret, "127.0.0.1", "Authorization"]) expect(stored, bad).not.toContain(bad);
      await page.click("#btn-send-back");
      await expect(page.locator("#net-body")).toBeVisible();
    } finally {
      await b.close();
    }
  });

  test("sr-07 the guards are in Studio, not only in the window: an unchecked or unallowed host is refused through the bridge", async ({ page }) => {
    const b = await backend();
    try {
      await openStudio(page);
      await expect(app(page, "SendRequest", { method: "GET", url: `${b.url}/ok`, headers: [], body: "" })).rejects.toThrow(/not been allowed/);
      expect(b.hits()).toBe(0);
      // An allow needs a check first: what is allowed is what was shown.
      await expect(app(page, "AllowHost", `127.0.0.1:${b.port}`)).rejects.toThrow(/not checked/);
      const info: any = await app(page, "CheckTarget", `${b.url}/ok`);
      expect(info.local).toBe(true);
      expect(info.allowed).toBe(false);
      expect(info.key).toBe(`127.0.0.1:${b.port}`);
      expect(b.hits()).toBe(0);
      await expect(app(page, "AllowHost", "no-port")).rejects.toThrow();
      await expect(app(page, "AllowHost", "127.0.0.1:0")).rejects.toThrow();
      // A name that is not on the test list is refused outright in this build: it cannot even be allowed, and a request to it is refused.
      const other: any = await app(page, "CheckTarget", `http://localhost:${b.port}/ok`);
      expect(other.refused).toBeTruthy();
      await expect(app(page, "AllowHost", other.key)).rejects.toThrow();
      await expect(app(page, "SendRequest", { method: "GET", url: `http://localhost:${b.port}/ok`, headers: [], body: "" })).rejects.toThrow(/not been allowed/);
      expect(b.hits()).toBe(0);
    } finally {
      await b.close();
    }
  });

  test("sr-08 pressing Enter twice sends once: the window does not start a second request", async ({ page }) => {
    const b = await backend();
    try {
      await openSend(page);
      await fill(page, `${b.url}/ok`);
      await page.click("#btn-send");
      await page.click("#btn-send-allow-ok");
      await expect(page.locator("#send-result")).toContainText("200");
      await fill(page, `${b.url}/slow`);
      let calls = 0;
      page.on("request", (r) => {
        if ((r.postData() ?? "").includes("SendRequest")) calls++; // what the window asks Studio to do, not what Studio then refuses
      });
      // The button is disabled while a request runs, so the keyboard is the way in: Enter in the address field, twice.
      await page.locator("#send-url").press("Enter");
      await page.locator("#send-url").press("Enter");
      await expect(page.locator("#send-result")).toContainText("200", { timeout: 10000 });
      expect(calls).toBe(1);
      expect(b.seen().filter((r) => r.url === "/slow")).toHaveLength(1);
    } finally {
      await b.close();
    }
  });

  test("sr-09 what was typed is hidden wherever the answer echoes it, under a name the redactor does not know", async ({ page }) => {
    const b = await backend();
    try {
      await openSend(page);
      await fill(page, `${b.url}/echo?q=urlquery4321abcd`, { method: "POST", headers: "X-Api-Key: plainvalueqq1234\nAuthorization: Token plainauthvalue5678\nX-Note: hello", body: '{"note":"bodysecret9z8y7x6w"}' });
      await page.click("#btn-send");
      await page.click("#btn-send-allow-ok");
      await expect(page.locator("#send-result")).toContainText("200");
      const result = page.locator("#send-result");
      await expect(result).toContainText("kept=hello"); // a value that was not a credential is still readable
      const all = await page.locator("body").innerText();
      for (const bad of ["plainvalueqq1234", "plainauthvalue5678", "bodysecret9z8y7x6w", "urlquery4321abcd", "plainvalue", "plainauth", "bodysecret"]) expect(all, bad).not.toContain(bad);
      await expect(result.locator(".net-hname", { hasText: /^X-Seen$/ }).locator("xpath=following-sibling::span[1]")).toHaveText("[redacted]");
      expect(b.seen().filter((r) => r.url.startsWith("/echo"))[0].auth).toBe("Token plainauthvalue5678"); // the server got what was typed
    } finally {
      await b.close();
    }
  });

  test("sr-10 Save as step inserts a call line into the open file: placeholders for what must not be written, no headers, no request", async ({ page }) => {
    const b = await backend();
    try {
      await openSend(page);
      await expect(page.locator("#send-save-note")).toContainText("Headers are not saved");
      await fill(page, `${b.url}/v1/reset/${netSecret}?token=${netSecret}&id=7`, { method: "POST", headers: `Authorization: Bearer ${netSecret}\nX-Note: hello`, body: JSON.stringify({ name: "Jane", password: netSecret, email: netEmail }) });
      await page.click("#btn-send-save");
      await expect(page.locator("#send-status")).toContainText("Inserted into the open file");
      await expect(page.locator("#send-status")).toContainText("2 headers were not saved");
      await expect(page.locator("#send-status")).toContainText("TOKEN");
      const editor = page.locator("#editor .view-lines");
      await expect(editor).toContainText("call POST");
      await expect(editor).toContainText("${TOKEN}");
      await expect(editor).toContainText("${PASSWORD}");
      const text = await editor.innerText();
      for (const bad of [netSecret, netEmail, "jane.doe", "Authorization", "Bearer", "X-Note", "hunter2"]) expect(text, bad).not.toContain(bad);
      expect(b.hits()).toBe(0); // saving asks nobody: no request, and no dialog
      await expect(overlay(page)).toBeHidden();
      // What the call step cannot say is not saved, and the editor does not change.
      const before = await editor.innerText();
      await fill(page, `${b.url}/ok`, { method: "PATCH", body: "{}" });
      await page.click("#btn-send-save");
      await expect(page.locator("#send-status")).toContainText("PATCH");
      await expect(page.locator("#send-status")).toContainText("cannot be saved as a step");
      expect(await editor.innerText()).toBe(before);
      // Nothing is stored in the browser.
      const stored = await page.evaluate(() => JSON.stringify({ l: { ...localStorage }, s: { ...sessionStorage } }));
      expect(stored).not.toContain("call POST");
    } finally {
      await b.close();
    }
  });
});
