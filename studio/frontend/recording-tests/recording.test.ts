import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

// The Record and Save handlers live in main.ts, which needs the DOM and the Wails bindings, so these tests read its source and check the
// wiring the bug depended on: the Stop branch keeps the header, refreshes Run, and Save with no file asks for a name.
const main = readFileSync(new URL("../src/main.ts", import.meta.url), "utf8");
const stopBranch = main.slice(main.indexOf("const script = await StopRecording()"), main.indexOf("// ---- Boot"));

describe("Record and Stop in main.ts", () => {
  it("Stop puts the test header in front of the recorded steps", () => {
    assert.equal(stopBranch.includes("model.setValue(recordedScript(recordingHead, script))"), true);
  });

  it("Stop refreshes the Run button, because the buffer has no file yet", () => {
    assert.equal(/finally \{\s*setRecording\(false\);\s*updateRunButton\(\);/.test(stopBranch), true);
  });

  it("Save with no file open asks for a name instead of doing nothing", () => {
    assert.equal(/async function saveCurrent\(\) \{\s*if \(!currentPath\) \{\s*await saveAs\(\);/.test(main), true);
  });
});

describe("recordingBuffer", () => {
  it("keeps the test header in front of the recorded steps", async () => {
    const { recordingBuffer } = await import("../src/recording.ts");
    const buf = recordingBuffer("12:00", "  tap \"Pay\"\n  type \"42\" into \"Amount\"\n");
    assert.equal(buf, "# Recorded on 12:00\n\ntest \"recorded flow\"\n  open the app\n  tap \"Pay\"\n  type \"42\" into \"Amount\"\n");
  });
});
