// Drift tests for the generated keyword lists (scripts/gen-keywords.mjs). They fail when the docs change and the lists are not
// regenerated, and when a command of dictionary.md has a lead that is neither a completion word nor a listed gap.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { DICTIONARY_PATH, parseDictionary } from "../../studio/frontend/scripts/gen-probescript-dictionary.mjs";
import { GAPS_PATH, KEYWORDS_OUTPUT_PATH, TEXTMATE_PATH, buildOutputs, leadOf } from "./gen-keywords.mjs";

// completionWords reads the words back out of the generated TypeScript module, one quoted string per line.
function completionWords(moduleText) {
  const block = /PROBESCRIPT_KEYWORDS: readonly string\[\] = \[\n([\s\S]*?)\n\];/.exec(moduleText);
  assert.ok(block, "PROBESCRIPT_KEYWORDS is not in the generated module");
  return [...block[1].matchAll(/^ {2}("(?:[^"\\]|\\.)*"),$/gm)].map((match) => JSON.parse(match[1]));
}

// textMateMatch reads the match of the generated-keywords rule back out of the grammar.
function textMateMatch(grammarText) {
  const grammar = JSON.parse(grammarText);
  const rule = grammar.repository["generated-keywords"];
  assert.ok(rule, "the grammar has no generated-keywords rule");
  return rule.match;
}

test("the generated keyword files are up to date with the docs", () => {
  const outputs = buildOutputs();
  assert.equal(
    readFileSync(KEYWORDS_OUTPUT_PATH, "utf8"),
    outputs.keywords,
    "src/providers/probescriptKeywords.generated.ts is out of date: run npm run gen:keywords in vscode/",
  );
  assert.equal(
    readFileSync(TEXTMATE_PATH, "utf8"),
    outputs.textmate,
    "syntaxes/probe.tmGrammar.json is out of date: run npm run gen:keywords in vscode/",
  );
});

test("every command of dictionary.md has its lead as a completion word, or is listed as a gap", () => {
  const words = completionWords(readFileSync(KEYWORDS_OUTPUT_PATH, "utf8"));
  const gaps = JSON.parse(readFileSync(GAPS_PATH, "utf8")).leads;
  const missing = [];
  for (const row of parseDictionary(readFileSync(DICTIONARY_PATH, "utf8"))) {
    const lead = leadOf(row.command);
    if (lead && !words.includes(lead) && !gaps.includes(lead)) {
      missing.push(`${row.command} (lead "${lead}")`);
    }
  }
  assert.deepEqual(missing, [], "these commands have no completion word and are not in scripts/keyword-gaps.json");
});

test("the TextMate generated-keywords rule matches every completion word", () => {
  const words = completionWords(readFileSync(KEYWORDS_OUTPUT_PATH, "utf8"));
  const match = new RegExp(textMateMatch(readFileSync(TEXTMATE_PATH, "utf8")));
  const unmatched = words.filter((word) => !match.test(word));
  assert.deepEqual(unmatched, [], "these completion words are not matched by the grammar rule");
});
