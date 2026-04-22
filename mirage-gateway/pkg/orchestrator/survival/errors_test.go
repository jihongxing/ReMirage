package survival

import (
	"strings"
	"testing"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

func TestErrInvalidTransition_Error(t *testing.T) {
	err := &ErrInvalidTransition{
		From: orchestrator.SurvivalModeNormal,
		To:   orchestrator.SurvivalModeLastResort,
	}
	msg := err.Error()
	if !strings.Contains(msg, "Normal") {
		t.Errorf("expected error to contain 'Normal', got %q", msg)
	}
	if !strings.Contains(msg, "LastResort") {
		t.Errorf("expected error to contain 'LastResort', got %q", msg)
	}
}

func TestErrConstraintViolation_Error(t *testing.T) {
	err := &ErrConstraintViolation{
		ConstraintType: "min_dwell_time",
		Remaining:      30 * time.Second,
	}
	msg := err.Error()
	if !strings.Contains(msg, "min_dwell_time") {
		t.Errorf("expected error to contain 'min_dwell_time', got %q", msg)
	}
	if !strings.Contains(msg, "30s") {
		t.Errorf("expected error to contain '30s', got %q", msg)
	}
}

func TestErrAdmissionDenied_Error(t *testing.T) {
	err := &ErrAdmissionDenied{
		Policy:       AdmissionClosed,
		ServiceClass: orchestrator.ServiceClassDiamond,
	}
	msg := err.Error()
	if !strings.Contains(msg, "Closed") {
		t.Errorf("expected error to contain 'Closed', got %q", msg)
	}
	if !strings.Contains(msg, "Diamond") {
		t.Errorf("expected error to contain 'Diamond', got %q", msg)
	}
}
