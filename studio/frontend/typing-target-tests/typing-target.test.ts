import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { isTextEntry } from "../src/typing-target.ts";

describe("isTextEntry", () => {
  it("is true for the elements that take typed text", () => {
    assert.equal(isTextEntry({ tagName: "TEXTAREA" }), true); // the chat input, the send panel's body, Monaco's own input
    assert.equal(isTextEntry({ tagName: "textarea" }), true);
    assert.equal(isTextEntry({ tagName: "SELECT" }), true);
    assert.equal(isTextEntry({ tagName: "DIV", isContentEditable: true }), true);
    for (const type of ["", "text", "search", "url", "tel", "email", "password", "number", "TEXT"]) assert.equal(isTextEntry({ tagName: "INPUT", type }), true, type);
  });
  it("is false for the elements that do not", () => {
    assert.equal(isTextEntry(null), false);
    assert.equal(isTextEntry(undefined), false);
    assert.equal(isTextEntry({}), false);
    assert.equal(isTextEntry({ tagName: "BODY" }), false);
    assert.equal(isTextEntry({ tagName: "BUTTON" }), false);
    assert.equal(isTextEntry({ tagName: "DIV" }), false);
    for (const type of ["checkbox", "radio", "button", "submit", "range", "color", "file"]) assert.equal(isTextEntry({ tagName: "INPUT", type }), false, type);
  });
});
