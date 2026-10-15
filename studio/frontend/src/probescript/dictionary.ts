// Editor help for ProbeScript, built from the generated dictionary (scripts/gen-probescript-dictionary.mjs). Nothing here
// imports monaco-editor, so the unit tests run under node --test; src/probescript.ts adapts these results to Monaco.

export interface CommandEntry {
  readonly command: string;
  readonly syntax: string;
  readonly description: string;
  readonly category: string;
}

export interface CompletionSpec {
  readonly label: string;
  readonly kind: "command" | "keyword";
  readonly detail: string;
  /** Markdown shown next to the suggestion. */
  readonly documentation: string;
  readonly insertText: string;
  /** True when insertText uses snippet placeholders (${n:...}). */
  readonly insertAsSnippet: boolean;
}

// escapeSnippet escapes the characters that have a meaning in an editor snippet.
function escapeSnippet(text: string): string {
  return text.replace(/[\\$}]/g, "\\$&");
}

// snippetFor turns the first form of a syntax line into an editor snippet. Each quoted argument becomes a tab stop that
// starts with the docs' own example text. A syntax line without quoted arguments is inserted as it is.
export function snippetFor(syntax: string): { text: string; snippet: boolean } {
  const first = syntax.split(/\s+or\s+/)[0].trim();
  const quoted = /"([^"]*)"/g;
  const parts: string[] = [];
  let last = 0;
  let stops = 0;
  let match: RegExpExecArray | null;
  while ((match = quoted.exec(first)) !== null) {
    stops += 1;
    parts.push(escapeSnippet(first.slice(last, match.index)));
    parts.push('"${' + stops + ":" + escapeSnippet(match[1]) + '}"');
    last = match.index + match[0].length;
  }
  if (stops === 0) {
    return { text: first, snippet: false };
  }
  parts.push(escapeSnippet(first.slice(last)));
  return { text: parts.join(""), snippet: true };
}

// completionSpecs lists what the editor offers: one entry per command, then one per reserved word that no command
// already names. A command's label is its command text; its documentation is its syntax and its description.
export function completionSpecs(commands: readonly CommandEntry[], keywords: readonly string[]): CompletionSpec[] {
  const specs: CompletionSpec[] = [];
  const named = new Set<string>();
  for (const entry of commands) {
    if (named.has(entry.command)) {
      continue; // the same command in two tables: the first one wins
    }
    named.add(entry.command);
    const { text, snippet } = snippetFor(entry.syntax);
    specs.push({
      label: entry.command,
      kind: "command",
      detail: entry.category,
      documentation: "```probe\n" + entry.syntax + "\n```\n\n" + entry.description,
      insertText: text,
      insertAsSnippet: snippet,
    });
  }
  for (const word of keywords) {
    if (named.has(word)) {
      continue;
    }
    named.add(word);
    specs.push({
      label: word,
      kind: "keyword",
      detail: "ProbeScript keyword",
      documentation: "ProbeScript keyword `" + word + "`.",
      insertText: word,
      insertAsSnippet: false,
    });
  }
  return specs;
}

const MAX_HOVER_ENTRIES = 5;

// hoverMarkdown explains a word under the cursor: the commands whose text is that word (first) and the commands that
// start with it, or the keyword itself. It returns undefined for a word that ProbeScript does not define.
export function hoverMarkdown(
  word: string,
  commands: readonly CommandEntry[],
  keywords: readonly string[],
): string | undefined {
  const exact = commands.filter(entry => entry.command === word);
  const startsWith = commands.filter(entry => entry.command !== word && entry.command.split(" ")[0] === word);
  const entries = [...exact, ...startsWith].slice(0, MAX_HOVER_ENTRIES);
  if (entries.length > 0) {
    return entries
      .map(entry => "**" + entry.command + "** · " + entry.category + "\n\n```probe\n" + entry.syntax + "\n```\n\n" + entry.description)
      .join("\n\n---\n\n");
  }
  if (keywords.includes(word)) {
    return "ProbeScript keyword `" + word + "`.";
  }
  return undefined;
}
