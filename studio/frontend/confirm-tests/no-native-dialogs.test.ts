import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

// The window's webview does not show a native confirm(), alert() or prompt(): a confirm answers no without a word, so the click that asked it does
// nothing. A question is asked in the page (see confirm-dialog.ts and discard-dialog.ts), with the safe defaults of the clear-data question.
// This fails on the first use of one in the sources, whatever the line is for.

const root = join(import.meta.dirname, "..");
const src = join(root, "src");

/** Every .ts file under src (probescript/ and device-view/ too) and the page itself, whose inline script could call one as well. */
function sources(): string[] {
  const out: string[] = [join(root, "index.html")];
  const walk = (dir: string) => {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      if (e.isDirectory()) walk(join(dir, e.name));
      else if (e.name.endsWith(".ts")) out.push(join(dir, e.name));
    }
  };
  walk(src);
  return out;
}

/** A source line without its comment and its string contents, so a word in a message is not a call. */
function code(line: string): string {
  return line.replace(/\/\/.*$/, "").replace(/(["'`])(?:\\.|(?!\1).)*\1/g, '""');
}

export function nativeDialogCalls(text: string): string[] {
  const found: string[] = [];
  text.split("\n").forEach((raw, i) => {
    const c = code(raw);
    for (const m of c.matchAll(/(^|[^.\w$])(?:window\.)?(confirm|alert|prompt)\s*\(([^)]*)/g)) {
      if (/^\s*\w+\s*\??:/.test(m[3])) continue; // `confirm(p: Preview)`: a method declaration, not a call
      found.push(`line ${i + 1}: ${raw.trim()}`);
    }
    if (/\bwindow\.(confirm|alert|prompt)\b/.test(c) && !found.some((f) => f.startsWith(`line ${i + 1}:`))) found.push(`line ${i + 1}: ${raw.trim()}`);
  });
  return found;
}

describe("no native dialogs", () => {
  it("the sources use no confirm(), alert() or prompt()", () => {
    const bad: string[] = [];
    for (const f of sources()) {
      for (const hit of nativeDialogCalls(readFileSync(f, "utf8"))) bad.push(`${relative(root, f)} ${hit}`);
    }
    assert.deepEqual(bad, [], "the window shows no native dialog: ask in the page (discard-dialog.ts is the pattern)");
  });
  it("the scan reaches the subfolders and the page", () => {
    const names = sources().map((f) => relative(root, f));
    assert.ok(names.includes("index.html"), "index.html");
    assert.ok(names.some((n) => n.startsWith(join("src", "probescript") + "/")), "src/probescript");
    assert.ok(names.includes(join("src", "main.ts")), "src/main.ts");
  });
  it("the check sees a native call, and ignores declarations, methods and text", () => {
    assert.equal(nativeDialogCalls('if (!confirm("Discard?")) return;').length, 1);
    assert.equal(nativeDialogCalls('window.alert("x")').length, 1);
    assert.equal(nativeDialogCalls("const ok = prompt('name');").length, 1);
    assert.equal(nativeDialogCalls("  confirm(p: ExplainPreview): Promise<boolean>;").length, 0);
    assert.equal(nativeDialogCalls("const sent = await deps.confirm(p);").length, 0);
    assert.equal(nativeDialogCalls('toast("Do not confirm(this)")').length, 0);
    assert.equal(nativeDialogCalls("// was confirm(…) before").length, 0);
  });
});
