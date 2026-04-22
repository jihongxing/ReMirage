package budget

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty9_ErrBudgetDenied_ErrorContainsFields
// Feature: v2-budget-engine, Property 9: Error message content correctness
// **Validates: Requirements 10.4**
// For any ErrBudgetDenied, Error() must contain the verdict value and reason value.
func TestProperty9_ErrBudgetDenied_ErrorContainsFields(t *testing.T) {
	verdicts := []BudgetVerdict{
		VerdictAllow, VerdictAllowDegraded, VerdictAllowWithCharge,
		VerdictDenyAndHold, VerdictDenyAndSuspend,
	}

	rapid.Check(t, func(t *rapid.T) {
		verdict := verdicts[rapid.IntRange(0, len(verdicts)-1).Draw(t, "verdict_idx")]
		reason := rapid.StringMatching(`[a-zA-Z0-9 _\-]{1,100}`).Draw(t, "reason")

		err := &ErrBudgetDenied{
			Verdict: verdict,
			Reason:  reason,
		}
		msg := err.Error()

		if !strings.Contains(msg, string(verdict)) {
			t.Fatalf("Error() %q does not contain verdict %q", msg, verdict)
		}
		if !strings.Contains(msg, reason) {
			t.Fatalf("Error() %q does not contain reason %q", msg, reason)
		}
	})
}

// TestProperty9_ErrServiceClassDenied_ErrorContainsFields
// Feature: v2-budget-engine, Property 9: Error message content correctness
// **Validates: Requirements 10.5**
// For any ErrServiceClassDenied, Error() must contain the service_class value and denied_mode value.
func TestProperty9_ErrServiceClassDenied_ErrorContainsFields(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		serviceClass := rapid.StringMatching(`[A-Za-z]{1,30}`).Draw(t, "service_class")
		deniedMode := rapid.StringMatching(`[A-Za-z]{1,30}`).Draw(t, "denied_mode")

		err := &ErrServiceClassDenied{
			ServiceClass: serviceClass,
			DeniedMode:   deniedMode,
		}
		msg := err.Error()

		if !strings.Contains(msg, serviceClass) {
			t.Fatalf("Error() %q does not contain service_class %q", msg, serviceClass)
		}
		if !strings.Contains(msg, deniedMode) {
			t.Fatalf("Error() %q does not contain denied_mode %q", msg, deniedMode)
		}
	})
}
