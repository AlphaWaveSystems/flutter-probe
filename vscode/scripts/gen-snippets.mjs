// Generates the skeleton snippets of the ProbeScript language: one for each command of dictionary.md that takes an argument and that
// no hand-written snippet (snippets/probescript.json) covers. The skeleton is the row's first example with its quoted texts and
// numbers as tab stops, and its prefix is that example, so typing the command offers the whole form. A command without an
// argument is left out: the keyword completion already offers it.
//
// Two files are written:
//   - snippets/probescript.generated.json, the snippets VS Code reads (listed under contributes.snippets);
//   - snippets/probescript.generated.contexts.json, where each snippet stands in a file: `test` (a step inside a test, the
//     default), `top` (a file-level line), `file` (a file-level line after a test) or `composite` (a line inside a composite
//     test). The Go gate (internal/runner/skeleton_snippets_test.go) wraps each snippet in its context and lints it.
//
// A hand-written snippet covers a row when its first line has the same command form: the same words once the arguments are
// removed (`tap on "${1:element}"` covers `tap "Label"`; `wait until "${1:x}" appears` does not cover `wait 5 seconds`).
//
// Usage (from vscode/):
//   node scripts/gen-snippets.mjs           write the generated files
//   node scripts/gen-snippets.mjs --check   exit 1 when a generated file is out of date

import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { DICTIONARY_PATH, parseDictionary } from "../../studio/frontend/scripts/gen-probescript-dictionary.mjs";
import { exampleSnippet, hasTabStop, splitExamples } from "./gen-keywords.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const vscodeRoot = resolve(here, "..");

export const HAND_WRITTEN_PATH = resolve(vscodeRoot, "snippets/probescript.json");
export const SNIPPETS_OUTPUT_PATH = resolve(vscodeRoot, "snippets/probescript.generated.json");
export const CONTEXTS_OUTPUT_PATH = resolve(vscodeRoot, "snippets/probescript.generated.contexts.json");

// CONTEXTS says where a form stands when it is not a step inside a test: a composite test header is a file-level line, `with
// examples from` follows a test at file level, and `sync` sits directly inside a composite test.
export const CONTEXTS = {
  "composite test": "top",
  "with examples from": "file",
  sync: "composite",
};

// SKIPPED names the forms that are left out, with the reason. `respond` is the line under `when the app calls`, which is not a
// step on its own; the HTTP Mock snippet has the whole form.
export const SKIPPED = {
  respond: "the line under when the app calls, not a step on its own; the HTTP Mock snippet has the whole form",
};

// isTemplate reports whether an example is a syntax template rather than a step: alternatives (`up/down`), optional parts (`[on …]`),
// an ellipsis, a parenthesis, a choice (`a|b`) or a placeholder letter (`N`). Quoted text is not looked at.
export function isTemplate(example) {
  const bare = example.replace(/"[^"]*"/g, '""');
  return /[[\]()|]|\.\.\.|[a-z]\/[a-z]|\bN\b/.test(bare);
}

// exclusion returns why a documented command gets no skeleton, or "" when it does.
export function exclusion(command, example) {
  if (command in SKIPPED) {
    return SKIPPED[command];
  }
  if (isTemplate(example)) {
    return "a syntax template, not a step";
  }
  return "";
}

// argumentToken matches a token that is an argument, not a word of the command: a quote, a tab stop, a selector, a bracket or a
// choice, a number, a placeholder in capitals (N), or an HTTP method (GET).
const argumentToken = /["$#<\[\]()/]|^\d+$|^[A-Z]+$/;

// plainWords returns the words of a command form with its arguments, its choices and its filler `on` removed: `tap on "${1:element}"`
// and `tap "Label"` both give tap; `wait until "Loading" appears` gives wait until appears; `wait 5 seconds` gives wait seconds.
export function plainWords(form) {
  return form
    .replace(/\$\{\d+\|[^}]*\|\}/g, " ")
    .replace(/"[^"]*"/g, " ")
    .replace(/,/g, " ")
    .trim()
    .split(/\s+/)
    .filter((token) => token !== "" && !argumentToken.test(token) && token !== "on")
    .join(" ");
}

// bodyLines returns the lines of a hand-written snippet, whose body is a string or a list of lines.
function bodyLines(snippet) {
  return (Array.isArray(snippet.body) ? snippet.body.join("\n") : snippet.body).split("\n");
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

function sortByKey(object) {
  return Object.fromEntries(Object.entries(object).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)));
}

// skeletonFiles returns the skeletons and their contexts, each keyed by the command, sorted by key.
export function skeletonFiles(dictionary, handWritten) {
  const skeletons = {};
  const contexts = {};
  for (const row of parseDictionary(dictionary)) {
    const example = splitExamples(row.syntax)[0];
    if (!example || !plainWords(example) || row.command in skeletons || row.command in SKIPPED) {
      continue;
    }
    if (isCovered(example, handWritten) || exclusion(row.command, example)) {
      continue;
    }
    const body = exampleSnippet(example);
    if (!hasTabStop(body)) {
      continue;
    }
    skeletons[row.command] = { prefix: example, body, description: plainText(row.description) };
    contexts[row.command] = CONTEXTS[row.command] ?? "test";
  }
  return { skeletons: sortByKey(skeletons), contexts: sortByKey(contexts) };
}

// buildSnippetOutputs returns the text of the two generated files, from the docs and the hand-written snippets as they are now.
export function buildSnippetOutputs() {
  const dictionary = readFileSync(DICTIONARY_PATH, "utf8");
  const handWritten = JSON.parse(readFileSync(HAND_WRITTEN_PATH, "utf8"));
  const { skeletons, contexts } = skeletonFiles(dictionary, handWritten);
  return {
    json: `${JSON.stringify(skeletons, null, 2)}\n`,
    contextsJson: `${JSON.stringify(contexts, null, 2)}\n`,
    skeletons,
    contexts,
  };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const outputs = buildSnippetOutputs();
  if (process.argv.includes("--check")) {
    const now = existsSync(SNIPPETS_OUTPUT_PATH) ? readFileSync(SNIPPETS_OUTPUT_PATH, "utf8") : "";
    const contextsNow = existsSync(CONTEXTS_OUTPUT_PATH) ? readFileSync(CONTEXTS_OUTPUT_PATH, "utf8") : "";
    if (now !== outputs.json || contextsNow !== outputs.contextsJson) {
      console.error("the generated snippet files are out of date: run npm run gen:snippets in vscode/");
      process.exit(1);
    }
    console.log("the generated snippet files are up to date");
  } else {
    writeFileSync(SNIPPETS_OUTPUT_PATH, outputs.json);
    writeFileSync(CONTEXTS_OUTPUT_PATH, outputs.contextsJson);
    console.log(`wrote ${SNIPPETS_OUTPUT_PATH} (${Object.keys(outputs.skeletons).length} skeleton snippets) and its contexts`);
  }
}
