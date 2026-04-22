package audit

import "fmt"

// ErrAuditRecordNotFound 审计记录不存在
type ErrAuditRecordNotFound struct {
	TxID string
}

func (e *ErrAuditRecordNotFound) Error() string {
	return fmt.Sprintf("audit record not found for tx_id: %s", e.TxID)
}

// ErrSessionNotFound Session 不存在
type ErrSessionNotFound struct {
	SessionID string
}

func (e *ErrSessionNotFound) Error() string {
	return fmt.Sprintf("session not found: %s", e.SessionID)
}

// ErrTransactionNotFound 事务不存在
type ErrTransactionNotFound struct {
	TxID string
}

func (e *ErrTransactionNotFound) Error() string {
	return fmt.Sprintf("transaction not found: %s", e.TxID)
}

// ErrInvalidAuditRecord 无效审计记录
type ErrInvalidAuditRecord struct {
	Field   string
	Message string
}

func (e *ErrInvalidAuditRecord) Error() string {
	return fmt.Sprintf("invalid audit record field %s: %s", e.Field, e.Message)
}
