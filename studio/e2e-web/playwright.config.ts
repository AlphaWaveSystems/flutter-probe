import { defineConfig } from "@playwright/test";
import * as path from "node:path";

// Studio runs in headless mode (PROBE_STUDIO_HEADLESS=1): the real frontend and Go bindings over HTTP,
// no native window. scripts/studio-e2e-web.sh builds Studio, prepares a workspace copy (and, for the
// device tests, a named simulator with the fixture app) and exports:
//   STUDIO_BIN        the Studio binary
//   STUDIO_WORKSPACE  a copy of the fixture workspace (tests may change it)
//   STUDIO_E2E_UDID   optional: a booted device running the fixture app (device tests are skipped without it)
const port = process.env.STUDIO_E2E_PORT ?? "34890";
const out = process.env.STUDIO_E2E_OUT ?? path.resolve(__dirname, "../../reports/studio-e2e-web");

export default defineConfig({
  testDir: "./tests",
  timeout: 120_000,
  expect: { timeout: 15_000 },
  workers: 1, // one Studio process holds one connection to one device
  fullyParallel: false,
  retries: 0,
  reporter: [["list"], ["html", { outputFolder: path.join(out, "html"), open: "never" }], ["json", { outputFile: path.join(out, "report.json") }]],
  outputDir: path.join(out, "artifacts"),
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    headless: process.env.STUDIO_E2E_HEADED !== "1", // STUDIO_E2E_HEADED=1 to watch; the window then opens in the browser, not Studio
    actionTimeout: 15_000,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    viewport: { width: 1480, height: 920 },
  },
  webServer: {
    command: process.env.STUDIO_BIN ?? "../build/bin/studio-headless",
    url: `http://127.0.0.1:${port}/`,
    reuseExistingServer: false,
    timeout: 60_000,
    env: {
      PROBE_STUDIO_HEADLESS: "1",
      PROBE_STUDIO_HEADLESS_PORT: port,
      PROBE_STUDIO_WORKSPACE: process.env.STUDIO_WORKSPACE ?? "",
      PROBE_STUDIO_DEVICE_VIEW: process.env.STUDIO_E2E_DEVICE_VIEW ?? "", // "1" runs Studio with the shared device view
    },
    stdout: "pipe",
    stderr: "pipe",
  },
});
