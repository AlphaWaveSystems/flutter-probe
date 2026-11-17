// Drift tests for the generated completion lists (scripts/gen-keywords.mjs). They fail when the docs change and the lists are not
// regenerated, when a command form of dictionary.md is not offered as a completion, and when a command's lead is not a word.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { DICTIONARY_PATH, parseDictionary } from "../../studio/frontend/scripts/gen-probescript-dictionary.mjs";
import {
  GAPS_PATH,
  KEYWORDS_OUTPUT_PATH,
  TEXTMATE_PATH,
  buildOutputs,
  examplePhrase,
  leadOf,
  splitExamples,
  exampleSnippet,
} from "./gen-keywords.mjs";

// quotedList reads the strings of the array that starts at `name` in the generated module, one entry per line.
function quotedList(moduleText, name) {
  const block = new RegExp(`${name}: readonly [^=]*= \\[\\n([\\s\\S]*?)\\n\\];`).exec(moduleText);
  assert.ok(block, `${name} is not in the generated module`);
  return [...block[1].matchAll(/^ {2}("(?:[^"\\]|\\.)*"),$/gm)].map((match) => JSON.parse(match[1]));
}

// commandLabels reads the command forms back out of the generated module: the label of each entry of PROBESCRIPT_COMMANDS.
function commandLabels(moduleText) {
  const block = /PROBESCRIPT_COMMANDS: readonly [^=]*= \[\n([\s\S]*?)\n\];/.exec(moduleText);
  assert.ok(block, "PROBESCRIPT_COMMANDS is not in the generated module");
  return [...block[1].matchAll(/^ {2}\{ label: ("(?:[^"\\]|\\.)*"), insert: /gm)].map((match) => JSON.parse(match[1]));
}

// textMateMatch reads the match of the generated-keywords rule back out of the grammar.
function textMateMatch(grammarText) {
  const grammar = JSON.parse(grammarText);
  const rule = grammar.repository["generated-keywords"];
  assert.ok(rule, "the grammar has no generated-keywords rule");
  return rule.match;
}

function gaps() {
  return JSON.parse(readFileSync(GAPS_PATH, "utf8"));
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

test("every command form of dictionary.md (its Command cell) is offered as a completion, or is listed in keyword-gaps.json", () => {
  const offered = commandLabels(readFileSync(KEYWORDS_OUTPUT_PATH, "utf8"));
  const words = quotedList(readFileSync(KEYWORDS_OUTPUT_PATH, "utf8"), "PROBESCRIPT_KEYWORDS");
  const listed = gaps().forms ?? [];
  const missing = [];
  for (const row of parseDictionary(readFileSync(DICTIONARY_PATH, "utf8"))) {
    const form = row.command;
    if (!offered.includes(form) && !words.includes(form) && !listed.includes(form)) {
      missing.push(form);
    }
  }
  assert.deepEqual(missing, [], "these command forms are not completions and are not listed in scripts/keyword-gaps.json");
});

test("every plain example phrase of dictionary.md (its Syntax cell) is offered as a completion, or is listed in keyword-gaps.json", () => {
  const text = readFileSync(KEYWORDS_OUTPUT_PATH, "utf8");
  const offered = commandLabels(text);
  const words = quotedList(text, "PROBESCRIPT_KEYWORDS");
  const listed = gaps().forms ?? [];
  const missing = new Set();
  for (const row of parseDictionary(readFileSync(DICTIONARY_PATH, "utf8"))) {
    for (const example of splitExamples(row.syntax)) {
      const phrase = examplePhrase(example);
      if (phrase && !offered.includes(phrase) && !words.includes(phrase) && !listed.includes(phrase)) {
        missing.add(`${phrase} (from "${example}")`);
      }
    }
  }
  assert.deepEqual([...missing], [], "these example phrases are not completions and are not listed in scripts/keyword-gaps.json");
});

test("every command of dictionary.md has its lead as a completion word, or is listed as a gap", () => {
  const words = quotedList(readFileSync(KEYWORDS_OUTPUT_PATH, "utf8"), "PROBESCRIPT_KEYWORDS");
  const leads = gaps().leads ?? [];
  const missing = [];
  for (const row of parseDictionary(readFileSync(DICTIONARY_PATH, "utf8"))) {
    const lead = leadOf(row.command);
    if (lead && !words.includes(lead) && !leads.includes(lead)) {
      missing.push(`${row.command} (lead "${lead}")`);
    }
  }
  assert.deepEqual(missing, [], "these commands have no completion word and are not in scripts/keyword-gaps.json");
});

test("the TextMate generated-keywords rule matches every completion word", () => {
  const words = quotedList(readFileSync(KEYWORDS_OUTPUT_PATH, "utf8"), "PROBESCRIPT_KEYWORDS");
  const match = new RegExp(textMateMatch(readFileSync(TEXTMATE_PATH, "utf8")));
  const unmatched = words.filter((word) => !match.test(word));
  assert.deepEqual(unmatched, [], "these completion words are not matched by the grammar rule");
});

test("an #id selector of an example becomes a tab stop, so the completion is a complete step", () => {
  assert.equal(exampleSnippet("see #password is filled"), "see #${1:password} is filled");
  assert.equal(exampleSnippet("drag #source to #target"), "drag #${1:source} to #${2:target}");
  // a hash inside a quoted text is part of the text, not a selector
  assert.equal(exampleSnippet('tap "#not-a-selector" 2 times'), 'tap "${1:#not-a-selector}" ${2:2} times');
});
