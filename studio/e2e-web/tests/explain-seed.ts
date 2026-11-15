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
