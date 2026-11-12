package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// maxSelectorProposals is how many model requests one run makes for selector proposals.
const maxSelectorProposals = 3

// selectorProposalAdvisory is the fixed label every proposal carries in the report.
const selectorProposalAdvisory = "Proposed by a language model from the failure text. Not applied to the test file and not verified against the app. It does not change this result."

// maybeAIHeal implements `probe test --ai-heal`. It runs after the results are final and before the report is written,
// and it only sets the advisory Proposal of a failed result: it never changes Passed, Skipped or Error, never writes a
// test file, and cannot return an error. With no ai: block it makes no request. One request per failed selector step
// (file, line and selector), at most maxSelectorProposals requests per run, each bounded by ai.timeout; after the first
// failed request it stops asking, and that step's entry carries the cleaned reason.
func maybeAIHeal(ctx context.Context, w io.Writer, cfg *config.Config, results []runner.TestResult) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(w, "  AI selector proposal skipped: internal error (tests are unaffected)")
		}
	}()
	var failed []int
	for i, r := range results {
		if !r.Passed && !r.Skipped && r.Error != nil && r.FailedSelector != nil {
			failed = append(failed, i)
		}
	}
	if len(failed) == 0 {
		return
	}
	if !cfg.AI.Configured() {
		fmt.Fprintln(w, "  AI selector proposal skipped: no ai: provider configured in probe.yaml (tests are unaffected)")
		return
	}
	endpoint := config.ResolveEnvVar(cfg.AI.Endpoint)
	c, err := ai.NewTextCompleter(cfg.AI.Provider, config.ResolveEnvVar(cfg.AI.APIKey), cfg.AI.Model, endpoint, cfg.AI.Timeout)
	if err != nil {
		fmt.Fprintf(w, "  AI selector proposal skipped: %v (tests are unaffected)\n", err)
		return
	}
	// A ceiling on top of the per-request timeouts, so a wedged model server cannot hold the report back for long.
	hctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	seen := map[string]*runner.SelectorProposal{}
	requests, skipped, stopped := 0, 0, false
	for _, i := range failed {
		fs := results[i].FailedSelector
		key := fmt.Sprintf("%s\x00%d\x00%s", results[i].File, fs.Line, fs.Selector)
		if p, ok := seen[key]; ok {
			results[i].Proposal = p
			continue
		}
		if stopped || requests >= maxSelectorProposals {
			skipped++
			continue
		}
		requests++
		// The failure text can hold a resolved environment value: the model gets it masked, like triage does, and what
		// comes back is masked again before it reaches the report.
		a := ai.ProposeSelector(hctx, c, ai.SelectorFailure{
			Selector: fs.Selector,
			Step:     fs.Step,
			Error:    sysdialog.ScrubKnown(results[i].Error.Error()),
		}, cfg.AI.Timeout)
		p := &runner.SelectorProposal{
			Advisory:         selectorProposalAdvisory,
			Line:             fs.Line,
			FailedSelector:   fs.Selector,
			ProposedSelector: sysdialog.ScrubKnown(a.Proposed),
			Unavailable:      sysdialog.ScrubKnown(a.Unavailable),
			Model:            ai.ModelName(cfg.AI.Provider, cfg.AI.Model),
			Provider:         cfg.AI.Provider,
			Location:         ai.Location(cfg.AI.Provider, endpoint),
		}
		seen[key] = p
		results[i].Proposal = p
		if a.Proposed != "" {
			fmt.Fprintf(w, "  AI selector proposal (advisory, not applied, not verified) for %s line %d: %s -> %s\n", results[i].File, fs.Line, p.FailedSelector, p.ProposedSelector)
		} else {
			fmt.Fprintf(w, "  AI selector proposal unavailable for %s line %d: %s (tests are unaffected)\n", results[i].File, fs.Line, p.Unavailable)
		}
		stopped = a.ProviderFailed
	}
	if skipped > 0 {
		fmt.Fprintf(w, "  AI selector proposal: %d more failed selector step(s) were not sent (at most %d requests per run, none after a failed request)\n", skipped, maxSelectorProposals)
	}
}

// withoutProposals returns a copy of results with no selector proposal. An uploaded report is built from it, so the
// upload carries exactly what it carried before proposals existed: model text stays in the local report.
func withoutProposals(results []runner.TestResult) []runner.TestResult {
	out := make([]runner.TestResult, len(results))
	for i, r := range results {
		r.Proposal = nil
		out[i] = r
	}
	return out
}
