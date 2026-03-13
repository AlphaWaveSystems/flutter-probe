// Package ai provides AI-assisted capabilities for FlutterProbe:
//   - Self-healing selectors (find broken locators and suggest repairs)
//   - AI test generation from recordings
package ai

import (
	"fmt"
	"sort"
	"strings"
)

// ---- Self-Healing Selectors ----

// WidgetInfo is lightweight widget metadata returned by the agent.
type WidgetInfo struct {
	Type      string
	Key       string
	Text      string
	Hint      string
	Semantics string
	X, Y      float64
	Width, Height float64
	Depth     int
}

// HealResult is the output of TryHeal.
type HealResult struct {
	OriginalSelector string
	HealedSelector   string
	Confidence       float64 // 0.0 – 1.0
	Strategy         string  // text_fuzzy | key_partial | type_position | semantic
	Explanation      string
}

// SelfHealer attempts to repair a broken selector by fuzzy-matching
// against the live widget tree.
type SelfHealer struct {
	// FuzzyThreshold is the minimum similarity score to accept a healed match (0.0–1.0).
	FuzzyThreshold float64
}

// NewSelfHealer creates a SelfHealer.
func NewSelfHealer() *SelfHealer {
	return &SelfHealer{FuzzyThreshold: 0.7}
}

