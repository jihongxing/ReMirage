package budget

import (
	"strings"
	"testing"
)

func TestErrBudgetDenied_Error(t *testing.T) {
	err := &ErrBudgetDenied{
		Verdict: VerdictDenyAndHold,
		Reason:  "cost exceeds budget by 30%",
	}
	msg := err.Error()
	if !strings.Contains(msg, string(VerdictDenyAndHold)) {
		t.Errorf("Error() should contain verdict %q, got %q", VerdictDenyAndHold, msg)
	}
	if !strings.Contains(msg, "cost exceeds budget by 30%") {
		t.Errorf("Error() should contain reason, got %q", msg)
	}
}

func TestErrBudgetDenied_ErrorSuspend(t *testing.T) {
	err := &ErrBudgetDenied{
		Verdict: VerdictDenyAndSuspend,
		Reason:  "daily budget exceeded 150%",
	}
	msg := err.Error()
	if !strings.Contains(msg, string(VerdictDenyAndSuspend)) {
		t.Errorf("Error() should contain verdict %q, got %q", VerdictDenyAndSuspend, msg)
	}
	if !strings.Contains(msg, "daily budget exceeded 150%") {
		t.Errorf("Error() should contain reason, got %q", msg)
	}
}

func TestErrServiceClassDenied_Error(t *testing.T) {
	err := &ErrServiceClassDenied{
		ServiceClass: "Standard",
		DeniedMode:   "Hardened",
	}
	msg := err.Error()
	if !strings.Contains(msg, "Standard") {
		t.Errorf("Error() should contain service class, got %q", msg)
	}
	if !strings.Contains(msg, "Hardened") {
		t.Errorf("Error() should contain denied mode, got %q", msg)
	}
}

func TestErrServiceClassDenied_ErrorEscape(t *testing.T) {
	err := &ErrServiceClassDenied{
		ServiceClass: "Platinum",
		DeniedMode:   "Escape",
	}
	msg := err.Error()
	if !strings.Contains(msg, "Platinum") {
		t.Errorf("Error() should contain service class, got %q", msg)
	}
	if !strings.Contains(msg, "Escape") {
		t.Errorf("Error() should contain denied mode, got %q", msg)
	}
}

func TestErrInvalidBudgetProfile_Error(t *testing.T) {
	err := &ErrInvalidBudgetProfile{
		Field:   "latency_budget_ms",
		Message: "must be > 0",
	}
	msg := err.Error()
	if !strings.Contains(msg, "latency_budget_ms") {
		t.Errorf("Error() should contain field name, got %q", msg)
	}
	if !strings.Contains(msg, "must be > 0") {
		t.Errorf("Error() should contain message, got %q", msg)
	}
}
