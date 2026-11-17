// StepTriage is the rule-based suggestion for a failed step (studio/triage.go). Results shows it when the AI
// switch is off. Fix is a ProbeScript line to add before the step (absent when the rules have none); Reason
// says why the step failed.
export interface StepTriage {
  fix?: string;
  reason: string;
  // The text the screen showed that the failed text was another way of writing (studio/replace_step.go); then there is no fix.
  replacement?: string;
}

export interface TriageRow {
  label: string;
  text: string;
}

// triageRows returns the rows the Results pane shows under a failed step: the suggested fix when there is one,
// then the text the screen showed, then the reason.
export function triageRows(t: StepTriage): TriageRow[] {
  const rows: TriageRow[] = [];
  if (t.fix) rows.push({ label: "Suggested fix", text: t.fix });
  if (t.replacement) rows.push({ label: "On screen", text: t.replacement });
  rows.push({ label: "Reason", text: t.reason });
  return rows;
}
