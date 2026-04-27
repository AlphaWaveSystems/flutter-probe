// Monaco language definition for ProbeScript.
//
// This is a hand-tuned Monarch tokenizer that captures the same surface as
// `vscode/syntaxes/probescript.tmLanguage.json` (the VS Code extension's
// TextMate grammar). The two are intentionally similar but Monaco prefers
// Monarch over TextMate JSON, so this file owns the Studio version.

import * as monaco from "monaco-editor";

export const PROBESCRIPT_LANGUAGE_ID = "probescript";

export function registerProbeScript(): void {
  monaco.languages.register({
    id: PROBESCRIPT_LANGUAGE_ID,
    extensions: [".probe"],
    aliases: ["ProbeScript", "probescript"],
  });

  monaco.languages.setLanguageConfiguration(PROBESCRIPT_LANGUAGE_ID, {
    comments: { lineComment: "#" },
    brackets: [
      ["(", ")"],
      ["[", "]"],
      ["{", "}"],
    ],
    autoClosingPairs: [
      { open: '"', close: '"' },
      { open: "'", close: "'" },
      { open: "(", close: ")" },
      { open: "[", close: "]" },
      { open: "{", close: "}" },
      { open: "${", close: "}" },
    ],
  });

  monaco.languages.setMonarchTokensProvider(PROBESCRIPT_LANGUAGE_ID, {
    defaultToken: "",
    tokenPostfix: ".probe",

    keywords: [
      "test", "recipe", "use", "before", "after", "each", "all", "on", "failure",
      "if", "else", "repeat", "times", "with", "examples", "from",
      "tap", "double", "long", "press", "type", "clear", "swipe", "scroll",
      "see", "wait", "for", "until", "open", "link", "restart", "kill",
      "app", "data", "permission", "allow", "deny", "set", "location",
      "store", "as", "verify", "external", "browser", "opened",
      "call", "GET", "POST", "PUT", "DELETE", "body",
      "below", "above", "left", "right", "of",
      "is", "focused", "visible", "present",
      "take", "screenshot", "called",
      "animations", "to", "end",
      "copy", "paste", "clipboard",
    ],

    operators: ["@"],

    tokenizer: {
      root: [
        // Comments
        [/#.*$/, "comment"],

        // Strings
        [/"([^"\\]|\\.)*$/, "string.invalid"],
        [/"/, { token: "string.quote", bracket: "@open", next: "@string" }],

        // Variables: <name>, ${name}
        [/<[a-zA-Z_][\w.]*>/, "variable"],
        [/\$\{[a-zA-Z_][\w.]*\}/, "variable"],

        // Tags: @smoke, @critical
        [/@[a-zA-Z_][\w-]*/, "annotation"],

        // Test / recipe declarations get a brighter color via "type"
        [/^\s*(test|recipe)\b/, "type"],

        // Numbers
        [/-?\d+(\.\d+)?/, "number"],

        // Keywords
        [/[a-zA-Z_][\w-]*/, {
          cases: {
            "@keywords": "keyword",
            "@default": "identifier",
          },
        }],

        // Whitespace
        [/[ \t\r\n]+/, ""],
      ],

      string: [
        [/[^\\"]+/, "string"],
        [/\\./, "string.escape"],
        [/"/, { token: "string.quote", bracket: "@close", next: "@pop" }],
      ],
    },
  });
}
