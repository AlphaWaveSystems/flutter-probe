package runner

import (
	"testing"
	"time"
)

// TestReconnectDelay_Monotonic verifies the unjittered delay doubles per
// attempt and saturates at 8s.
func TestReconnectDelay_Monotonic(t *testing.T) {
	const base = 1 * time.Second
	const cap = 8 * time.Second

	tests := []struct {
		attempt    int
		wantTarget time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, cap},  // already at cap
		{10, cap}, // far past cap
	}

	for _, tc := range tests {
		// Run multiple times to bound the jitter; assert ±20% of target.
		for i := 0; i < 50; i++ {
			got := reconnectDelay(base, tc.attempt)
			lo := tc.wantTarget * 8 / 10
			hi := tc.wantTarget * 12 / 10
			if got < lo || got >= hi {
				t.Errorf("attempt=%d: got %s, want in [%s, %s)", tc.attempt, got, lo, hi)
				break
			}
		}
	}
}

// TestReconnectDelay_AttemptZeroOrNegative ensures the function does not
// panic on degenerate inputs and treats them as attempt=1.
func TestReconnectDelay_AttemptZeroOrNegative(t *testing.T) {
	const base = 1 * time.Second
	for _, attempt := range []int{0, -1, -100} {
		got := reconnectDelay(base, attempt)
		// Must be in [base*0.8, base*1.2) — ±20% of attempt-1 baseline.
		lo := base * 8 / 10
		hi := base * 12 / 10
		if got < lo || got >= hi {
			t.Errorf("attempt=%d: got %s, want in [%s, %s)", attempt, got, lo, hi)
		}
	}
}

// TestReconnectDelay_BudgetUnderCap verifies the cumulative delay across a
// 4-attempt run stays within the documented ~15s budget (15 ± jitter).
// 1+2+4+8 = 15s, ±20% jitter per term → worst case 18s, best case 12s.
func TestReconnectDelay_BudgetUnderCap(t *testing.T) {
	const base = 1 * time.Second
	for trial := 0; trial < 20; trial++ {
		var total time.Duration
		for a := 1; a <= 4; a++ {
			total += reconnectDelay(base, a)
		}
		const minBudget = 12 * time.Second
		const maxBudget = 18 * time.Second
		if total < minBudget || total > maxBudget {
			t.Errorf("4-attempt total %s outside [%s, %s]", total, minBudget, maxBudget)
		}
	}
}
