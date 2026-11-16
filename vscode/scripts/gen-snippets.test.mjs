// Drift tests for the generated skeleton snippets (scripts/gen-snippets.mjs). They fail when the docs change and the files are not
// regenerated, when a skeleton does not reproduce its documented example, when a skeleton has no context, and when a documented
// command with an argument has neither a hand-written snippet nor a skeleton nor a reason. The Go gate
// (internal/runner/skeleton_snippets_test.go) checks that every skeleton parses; this file cannot run the parser.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { DICTIONARY_PATH, parseDictionary } from "../../studio/frontend/scripts/gen-probescript-dictionary.mjs";
import { exampleSnippet, hasTabStop, splitExamples } from "./gen-keywords.mjs";
import {
  CONTEXTS_OUTPUT_PATH,
  HAND_WRITTEN_PATH,
  SNIPPETS_OUTPUT_PATH,
  buildSnippetOutputs,
  exclusion,
  isCovered,
  plainWords,
} from "./gen-snippets.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const CONTEXT_NAMES = new Set(["test", "top", "file", "composite"]);

// withDefaults returns a snippet with each tab stop replaced by its default text, which is what it inserts when the user leaves a
// tab stop as it is. Escapes are read the way VS Code reads them.
function withDefaults(snippet) {
  return snippet.replace(/\$\{\d+:((?:\\[\\$}]|[^}\\])*)\}|\\([\\$}])/g, (_, inner, escaped) =>
    inner !== undefined ? inner.replace(/\\([\\$}])/g, "$1") : escaped,
  );
}

test("the generated snippet files are up to date with the docs", () => {
  const outputs = buildSnippetOutputs();
  assert.equal(
    readFileSync(SNIPPETS_OUTPUT_PATH, "utf8"),
    outputs.json,
    "snippets/probescript.generated.json is out of date: run npm run gen:snippets in vscode/",
  );
  assert.equal(
    readFileSync(CONTEXTS_OUTPUT_PATH, "utf8"),
    outputs.contextsJson,
    "snippets/probescript.generated.contexts.json is out of date: run npm run gen:snippets in vscode/",
  );
});

test("each skeleton, with its tab stops at their defaults, is the documented example it was made from", () => {
  const { skeletons } = buildSnippetOutputs();
  const wrong = Object.entries(skeletons)
    .filter(([, skeleton]) => withDefaults(skeleton.body) !== skeleton.prefix)
    .map(([command]) => command);
  assert.deepEqual(wrong, [], "these skeletons do not reproduce their documented example");
});

test("each skeleton has a context the Go gate knows, and no context is left without a skeleton", () => {
  const { skeletons, contexts } = buildSnippetOutputs();
  assert.deepEqual(Object.keys(contexts).sort(), Object.keys(skeletons).sort());
  const unknown = Object.entries(contexts)
    .filter(([, context]) => !CONTEXT_NAMES.has(context))
    .map(([command, context]) => `${command}: ${context}`);
  assert.deepEqual(unknown, [], "these skeletons have a context the Go gate does not know");
});

test("every documented command with an argument has a hand-written snippet, a skeleton, or a reason it has neither", () => {
  const handWritten = JSON.parse(readFileSync(HAND_WRITTEN_PATH, "utf8"));
  const { skeletons } = buildSnippetOutputs();
  const missing = [];
  for (const row of parseDictionary(readFileSync(DICTIONARY_PATH, "utf8"))) {
    const example = splitExamples(row.syntax)[0];
    if (!example || !plainWords(example) || !hasTabStop(exampleSnippet(example))) {
      continue;
    }
    if (!isCovered(example, handWritten) && !(row.command in skeletons) && !exclusion(row.command, example)) {
      missing.push(`${row.command} (from "${example}")`);
    }
  }
  assert.deepEqual(missing, [], "these documented commands have no snippet and no reason in gen-snippets.mjs");
});

test("the generated snippet file is listed under contributes.snippets for ProbeScript", () => {
  const pkg = JSON.parse(readFileSync(resolve(here, "../package.json"), "utf8"));
  const listed = pkg.contributes.snippets.map((entry) => `${entry.language} ${entry.path}`);
  assert.ok(
    listed.includes("probescript ./snippets/probescript.generated.json"),
    "package.json does not list snippets/probescript.generated.json; add it to contributes.snippets",
  );
});
