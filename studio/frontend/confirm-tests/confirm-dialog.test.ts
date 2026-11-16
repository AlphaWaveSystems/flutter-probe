import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { CONFIRM_ARM_MS, confirmCopy, keyAction } from "../src/confirm-copy.ts";

describe("confirmCopy", () => {
  it("names the app and the device, and says nothing is deleted unless Clear data is pressed", () => {
    const c = confirmCopy({ id: "x", kind: "clear-app-data", appId: "com.example.app", deviceName: "Pixel 8" });
    assert.ok(c);
    assert.equal(c.app, "com.example.app");
    assert.equal(c.device, "Pixel 8");
    assert.match(c.warning, /cannot be undone/);
    assert.match(c.warning, /unless you press Clear data/);
    assert.equal(c.appLabel, "App (from probe.yaml)");
    assert.equal(c.cancel, "Cancel");
    assert.equal(c.danger, "Clear data");
  });
  it("puts a name in as it is (it is shown as text) and fills a missing one", () => {
    const c = confirmCopy({ id: "x", kind: "clear-app-data", appId: "<img src=x onerror=1>", deviceName: "" });
    assert.equal(c?.app, "<img src=x onerror=1>");
    assert.equal(c?.device, "(unknown device)");
    assert.equal(confirmCopy({ id: "x", kind: "clear-app-data", appId: "", deviceName: "d" })?.app, "(unknown app)");
  });
  it("has no question for a kind it does not know", () => {
    assert.equal(confirmCopy({ id: "x", kind: "format-disk", appId: "a", deviceName: "d" }), null);
  });
});

describe("arming", () => {
  it("Clear data waits half a second before it can be pressed", () => {
    assert.equal(CONFIRM_ARM_MS, 500);
  });
});

describe("keyAction", () => {
  it("Escape is always a no", () => {
    for (const f of ["cancel", "danger", "other"] as const) assert.equal(keyAction("Escape", false, f), "deny");
  });
  it("Enter on Clear data does nothing; Enter on Cancel is left to the button", () => {
    assert.equal(keyAction("Enter", false, "danger"), "ignore");
    assert.equal(keyAction("Enter", false, "cancel"), "default");
  });
  it("Space on Clear data presses it: a keyboard user can say yes on purpose", () => {
    assert.equal(keyAction(" ", false, "danger"), "press");
  });
  it("Tab stays inside the dialog, in both directions", () => {
    assert.equal(keyAction("Tab", false, "cancel"), "trap-forward");
    assert.equal(keyAction("Tab", true, "cancel"), "trap-back");
  });
  it("other keys are left alone", () => {
    assert.equal(keyAction("a", false, "cancel"), "default");
    assert.equal(keyAction(" ", false, "cancel"), "default");
    assert.equal(keyAction("a", false, "danger"), "default");
  });
});
