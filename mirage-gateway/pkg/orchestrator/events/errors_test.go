package events

import (
	"errors"
	"strings"
	"testing"
)

func TestErrValidationError(t *testing.T) {
	e := &ErrValidation{Field: "event_id", Message: "must not be empty"}
	if !strings.Contains(e.Error(), "event_id") {
		t.Fatal("ErrValidation.Error() should contain field name")
	}
	if !strings.Contains(e.Error(), "must not be empty") {
		t.Fatal("ErrValidation.Error() should contain message")
	}
}

func TestErrInvalidEventTypeError(t *testing.T) {
	e := &ErrInvalidEventType{Value: "bad.type"}
	if !strings.Contains(e.Error(), "bad.type") {
		t.Fatal("ErrInvalidEventType.Error() should contain value")
	}
}

func TestErrInvalidScopeError(t *testing.T) {
	e := &ErrInvalidScope{Value: "BadScope"}
	if !strings.Contains(e.Error(), "BadScope") {
		t.Fatal("ErrInvalidScope.Error() should contain value")
	}
}

func TestErrHandlerNotRegisteredError(t *testing.T) {
	e := &ErrHandlerNotRegistered{EventType: EventPersonaFlip}
	if !strings.Contains(e.Error(), string(EventPersonaFlip)) {
		t.Fatal("ErrHandlerNotRegistered.Error() should contain event type")
	}
}

func TestErrDuplicateRegistrationError(t *testing.T) {
	e := &ErrDuplicateRegistration{EventType: EventRollbackRequest}
	if !strings.Contains(e.Error(), string(EventRollbackRequest)) {
		t.Fatal("ErrDuplicateRegistration.Error() should contain event type")
	}
}

func TestErrEpochStaleError(t *testing.T) {
	e := &ErrEpochStale{EventEpoch: 5, CurrentEpoch: 10}
	s := e.Error()
	if !strings.Contains(s, "5") || !strings.Contains(s, "10") {
		t.Fatal("ErrEpochStale.Error() should contain both epochs")
	}
}

func TestErrDispatchFailedErrorAndUnwrap(t *testing.T) {
	cause := errors.New("handler boom")
	e := &ErrDispatchFailed{
		EventID:   "abc-123",
		EventType: EventBudgetReject,
		Cause:     cause,
	}
	s := e.Error()
	if !strings.Contains(s, "abc-123") {
		t.Fatal("ErrDispatchFailed.Error() should contain event_id")
	}
	if !strings.Contains(s, string(EventBudgetReject)) {
		t.Fatal("ErrDispatchFailed.Error() should contain event_type")
	}
	if !strings.Contains(s, "handler boom") {
		t.Fatal("ErrDispatchFailed.Error() should contain cause message")
	}
	if e.Unwrap() != cause {
		t.Fatal("ErrDispatchFailed.Unwrap() should return Cause")
	}
}
