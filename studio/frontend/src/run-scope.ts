// The run controls next to Run, and the grouping of finished tests by outcome. Nothing here touches the DOM, so the
// node tests can run it directly.

export type RunMode = "file" | "line" | "tag" | "failed";

export type Outcome = "pass" | "fail" | "skip";

export type RunScopeArgs = { line: number; tag: string; failedOnly: boolean };

export type RunScopeChoice = { ok: true; args: RunScopeArgs } | { ok: false; message: string };

// runScopeArgs turns the run controls into the arguments of RunFileScoped. cursorLine is the editor's 1-based line, or
// undefined when the editor has no position. tagText is whatever the tag field holds; a leading "@" is optional.
export function runScopeArgs(mode: RunMode, cursorLine: number | undefined, tagText: string): RunScopeChoice {
  switch (mode) {
    case "file":
      return { ok: true, args: { line: 0, tag: "", failedOnly: false } };
    case "line":
      if (!cursorLine || cursorLine < 1) {
        return { ok: false, message: "Put the cursor inside a test to run that test." };
      }
      return { ok: true, args: { line: cursorLine, tag: "", failedOnly: false } };
    case "tag": {
      const tag = tagText.trim().replace(/^@/, "").trim();
      if (tag === "") {
        return { ok: false, message: "Type a tag to run, such as smoke." };
      }
      return { ok: true, args: { line: 0, tag, failedOnly: false } };
    }
    case "failed":
      return { ok: true, args: { line: 0, tag: "", failedOnly: true } };
  }
}

const OUTCOME_GROUPS: { outcome: Outcome; label: string }[] = [
  { outcome: "fail", label: "Failed" },
  { outcome: "pass", label: "Passed" },
  { outcome: "skip", label: "Skipped" },
];

// groupByOutcome splits items into failed, passed and skipped groups, in that order. Items keep their run order inside
// a group, and empty groups are dropped.
export function groupByOutcome<T extends { outcome: Outcome }>(items: T[]): { label: string; items: T[] }[] {
  return OUTCOME_GROUPS.map(({ outcome, label }) => ({
    label,
    items: items.filter((item) => item.outcome === outcome),
  })).filter((group) => group.items.length > 0);
}
