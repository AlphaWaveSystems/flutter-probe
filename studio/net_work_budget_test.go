package main

import (
	"strings"
	"testing"
)

func TestNamedAssignmentsFailClosedWhenWorkBudgetIsExceeded(t *testing.T) {
	input := strings.Repeat("a:b=", 16000) + "password=plain-secret"
	if got := redactNamedAssignments(input, nil); got != redactedMark {
		t.Fatalf("expensive nested assignments should hide the whole field, got %q", got[:min(len(got), 100)])
	}
	if got := redactNamedAssignments("ordinary=visible password=plain-secret", nil); !strings.Contains(got, "ordinary=visible") || strings.Contains(got, "plain-secret") {
		t.Fatalf("ordinary assignments should retain safe text and hide secrets: %q", got)
	}
}
