import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { connectReason, nextStep, recordReason, runReason, saveReason, workspacePaneText, type FirstRunInput } from "../src/first-run.ts";

const ready: FirstRunInput = { loading: false, workspaceOpen: true, fileOpen: true, dirty: false, devicesLoaded: true, deviceCount: 1, bootedCount: 1, deviceSelected: true, deviceBooted: true, connected: false, running: false, listing: "files", adbNote: "" };
const at = (extra: Partial<FirstRunInput>): FirstRunInput => ({ ...ready, ...extra });

describe("nextStep", () => {
  it("says nothing while the workspace is loading, whatever else is true", () => {
    for (const s of [at({ loading: true, workspaceOpen: false }), at({ loading: true, fileOpen: false }), at({ loading: true, deviceCount: 0 })]) assert.equal(nextStep(s), "");
  });
  it("walks a first run in order: workspace, file, devices, selection, Connect", () => {
    assert.match(nextStep(at({ workspaceOpen: false, fileOpen: false })), /Open your project folder/);
    assert.match(nextStep(at({ fileOpen: false })), /Open a \.probe file/);
    assert.match(nextStep(at({ deviceCount: 0, bootedCount: 0, deviceSelected: false, deviceBooted: false })), /No device found/);
    assert.match(nextStep(at({ bootedCount: 0, deviceSelected: false, deviceBooted: false })), /Every device is shut down/);
    assert.match(nextStep(at({ deviceSelected: false, deviceBooted: false })), /Pick a device/);
    assert.match(nextStep(at({ deviceBooted: false })), /That device is shut down/);
    assert.match(nextStep(ready), /Press Connect/);
  });
  it("goes away once connected with a file open, and before the first device listing", () => {
    assert.equal(nextStep(at({ connected: true })), "");
    assert.equal(nextStep(at({ devicesLoaded: false, deviceCount: 0, bootedCount: 0 })), "");
    // connected without a file: the file is still the next step
    assert.match(nextStep(at({ connected: true, fileOpen: false })), /Open a \.probe file/);
  });
});

describe("nextStep when the folder has no files, or cannot be read", () => {
  it("names the folder, not a file, as the next step", () => {
    assert.match(nextStep(at({ fileOpen: false, listing: "empty" })), /no \.probe files in this folder/);
    assert.match(nextStep(at({ fileOpen: false, listing: "failed" })), /cannot be read/);
    assert.match(nextStep(at({ fileOpen: false, listing: "files" })), /Open a \.probe file/);
    assert.match(nextStep(at({ fileOpen: false, listing: "unknown" })), /Open a \.probe file/);
  });
  it("says nothing of it while loading", () => {
    assert.equal(nextStep(at({ loading: true, fileOpen: false, listing: "failed" })), "");
  });
  it("the pane says the folder cannot be read, with a button", () => {
    assert.deepEqual(workspacePaneText({ loading: false, workspaceOpen: true }, 0, true), { text: "This folder cannot be read (it may have moved or been deleted). Open another folder.", action: true });
    assert.equal(workspacePaneText({ loading: true, workspaceOpen: true }, 0, true), null);
  });
});

describe("the reasons a primary button is disabled", () => {
  it("Run needs a file and a connection, and says which", () => {
    assert.equal(runReason(at({ connected: true })), "");
    assert.equal(runReason(at({ fileOpen: false })), "Open a .probe file first.");
    assert.equal(runReason(at({ fileOpen: false, connected: true })), "Open a .probe file first.");
    assert.equal(runReason(ready), "Connect a device first.");
  });
  it("Run says a run is in progress, whatever else is true, and the reason goes with the run", () => {
    assert.equal(runReason(at({ connected: true, running: true })), "A run is in progress.");
    assert.equal(runReason(at({ running: true })), "A run is in progress."); // even when the connection went away during it
    assert.equal(runReason(at({ connected: true, running: false })), "");
  });
  it("Record needs a connection", () => {
    assert.equal(recordReason(at({ connected: true })), "");
    assert.equal(recordReason(ready), "Connect a device first.");
  });
  it("Save needs a file with changes", () => {
    assert.equal(saveReason(at({ dirty: true })), "");
    assert.equal(saveReason(at({ fileOpen: false })), "Open a .probe file to save.");
    // an unsaved buffer with no file (a recording): Save asks for a name when there is a workspace to put it in
    assert.equal(saveReason(at({ fileOpen: false, dirty: true, workspaceOpen: true })), "");
    assert.match(saveReason(at({ fileOpen: false, dirty: true, workspaceOpen: false })), /Open a workspace first/);
    assert.equal(saveReason(ready), "There are no unsaved changes.");
  });
  it("Connect says to pick a device, or what a shut-down one needs (its own hint first)", () => {
    assert.equal(connectReason(at({ connected: true })), "");
    assert.equal(connectReason(ready), "");
    assert.equal(connectReason(at({ deviceSelected: false, deviceBooted: false })), "Pick a device first.");
    assert.match(connectReason(at({ deviceBooted: false })), /Boot the simulator or emulator first/);
    assert.equal(connectReason(at({ deviceBooted: false }), "Unlock the phone."), "Unlock the phone.");
  });
});

describe("workspacePaneText", () => {
  it("is nothing while loading and when there are files", () => {
    assert.equal(workspacePaneText({ loading: true, workspaceOpen: false }, 0), null);
    assert.equal(workspacePaneText({ loading: false, workspaceOpen: true }, 3), null);
  });
  it("asks for a workspace with an action, and says an open one has no .probe files, with no action", () => {
    assert.deepEqual(workspacePaneText({ loading: false, workspaceOpen: false }, 0), { text: "No workspace open. Pick your project folder (the one with probe.yaml).", action: true });
    assert.deepEqual(workspacePaneText({ loading: false, workspaceOpen: true }, 0), { text: "No .probe files here. Open another folder, or add tests to this one.", action: false });
  });
});

describe("nextStep with the adb note", () => {
  const note = "adb was not found, so Android emulators and phones are not listed.";
  it("adds the note to the no-device and the all-shut-down steps", () => {
    assert.match(nextStep(at({ deviceCount: 0, bootedCount: 0, deviceSelected: false, deviceBooted: false, adbNote: note })), /No device found.*adb was not found/);
    assert.match(nextStep(at({ bootedCount: 0, deviceSelected: false, deviceBooted: false, adbNote: note })), /Every device is shut down.*adb was not found/);
  });
  it("leaves the other steps alone, and adds nothing when adb is found", () => {
    assert.doesNotMatch(nextStep(at({ deviceSelected: false, deviceBooted: false, adbNote: note })), /adb/);
    assert.doesNotMatch(nextStep(at({ deviceCount: 0, bootedCount: 0, deviceSelected: false, deviceBooted: false })), /adb/);
  });
});
