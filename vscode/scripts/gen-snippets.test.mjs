// Drift tests for the generated skeleton snippets (scripts/gen-snippets.mjs). They fail when the docs change and the file is not
// regenerated, when a skeleton does not reproduce its documented example, and when a documented command with an argument has
// neither a hand-written snippet nor a skeleton.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { DICTIONARY_PATH, parseDictionary } from "../../studio/frontend/scripts/gen-probescript-dictionary.mjs";
import { exampleSnippet, hasTabStop, splitExamples } from "./gen-keywords.mjs";
import { HAND_WRITTEN_PATH, SNIPPETS_OUTPUT_PATH, buildSnippetOutputs, isCovered, plainWords } from "./gen-snippets.mjs";

const here = dirname(fileURLToPath(import.meta.url));

// withDefaults returns a snippet with each tab stop replaced by its default text, which is what it inserts when the user leaves a
// tab stop as it is. Escapes are read the way VS Code reads them.
function withDefaults(snippet) {
  return snippet.replace(/\$\{\d+:((?:\\[\\$}]|[^}\\])*)\}|\\([\\$}])/g, (_, inner, escaped) =>
    inner !== undefined ? inner.replace(/\\([\\$}])/g, "$1") : escaped,
  );
}

test("the generated snippet file is up to date with the docs", () => {
  assert.equal(
    readFileSync(SNIPPETS_OUTPUT_PATH, "utf8"),
    buildSnippetOutputs().json,
    "snippets/probescript.generated.json is out of date: run npm run gen:snippets in vscode/",
  );
});

test("each skeleton, with its tab stops at their defaults, is the documented example it was made from", () => {
  const { skeletons } = buildSnippetOutputs();
  const wrong = Object.entries(skeletons)
    .filter(([, skeleton]) => withDefaults(skeleton.body) !== skeleton.prefix)
    .map(([command]) => command);
  assert.deepEqual(wrong, [], "these skeletons do not reproduce their documented example");
});

test("every documented command with an argument has a hand-written snippet or a skeleton", () => {
  const handWritten = JSON.parse(readFileSync(HAND_WRITTEN_PATH, "utf8"));
  const skeletons = JSON.parse(readFileSync(SNIPPETS_OUTPUT_PATH, "utf8"));
  const missing = [];
  for (const row of parseDictionary(readFileSync(DICTIONARY_PATH, "utf8"))) {
    const example = splitExamples(row.syntax)[0];
    if (!example || !plainWords(example) || !hasTabStop(exampleSnippet(example))) {
      continue;
    }
    if (!isCovered(example, handWritten) && !(row.command in skeletons)) {
      missing.push(`${row.command} (from "${example}")`);
    }
  }
  assert.deepEqual(missing, [], "these documented commands have no snippet: add a hand-written one or regenerate");
});

test("the generated snippet file is listed under contributes.snippets for ProbeScript", () => {
  const pkg = JSON.parse(readFileSync(resolve(here, "../package.json"), "utf8"));
  const listed = pkg.contributes.snippets.map((entry) => `${entry.language} ${entry.path}`);
  assert.ok(
    listed.includes("probescript ./snippets/probescript.generated.json"),
    "package.json does not list snippets/probescript.generated.json; add it to contributes.snippets",
  );
});
