import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import {
  OUTPUT_PATH,
  generate,
  parseDictionary,
  parseGrammar,
  renderModule,
} from "../scripts/gen-probescript-dictionary.mjs";
import { COMMANDS, KEYWORDS } from "../src/probescript/dictionary.generated.ts";
import { completionSpecs, hoverMarkdown, snippetFor } from "../src/probescript/dictionary.ts";

const DICTIONARY_FIXTURE = `
## Actions

| Command | Syntax | Description |
|---|---|---|
| \`tap\` | \`tap "Label"\` or \`tap #key\` | Tap a widget |
| \`swipe\` | \`swipe up/down/left/right\` | Swipe gesture |

## Hooks

| Hook | Syntax | Description |
|---|---|---|
| \`before each\` | \`before each\` | Run before every test |

## Placeholders

| Placeholder | Example Output | Description |
|---|---|---|
| \`<random.email>\` | \`user@test.com\` | Random email |
`;

const GRAMMAR_FIXTURE = `
\`\`\`ebnf
keyword = "test" | "recipe" | "tap"
        | "the" | "is" ;
(* the listed-word "verify" is documented here but lexes as a plain WORD *)
listed-word = "verify" | "set" ;
filler  = "the" | "is" ;
other = "not-a-keyword" ;
\`\`\`
`;

describe("parseDictionary", () => {
  it("reads command, keyword and hook tables with their category, and ignores other tables", () => {
    assert.deepEqual(parseDictionary(DICTIONARY_FIXTURE), [
      { command: "tap", syntax: 'tap "Label" or tap #key', description: "Tap a widget", category: "Actions" },
      { command: "swipe", syntax: "swipe up/down/left/right", description: "Swipe gesture", category: "Actions" },
      { command: "before each", syntax: "before each", description: "Run before every test", category: "Hooks" },
    ]);
  });
});

describe("parseGrammar", () => {
  it("reads the keyword and listed-word productions, leaves out the fillers, and ignores (* comments *)", () => {
    assert.deepEqual(parseGrammar(GRAMMAR_FIXTURE), ["test", "recipe", "tap", "verify", "set"]);
  });
});

describe("renderModule", () => {
  it("writes the hash and both lists as const arrays", () => {
    const text = renderModule({
      commands: [{ command: "tap", syntax: 'tap "x"', description: "d", category: "c" }],
      keywords: ["test"],
      sourceHash: "abc",
    });
    assert.match(text, /export const SOURCE_SHA256 = "abc";/);
    assert.match(text, /export const KEYWORDS = \[\n {2}"test"\n\] as const;/);
  });
});

describe("snippetFor", () => {
  it("turns each quoted argument of the first form into a tab stop with the docs' example", () => {
    assert.deepEqual(snippetFor('tap "Label" or tap #key'), { text: 'tap "${1:Label}"', snippet: true });
    assert.deepEqual(snippetFor('type "text" into "Field"'), {
      text: 'type "${1:text}" into "${2:Field}"',
      snippet: true,
    });
  });

  it("inserts a syntax line without quoted arguments as it is", () => {
    assert.deepEqual(snippetFor("swipe up/down/left/right"), { text: "swipe up/down/left/right", snippet: false });
  });

  it("escapes the characters that mean something inside a snippet", () => {
    assert.equal(snippetFor('log "costs $5}"').text, 'log "${1:costs \\$5\\}}"');
  });
});

describe("completionSpecs", () => {
  it("lists commands first, then reserved words that no command already names", () => {
    const specs = completionSpecs(
      [{ command: "tap", syntax: 'tap "Label"', description: "Tap", category: "Actions" }],
      ["tap", "test"],
    );
    assert.deepEqual(
      specs.map(s => [s.label, s.kind]),
      [["tap", "command"], ["test", "keyword"]],
    );
    assert.equal(specs[0].insertText, 'tap "${1:Label}"');
    assert.equal(specs[0].insertAsSnippet, true);
    assert.equal(specs[1].insertAsSnippet, false);
  });
});

describe("hoverMarkdown", () => {
  const commands = [
    { command: "tap", syntax: 'tap "Label"', description: "Tap a widget", category: "Actions" },
    { command: "tap if visible", syntax: 'tap "OK" if visible', description: "Tap only if present", category: "Actions" },
  ];

  it("explains a command by its exact name first, then by the commands that start with the word", () => {
    const text = hoverMarkdown("tap", commands, []) ?? "";
    assert.ok(text.indexOf('tap "Label"') < text.indexOf('tap "OK" if visible'));
    assert.match(text, /Tap a widget/);
  });

  it("explains a keyword that no command names, and returns undefined for other words", () => {
    assert.equal(hoverMarkdown("test", commands, ["test"]), "ProbeScript keyword `test`.");
    assert.equal(hoverMarkdown("Label", commands, ["test"]), undefined);
  });
});

describe("generated dictionary", () => {
  it("is the output of the generator for the docs as they are now", () => {
    assert.equal(
      readFileSync(OUTPUT_PATH, "utf8"),
      generate(),
      "dictionary.generated.ts is out of date: run npm run gen:probescript in studio/frontend",
    );
  });

  it("covers the documented commands and the reserved words", () => {
    assert.ok(COMMANDS.length > 100, `only ${COMMANDS.length} commands`);
    assert.ok(KEYWORDS.includes("test"));
    assert.ok(KEYWORDS.includes("recipe"));
    assert.ok(!KEYWORDS.includes("the"), "filler words are not completions");
  });
});
