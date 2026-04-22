package audit

import (
	"strings"
	"testing"
)

func TestErrAuditRecordNotFound(t *testing.T) {
	err := &ErrAuditRecordNotFound{TxID: "tx-123"}
	msg := err.Error()
	if !strings.Contains(msg, "tx-123") {
		t.Errorf("expected error to contain tx_id, got: %s", msg)
	}
	if !strings.Contains(msg, "audit record not found") {
		t.Errorf("expected error to describe audit record not found, got: %s", msg)
	}
}

func TestErrSessionNotFound(t *testing.T) {
	err := &ErrSessionNotFound{SessionID: "sess-456"}
	msg := err.Error()
	if !strings.Contains(msg, "sess-456") {
		t.Errorf("expected error to contain session_id, got: %s", msg)
	}
	if !strings.Contains(msg, "session not found") {
		t.Errorf("expected error to describe session not found, got: %s", msg)
	}
}

func TestErrTransactionNotFound(t *testing.T) {
	err := &ErrTransactionNotFound{TxID: "tx-789"}
	msg := err.Error()
	if !strings.Contains(msg, "tx-789") {
		t.Errorf("expected error to contain tx_id, got: %s", msg)
	}
	if !strings.Contains(msg, "transaction not found") {
		t.Errorf("expected error to describe transaction not found, got: %s", msg)
	}
}

func TestErrInvalidAuditRecord(t *testing.T) {
	err := &ErrInvalidAuditRecord{Field: "audit_id", Message: "must not be empty"}
	msg := err.Error()
	if !strings.Contains(msg, "audit_id") {
		t.Errorf("expected error to contain field name, got: %s", msg)
	}
	if !strings.Contains(msg, "must not be empty") {
		t.Errorf("expected error to contain message, got: %s", msg)
	}
}
