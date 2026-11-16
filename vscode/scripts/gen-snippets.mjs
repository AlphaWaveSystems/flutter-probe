// Generates vscode/snippets/probescript.generated.json, the skeleton snippets of the ProbeScript language: one for each command of
// dictionary.md that takes an argument and that no hand-written snippet (snippets/probescript.json) covers. The skeleton is the
// row's first example with its quoted texts and numbers as tab stops, and its prefix is that example, so typing the command
// offers the whole form. A command without an argument is left out: the keyword completion already offers it.
//
// A hand-written snippet covers a row when its first line has the same command form: the same words once the arguments are removed
// (`tap on "${1:element}"` covers `tap "Label"`; `wait until "${1:x}" appears` does not cover `wait 5 seconds`).
//
// Usage (from vscode/):
//   node scripts/gen-snippets.mjs           write the generated file
//   node scripts/gen-snippets.mjs --check   exit 1 when the generated file is out of date

import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { DICTIONARY_PATH, parseDictionary } from "../../studio/frontend/scripts/gen-probescript-dictionary.mjs";
import { exampleSnippet, hasTabStop, splitExamples } from "./gen-keywords.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const vscodeRoot = resolve(here, "..");

export const HAND_WRITTEN_PATH = resolve(vscodeRoot, "snippets/probescript.json");
export const SNIPPETS_OUTPUT_PATH = resolve(vscodeRoot, "snippets/probescript.generated.json");

// bodyLines returns the lines of a hand-written snippet, whose body is a string or a list of lines.
function bodyLines(snippet) {
  return (Array.isArray(snippet.body) ? snippet.body.join("\n") : snippet.body).split("\n");
}

// argumentToken matches a token that is an argument, not a word of the command: a quote, a tab stop, a selector, a bracket or a
// choice, a number, a placeholder in capitals (N), or an HTTP method (GET).
const argumentToken = /["$#<\[\]()/]|^\d+$|^[A-Z]+$/;

// plainWords returns the words of a command form with its arguments and its filler `on` removed: `tap on "${1:element}"` and
// `tap "Label"` both give tap; `wait until "Loading" appears` gives wait until appears; `wait 5 seconds` gives wait seconds.
export function plainWords(form) {
  return form
    .replace(/"[^"]*"/g, " ")
    .replace(/,/g, " ")
    .trim()
    .split(/\s+/)
    .filter((token) => token !== "" && !argumentToken.test(token) && token !== "on")
    .join(" ");
}

// isCovered reports whether a hand-written snippet has the same command form as the example: the words, with the arguments
// removed, are the same. So `tap on "${1:element}"` covers `tap "Label"`, and `wait until "${1:x}" appears` covers
// `wait until "Loading" appears`, but not `wait 5 seconds`.
export function isCovered(example, handWritten) {
  const wanted = plainWords(example);
  return Object.values(handWritten).some((snippet) => plainWords(bodyLines(snippet)[0]) === wanted);
}

// plainText removes the markdown of a dictionary description: backticks and link targets.
function plainText(markdown) {
  return markdown.replace(/`/g, "").replace(/\[([^\]]+)\]\([^)]*\)/g, "$1").trim();
}

// skeletonSnippets returns the skeleton of each uncovered command with an argument, keyed by the command. Sorted by key.
export function skeletonSnippets(dictionary, handWritten) {
  const skeletons = {};
  for (const row of parseDictionary(dictionary)) {
    const example = splitExamples(row.syntax)[0];
    if (!example || !plainWords(example) || isCovered(example, handWritten) || row.command in skeletons) {
      continue;
    }
    const body = exampleSnippet(example);
    if (!hasTabStop(body)) {
      continue;
    }
    skeletons[row.command] = { prefix: example, body, description: plainText(row.description) };
  }
  return Object.fromEntries(Object.entries(skeletons).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)));
}

// buildSnippetOutputs returns the text of the generated snippet file, from the docs and the hand-written snippets as they are now.
export function buildSnippetOutputs() {
  const dictionary = readFileSync(DICTIONARY_PATH, "utf8");
  const handWritten = JSON.parse(readFileSync(HAND_WRITTEN_PATH, "utf8"));
  const skeletons = skeletonSnippets(dictionary, handWritten);
  return { json: `${JSON.stringify(skeletons, null, 2)}\n`, skeletons };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const outputs = buildSnippetOutputs();
  if (process.argv.includes("--check")) {
    const now = existsSync(SNIPPETS_OUTPUT_PATH) ? readFileSync(SNIPPETS_OUTPUT_PATH, "utf8") : "";
    if (now !== outputs.json) {
      console.error("the generated snippet file is out of date: run npm run gen:snippets in vscode/");
      process.exit(1);
    }
    console.log("the generated snippet file is up to date");
  } else {
    writeFileSync(SNIPPETS_OUTPUT_PATH, outputs.json);
    console.log(`wrote ${SNIPPETS_OUTPUT_PATH} (${Object.keys(outputs.skeletons).length} skeleton snippets)`);
  }
}
