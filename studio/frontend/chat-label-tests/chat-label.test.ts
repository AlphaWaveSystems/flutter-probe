import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { chatLabelRefresher, labelText, type ChatLabelParts } from "../src/chat-label.ts";

const local: ChatLabelParts = { prefix: "", model: "qwen3-coder:30b", suffix: ", on this machine" };

describe("labelText", () => {
  it("joins the three parts into the whole line", () => {
    assert.equal(labelText(local), "qwen3-coder:30b, on this machine");
    assert.equal(labelText({ prefix: "Anthropic, ", model: "claude-x", suffix: "" }), "Anthropic, claude-x");
    assert.equal(labelText({ prefix: "AI is off", model: "", suffix: "" }), "AI is off");
  });
});

describe("chatLabelRefresher", () => {
  it("shows the label it fetched", async () => {
    const shown: Array<ChatLabelParts | null> = [];
    await chatLabelRefresher(async () => local, (p) => shown.push(p))();
    assert.deepEqual(shown, [local]);
  });

  it("leaves the header blank when a refresh fails, and does not keep the last label", async () => {
    const shown: Array<ChatLabelParts | null> = [];
    let fail = false;
    const refresh = chatLabelRefresher(async () => {
      if (fail) throw new Error("the bridge is down");
      return local;
    }, (p) => shown.push(p));
    await refresh();
    fail = true;
    await refresh();
    assert.deepEqual(shown, [local, null]); // the failed refresh blanks it
  });

  it("ignores an older refresh that finishes after a newer one", async () => {
    let finishFirst!: (p: ChatLabelParts) => void;
    const queue = [new Promise<ChatLabelParts>((r) => (finishFirst = r)), Promise.resolve({ prefix: "AI is off", model: "", suffix: "" })];
    const shown: Array<ChatLabelParts | null> = [];
    const refresh = chatLabelRefresher(() => queue.shift()!, (p) => shown.push(p));
    const a = refresh();
    const b = refresh();
    await b;
    finishFirst(local);
    await a;
    assert.deepEqual(shown.map((p) => (p ? labelText(p) : null)), ["AI is off"]);
  });
});
