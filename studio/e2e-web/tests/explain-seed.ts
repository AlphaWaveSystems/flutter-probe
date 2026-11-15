// The run Studio starts with in the browser suite (PROBE_STUDIO_SEED_RUN, headless only): two failed tests whose error text is hostile on
// purpose, a passed test, and a skipped test that carries error text and must never be sent. Credential-shaped text is built from pieces.

export const seedToken = "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789";
export const seedEmail = "jane.doe@example.com";
export const seedHome = "/Users/alice/app";
export const seedFailed = 2; // failed tests with error text: what the provider must be asked about

/** A hidden character in the middle of s. */
function split(s: string, ch: string): string {
  const h = Math.floor(s.length / 2);
  return s.slice(0, h) + ch + s.slice(h);
}

export function seedRun() {
  return [
    {
      name: "sign in",
      file: "probe-tests/login.probe",
      passed: false,
      skipped: false,
      durationMs: 12,
      error:
        `no widget with text "Sign in" <img src=x onerror="window.__explainXss=1"> ` +
        `${split(seedToken, "⵿")} ${split(seedEmail, "")} ${split(seedHome, "⁥")} \u001b[31m`,
    },
    { name: "<b>sign out</b>", file: "/Users/alice/other/logout.probe", passed: false, skipped: false, durationMs: 9, error: 'timed out waiting for "Sign out"' },
    { name: "checkout", file: "probe-tests/checkout.probe", passed: true, skipped: false, durationMs: 30 },
    { name: "skipped one", file: "probe-tests/skip.probe", passed: false, skipped: true, durationMs: 0, error: "skipped-error-text-must-not-be-sent" },
  ];
}

// The request log the browser suite starts with (PROBE_STUDIO_SEED_EXCHANGES, headless only): a secret in every position the agent
// records, hostile text, a binary body, a body over 1 MB, a failed request and ten thousand headers on one. The values are built from
// pieces. A marker, netSecret, is the value that must never be on the page.

export const netSecret = "hunter2-Xk9pq";
export const netToken = "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789";
export const netEmail = "jane.doe@example.com";
export const netHome = "/Users/alice/app";

export function seedExchanges() {
  const manyHeaders: Record<string, string> = {};
  for (let i = 0; i < 10000; i++) manyHeaders[`X-Many-${String(i).padStart(5, "0")}`] = "v";
  return {
    epoch: "e2e-epoch",
    latest: 7,
    count: 7,
    entries: [
      {
        seq: 1, method: "GET", url: `https://user:${netSecret}@api.example.test/v1/me?token=${netSecret}&id=7#access_token=${netSecret}`, status: 200, durationMs: 41,
        requestHeaders: { Authorization: `Bearer ${netSecret}`, Cookie: `sid=${netSecret}`, "Content-Type": "application/json", "X_Api_Key": netSecret, Accept: "application/json" },
        responseHeaders: { "Content-Type": "application/json", "Set-Cookie": `sid=${netSecret}; HttpOnly`, "X-Request-Id": "req-1" },
        requestBody: "", responseBody: JSON.stringify({ name: "Jane", email: netEmail, home: netHome, password: netSecret, nested: { apiKey: netSecret, items: [{ token: netSecret }, { ok: 1 }] } }),
        requestBytes: 0, responseBytes: 200,
      },
      {
        seq: 2, method: "POST", url: "https://api.example.test/v1/login", status: 401, durationMs: 88,
        requestHeaders: { "Content-Type": "application/x-www-form-urlencoded" }, responseHeaders: { "Content-Type": "text/html" },
        requestBody: `user=jane&password=${netSecret}&next=%2Fhome%2Falice`,
        responseBody: `<img src=x onerror="window.__netXss=1"><script>window.__netXss=2</script> ${netToken}`,
        requestBytes: 60, responseBytes: 120,
      },
      {
        seq: 3, method: "GET", url: "https://cdn.example.test/logo.png", status: 200, durationMs: 12,
        requestHeaders: {}, responseHeaders: { "Content-Type": "image/png" }, requestBody: "", responseBody: "\u0089PNG\r\n\u001a\n\u0000\u0000", requestBytes: 0, responseBytes: 4096,
      },
      {
        seq: 4, method: "GET", url: "https://api.example.test/v1/export", status: 200, durationMs: 900,
        requestHeaders: {}, responseHeaders: { "Content-Type": "text/plain" }, requestBody: "", responseBody: "", requestBytes: 0, responseBytes: 3 * 1024 * 1024,
      },
      {
        seq: 5, method: "GET", url: `https://down.example.test/health?key=${netSecret}`, status: 0, durationMs: 3000, error: `connection refused for https://down.example.test/health?key=${netSecret} (${netToken})`,
        requestHeaders: {}, responseHeaders: {}, requestBody: "", responseBody: "", requestBytes: 0, responseBytes: 0,
      },
      {
        seq: 6, method: "PUT", url: "https://api.example.test/v1/many", status: 204, durationMs: 7,
        requestHeaders: manyHeaders, responseHeaders: {}, requestBody: "", responseBody: "", requestBytes: 0, responseBytes: 0,
      },
      {
        seq: 7, method: "GET", url: "https://api.example.test/v1/mocked", status: 200, durationMs: 1, mocked: true,
        requestHeaders: {}, responseHeaders: {}, requestBody: "", responseBody: "{\"mock\":true}", requestBytes: 0, responseBytes: 13,
      },
    ],
  };
}