// TryHeal attempts to find a replacement selector for a broken one.
// [tree] is the current live widget tree (list of all visible widgets).
func (h *SelfHealer) TryHeal(brokenSelector, brokenKind string, tree []WidgetInfo) (*HealResult, error) {
	if len(tree) == 0 {
		return nil, fmt.Errorf("selfheal: empty widget tree")
	}

	type candidate struct {
		widget WidgetInfo
		score  float64
		sel    string
		strat  string
		expl   string
	}
	var candidates []candidate

	for _, w := range tree {
		// Strategy 1: fuzzy text match
		if brokenKind == "text" && w.Text != "" {
			score := fuzzyScore(brokenSelector, w.Text)
			if score >= h.FuzzyThreshold {
				candidates = append(candidates, candidate{
					widget: w,
					score:  score,
					sel:    fmt.Sprintf("%q", w.Text),
					strat:  "text_fuzzy",
					expl:   fmt.Sprintf("Text changed from %q to %q (%.0f%% similar)", brokenSelector, w.Text, score*100),
				})
			}
		}

		// Strategy 2: partial key match
		if brokenKind == "id" && w.Key != "" {
			cleaned := strings.TrimPrefix(brokenSelector, "#")
			score := fuzzyScore(cleaned, w.Key)
			if strings.Contains(w.Key, cleaned) || strings.Contains(cleaned, w.Key) || score >= h.FuzzyThreshold {
				candidates = append(candidates, candidate{
					widget: w,
					score:  score,
					sel:    "#" + w.Key,
					strat:  "key_partial",
					expl:   fmt.Sprintf("Key %q partially matches broken selector %q", w.Key, brokenSelector),
				})
			}
		}


		// Strategy 3: semantic label match
		if w.Semantics != "" {
			score := fuzzyScore(brokenSelector, w.Semantics)
			if score >= h.FuzzyThreshold {
				candidates = append(candidates, candidate{
					widget: w,
					score:  score * 0.9, // slight penalty for semantic vs. direct
					sel:    fmt.Sprintf("%q", w.Semantics),
					strat:  "semantic",
					expl:   fmt.Sprintf("Semantic label %q matches broken selector %q", w.Semantics, brokenSelector),
				})
			}
		}

		// Strategy 4: type + position (fallback)
		if w.Type != "" && brokenKind == "type" {
			if strings.EqualFold(w.Type, brokenSelector) {
				candidates = append(candidates, candidate{
					widget: w,
					score:  0.8,
					sel:    w.Type,
					strat:  "type_position",
					expl:   "Widget type matched directly",
				})
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("selfheal: no replacement found for %q", brokenSelector)
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	best := candidates[0]
	return &HealResult{
		OriginalSelector: brokenSelector,
		HealedSelector:   best.sel,
		Confidence:       best.score,
		Strategy:         best.strat,
		Explanation:      best.expl,
	}, nil
}

// ---- AI Test Generation ----

// RecordingEvent is a captured user interaction from probe record.
type RecordingEvent struct {
	Action    string            // tap | type | swipe | see
	Target    string            // text / id / type
	TargetID  string
	Text      string            // for type events
	Direction string            // for swipe
	Metadata  map[string]string
}

// TestGenerator generates ProbeScript tests from recordings.
type TestGenerator struct {
	// ModelEndpoint is the AI backend URL (empty = local heuristic mode).
	ModelEndpoint string
	APIKey        string
}

// NewTestGenerator creates a TestGenerator.
func NewTestGenerator() *TestGenerator {
	return &TestGenerator{}
}

// GenerateFromRecording converts a list of recording events into a .probe file.
// When ModelEndpoint is empty, uses heuristic generation.
func (g *TestGenerator) GenerateFromRecording(name string, events []RecordingEvent) string {
	if g.ModelEndpoint != "" {
		// AI mode: send to model API
		return g.aiGenerate(name, events)
	}
	return g.heuristicGenerate(name, events)
}

// heuristicGenerate uses rule-based logic to produce clean ProbeScript.
func (g *TestGenerator) heuristicGenerate(name string, events []RecordingEvent) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("test %q\n", name))
	sb.WriteString("  open the app\n")

	prev := ""
	for _, e := range events {
		var line string
		switch e.Action {
		case "tap":
			if e.TargetID != "" {
				line = fmt.Sprintf("  tap #%s", e.TargetID)
			} else if e.Target != "" {
				line = fmt.Sprintf("  tap %q", e.Target)
			}
		case "type":
			if e.TargetID != "" {
				line = fmt.Sprintf("  type %q into #%s", e.Text, e.TargetID)
			} else if e.Target != "" {
				line = fmt.Sprintf("  type %q into the %q field", e.Text, e.Target)
			} else {
				line = fmt.Sprintf("  type %q", e.Text)
			}
		case "swipe":
			line = fmt.Sprintf("  swipe %s", e.Direction)
		case "scroll":
			line = fmt.Sprintf("  scroll %s", e.Direction)
		case "see":
			line = fmt.Sprintf("  see %q", e.Target)
		case "navigate":
			line = "  go back"
		case "wait":
			line = "  wait for the page to load"
		}

		// Deduplicate consecutive identical lines
		if line != "" && line != prev {
			sb.WriteString(line + "\n")
			prev = line

			// Auto-insert assertions after navigation events
			if e.Action == "tap" && e.Target != "" {
				sb.WriteString("  wait for the page to load\n")
			}
		}
	}

	return sb.String()
}

// aiGenerate calls a model API to produce natural, well-structured tests.
// This is a stub — in P4 this calls the configured AI endpoint.
func (g *TestGenerator) aiGenerate(name string, events []RecordingEvent) string {
	// Build a prompt describing the recording
	var prompt strings.Builder
	prompt.WriteString("Convert these recorded user interactions into a clean ProbeScript test.\n\n")
	prompt.WriteString(fmt.Sprintf("Test name: %q\n\nEvents:\n", name))
	for i, e := range events {
		prompt.WriteString(fmt.Sprintf("  %d. %s %q\n", i+1, e.Action, e.Target))
	}
	prompt.WriteString("\nGenerate a well-structured .probe file with:\n")
	prompt.WriteString("- Appropriate wait steps for navigation\n")
	prompt.WriteString("- Assertions after key actions\n")
	prompt.WriteString("- Descriptive test name\n")

	// TODO: POST to g.ModelEndpoint with the prompt + API key
	// For now, fall back to heuristic
	return g.heuristicGenerate(name, events)
}

// ---- String similarity ----

// fuzzyScore computes a similarity score between two strings (0.0–1.0).
// Uses a combination of exact, prefix, and Levenshtein-based scoring.
func fuzzyScore(a, b string) float64 {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))

	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}

	// Prefix match
	shorter, longer := a, b
	if len(b) < len(a) {
		shorter, longer = b, a
	}
	if strings.HasPrefix(longer, shorter) {
		return 0.9
	}
	if strings.Contains(longer, shorter) {
		return 0.8
	}

	// Levenshtein similarity
	dist := levenshtein(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			if ra[i-1] == rb[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				dp[i][j] = 1 + min3(dp[i-1][j], dp[i][j-1], dp[i-1][j-1])
			}
		}
	}
	return dp[la][lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
