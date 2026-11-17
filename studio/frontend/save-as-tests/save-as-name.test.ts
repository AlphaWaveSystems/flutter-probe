import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { saveAsName } from "../src/save-as-name.ts";

describe("saveAsName", () => {
  it("accepts a plain name, and adds .probe to a name without an extension", () => {
    assert.deepEqual(saveAsName("recorded.probe", []), { ok: true, name: "recorded.probe" });
    assert.deepEqual(saveAsName("  login flow  ", []), { ok: true, name: "login flow.probe" });
    assert.deepEqual(saveAsName("Login.PROBE", []), { ok: true, name: "Login.probe" }); // the extension is written in lower case
  });
  it("refuses an empty name, another extension and a name that is only the extension", () => {
    for (const bad of ["", "   ", "x.txt", "x.probe.bak", ".probe"]) assert.equal(saveAsName(bad, []).ok, false, bad);
  });
  it("refuses a path, a hidden file and the characters a file system refuses", () => {
    for (const bad of ["../x.probe", "a/b.probe", "a\\b.probe", ".hidden.probe", "a:b.probe", "a*.probe", 'a"b.probe', "a|b.probe", "a<b>.probe", "..", "."]) {
      assert.equal(saveAsName(bad, []).ok, false, bad);
    }
  });
  it("refuses control, direction and zero-width characters", () => {
    for (const bad of ["a\u0007b.probe", "a\u202eb.probe", "a\u200bb.probe", "a\u2028b.probe", "a\ufeffb.probe", "a\u00adb.probe"]) assert.equal(saveAsName(bad, []).ok, false, JSON.stringify(bad));
  });
  it("refuses a name that exists, without regard to case, and says Save never replaces a file", () => {
    const r = saveAsName("Login", ["login.probe"]);
    assert.equal(r.ok, false);
    assert.match(r.ok ? "" : r.reason, /already exists/);
    assert.equal(saveAsName("other", ["login.probe"]).ok, true);
  });
  it("refuses a trailing dot or space before .probe, and the names Windows reserves", () => {
    for (const bad of ["a..probe", "a .probe", "a.", "CON", "con.probe", "NUL.probe", "Com1", "lpt9.probe", "aux.x.probe"]) assert.equal(saveAsName(bad, []).ok, false, bad);
    for (const good of ["console", "com10", "lpt", "a.b.probe"]) assert.equal(saveAsName(good, []).ok, true, good);
  });
  it("a name that is the same as an existing one in another case or another Unicode normalisation is refused", () => {
    assert.equal(saveAsName("\u00e9cran", ["e\u0301cran.probe"]).ok, false); // NFC asked, NFD on disk
    assert.equal(saveAsName("e\u0301cole", ["\u00e9cole.probe"]).ok, false); // NFD asked, NFC on disk
    assert.equal(saveAsName("\u00c9te", ["e\u0301TE.probe"]).ok, false);
    assert.equal(saveAsName("ecole", ["\u00e9cole.probe"]).ok, true); // a different name
  });
  it("a long name is refused by code points, not by UTF-16 units", () => {
    assert.equal(saveAsName("\u{1F600}".repeat(50) + ".probe", []).ok, true); // 50 code points plus the extension
    assert.equal(saveAsName("x".repeat(101), []).ok, false);
  });
});
