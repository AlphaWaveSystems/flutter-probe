// Generates the ProbeScript completion lists and the keyword rule of the VS Code extension from the same docs as the Studio
// dictionary, and with the same parsers (studio/frontend/scripts/gen-probescript-dictionary.mjs exports them):
//   - src/providers/probescriptKeywords.generated.ts:
//       PROBESCRIPT_KEYWORDS, the words the completion list offers;
//       PROBESCRIPT_COMMANDS, the command forms the completion list offers, each with its snippet text;
//   - syntaxes/probe.tmGrammar.json: the `generated-keywords` rule, the keyword words as one match. Only that entry is rewritten.
//
// A keyword word is the lead of a command in dictionary.md (the words typed before its first argument: `tap` for `tap "Label"`,
// `press` for `press <key>`, `wait` for `wait N seconds`) and each reserved word of grammar.md, except the plain English
// function words listed below.
//
// A command form is the Command cell of a row of dictionary.md (`press <key>`, `wait N seconds`, `see exactly N requests`)
// and each plain phrase of its Syntax cell (`see no requests` for `see no requests "/x"`, `with examples:` for
// `with examples:`). Its snippet text has the arguments as tab stops. scripts/keyword-gaps.json lists the forms that are
// deliberately not offered, with the reason; scripts/gen-keywords.test.mjs fails when a form is in neither place.
//
// Usage (from vscode/):
//   node scripts/gen-keywords.mjs           write the generated files
//   node scripts/gen-keywords.mjs --check   exit 1 when a generated file is out of date

import { createHash } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  DICTIONARY_PATH,
  GRAMMAR_PATH,
  parseDictionary,
  parseGrammar,
} from "../../studio/frontend/scripts/gen-probescript-dictionary.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const vscodeRoot = resolve(here, "..");

export const KEYWORDS_OUTPUT_PATH = resolve(vscodeRoot, "src/providers/probescriptKeywords.generated.ts");
export const TEXTMATE_PATH = resolve(vscodeRoot, "syntaxes/probe.tmGrammar.json");
export const GAPS_PATH = resolve(here, "keyword-gaps.json");
export const EXTRA_FORMS_PATH = resolve(here, "extra-forms.json");

// The English function words of the reserved list: a completion for "the" or "into" would offer nothing a user wants.
export const FUNCTION_WORDS = new Set([
  "a", "an", "and", "are", "as", "at", "each", "for", "from", "in", "into", "is", "it", "of", "that", "the", "this", "to", "with",
]);

