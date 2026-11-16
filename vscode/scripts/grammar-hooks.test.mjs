// Drift test for the TextMate hook rule (syntaxes/probe.tmGrammar.json). Every hook of dictionary.md must be highlighted as a hook
// when it starts a line. The parser takes a hook only at the start of a line, so an indented one is not highlighted.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { DICTIONARY_PATH, parseDictionary } from "../../studio/frontend/scripts/gen-probescript-dictionary.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const grammar = JSON.parse(readFileSync(resolve(here, "../syntaxes/probe.tmGrammar.json"), "utf8"));
const hookRule = new RegExp(grammar.repository.hook.match);
const hookForm = /^(before (each|all)|after (each|all)|on failure)$/;

test("every hook of dictionary.md is matched by the hook rule at the start of a line", () => {
  const hooks = parseDictionary(readFileSync(DICTIONARY_PATH, "utf8"))
    .map((row) => row.command)
    .filter((command) => hookForm.test(command));
  assert.equal(hooks.length, 5, "dictionary.md should list five hooks: before each, after each, before all, after all, on failure");
  assert.deepEqual(
    hooks.filter((command) => !hookRule.test(command)),
    [],
    "these hooks of dictionary.md are not highlighted as hooks by the grammar",
  );
});

test("the hook rule also matches the test-suffixed forms", () => {
  assert.ok(hookRule.test("before each test"));
  assert.ok(hookRule.test("after each test"));
});

test("the hook rule does not match an indented hook, which the parser rejects", () => {
  assert.equal(hookRule.test("  before each"), false);
});