// leadOf returns the words a user types before a command's first argument. It stops at a quote, a placeholder, an ellipsis or
// a word that is not lower case (`<key>`, `N`, `...`, `"Label"`).
export function leadOf(command) {
  const words = [];
  for (const word of command.split(/\s+/)) {
    if (!/^[a-z][a-z']*$/.test(word)) {
      break;
    }
    words.push(word);
  }
  return words.join(" ");
}

// splitExamples returns the alternatives of a Syntax cell: `tap "Label"` or `tap #key` are two examples of the tap row.
export function splitExamples(syntax) {
  return syntax.split(/\s+or\s+|\s+·\s+/).map((part) => part.trim()).filter(Boolean);
}

// examplePhrase returns the plain words of an example before its first argument: `see no requests` for `see no requests "/x"`,
// `with examples:` for `with examples:`. It is empty for an example that starts with a placeholder or a choice of words
// (`swipe up/down/left/right`), since those are covered by the lead of the command.
export function examplePhrase(example) {
  const head = example.split(/["<\[#(]/)[0].trim();
  return /^[a-z][a-z' :]*$/.test(head) ? head.replace(/\s+/g, " ") : "";
}

// commandForms returns the command forms the completion list offers, sorted by label: the Command cell of each row and the
// plain phrase of each of its examples, each with the snippet insertText makes; then the extra forms of extra-forms.json,
// which the parser accepts and the dictionary does not list, with their snippet as written there.
export function commandForms(dictionary, extra) {
  const forms = new Map();
  for (const row of parseDictionary(dictionary)) {
    forms.set(row.command, insertText(row.command));
    for (const example of splitExamples(row.syntax)) {
      const phrase = examplePhrase(example);
      if (phrase && !forms.has(phrase)) {
        forms.set(phrase, insertText(phrase));
      }
    }
  }
  for (const form of extra) {
    forms.set(form.label, form.insert);
  }
  return [...forms].map(([label, insert]) => ({ label, insert })).sort((a, b) => (a.label < b.label ? -1 : a.label > b.label ? 1 : 0));
}

// commandLabels returns the labels of commandForms, for the tests.
export function commandLabels(dictionary, extra) {
  return commandForms(dictionary, extra).map((form) => form.label);
}

// insertText is the snippet a completion inserts for a command form: each placeholder (`<key>`, `N`) becomes a tab stop, and
// the ellipsis of a form (`open link ... in the app`) is dropped.
export function insertText(label) {
  let stop = 0;
  return label
    .replace(/<([a-z.]+)>/g, (_, name) => `\${${++stop}:${name}}`)
    .replace(/\bN\b/g, () => `\${${++stop}:N}`)
    .replace(/\s*\.\.\.\s*/g, " ")
    .trim();
}

// keywordWords returns the completion words: the command leads, then the reserved words, without duplicates. Sorted, so the
// output does not change when a row moves in the docs.
export function keywordWords(dictionary, grammar) {
  const words = new Set();
  for (const row of parseDictionary(dictionary)) {
    const lead = leadOf(row.command);
    if (lead) {
      words.add(lead);
    }
  }
  for (const word of parseGrammar(grammar)) {
    if (!FUNCTION_WORDS.has(word)) {
      words.add(word);
    }
  }
  return [...words].sort();
}

function escapeRegex(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// keywordMatch is the TextMate match for the words: longest first, so `wait until` wins over `wait`.
export function keywordMatch(words) {
  const alternatives = [...words].sort((a, b) => b.length - a.length || a.localeCompare(b)).map(escapeRegex);
  return `\\b(${alternatives.join("|")})\\b`;
}

function renderKeywordsModule(words, commands, sourceHash) {
  const wordList = words.map((word) => `  ${JSON.stringify(word)},`).join("\n");
  const commandList = commands
    .map((form) => `  { label: ${JSON.stringify(form.label)}, insert: ${JSON.stringify(form.insert)} },`)
    .join("\n");
  return [
    "// Generated by vscode/scripts/gen-keywords.mjs from the ProbeScript docs (dictionary.md and grammar.md), with the parsers of",
    "// studio/frontend/scripts/gen-probescript-dictionary.mjs. Do not edit by hand. After a change under",
    "// website/src/content/docs/probescript/, run `npm run gen:keywords` in vscode/.",
    "",
    "/** The words the completion list offers: the lead of each command, and each reserved word that is not a function word. */",
    "export const PROBESCRIPT_KEYWORDS: readonly string[] = [",
    wordList,
    "];",
    "",
    "/** The command forms the completion list offers, with the snippet each inserts (the arguments are tab stops). */",
    "export const PROBESCRIPT_COMMANDS: readonly { readonly label: string; readonly insert: string }[] = [",
    commandList,
    "];",
    "",
    `export const SOURCE_SHA256 = ${JSON.stringify(sourceHash)};`,
    "",
  ].join("\n");
}

// rewriteTextMate replaces the generated-keywords entry of the grammar with one whose match is the given one. The rest of the
// file is kept byte for byte.
export function rewriteTextMate(grammarText, match) {
  const entry = /"generated-keywords": \{\n\s*"match": "(?:[^"\\]|\\.)*",\n\s*"name": "[^"]*"\n\s*\}/;
  if (!entry.test(grammarText)) {
    throw new Error('probe.tmGrammar.json has no "generated-keywords" entry to rewrite');
  }
  const rendered = `"generated-keywords": {\n      "match": ${JSON.stringify(match)},\n      "name": "keyword.other.probe"\n    }`;
  return grammarText.replace(entry, rendered);
}

// buildOutputs returns the text of the two generated files, from the docs as they are now.
export function buildOutputs() {
  const dictionary = readFileSync(DICTIONARY_PATH, "utf8");
  const grammar = readFileSync(GRAMMAR_PATH, "utf8");
  const words = keywordWords(dictionary, grammar);
  const extra = JSON.parse(readFileSync(EXTRA_FORMS_PATH, "utf8")).forms;
  const commands = commandForms(dictionary, extra);
  const sourceHash = createHash("sha256").update(dictionary).update("\0").update(grammar).digest("hex");
  const keywords = renderKeywordsModule(words, commands, sourceHash);
  const textmate = rewriteTextMate(readFileSync(TEXTMATE_PATH, "utf8"), keywordMatch(words));
  return { keywords, textmate, words, commands };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const outputs = buildOutputs();
  if (process.argv.includes("--check")) {
    const keywordsNow = existsSync(KEYWORDS_OUTPUT_PATH) ? readFileSync(KEYWORDS_OUTPUT_PATH, "utf8") : "";
    const textmateNow = readFileSync(TEXTMATE_PATH, "utf8");
    if (keywordsNow !== outputs.keywords || textmateNow !== outputs.textmate) {
      console.error("the generated keyword files are out of date: run npm run gen:keywords in vscode/");
      process.exit(1);
    }
    console.log("the generated keyword files are up to date");
  } else {
    writeFileSync(KEYWORDS_OUTPUT_PATH, outputs.keywords);
    writeFileSync(TEXTMATE_PATH, outputs.textmate);
    console.log(`wrote ${KEYWORDS_OUTPUT_PATH} and the generated-keywords rule of ${TEXTMATE_PATH}`);
  }
}
