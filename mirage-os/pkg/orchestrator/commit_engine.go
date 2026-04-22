// Package orchestrator - 事务化替补引擎
// 节点摘除、热备接替、拓扑发布、路由更新引入事务化控制
package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"
)

// TxStatus 事务状态
type TxStatus int

const (
	TxStatusPending    TxStatus = iota // 待执行
	TxStatusCommitted                  // 已提交
	TxStatusRolledBack                 // 已回滚
	TxStatusFailed                     // 失败
)

// String 返回状态名称
func (s TxStatus) String() string {
	switch s {
	case TxStatusPending:
		return "PENDING"
	case TxStatusCommitted:
		return "COMMITTED"
	case TxStatusRolledBack:
		return "ROLLED_BACK"
	case TxStatusFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// TxStep 事务步骤
type TxStep struct {
	Name     string
	Action   func(ctx context.Context) error
	Rollback func(ctx context.Context) error
	Done     bool
}

// ReplacementTx 替换事务
type ReplacementTx struct {
	TxID       string
	OldGateway string
	NewGateway string
	CellID     string
	Steps      []TxStep
	Status     TxStatus
	CreatedAt  time.Time
	Error      string
}

// CommitEngine 事务化替补引擎
type CommitEngine struct {
	txLog map[string]*ReplacementTx // txID → tx
	mu    sync.Mutex

	// 外部依赖（通过回调注入）
	removeNode    func(ctx context.Context, gwID string) error
	activateNode  func(ctx context.Context, gwID string) error
	updateTopo    func(ctx context.Context, cellID string) error
	publishRoutes func(ctx context.Context, cellID string) error
}

// NewCommitEngine 创建事务化替补引擎
func NewCommitEngine() *CommitEngine {
	return &CommitEngine{
		txLog: make(map[string]*ReplacementTx),
	}
}

// SetCallbacks 设置事务步骤回调
func (e *CommitEngine) SetCallbacks(
	removeNode func(ctx context.Context, gwID string) error,
	activateNode func(ctx context.Context, gwID string) error,
	updateTopo func(ctx context.Context, cellID string) error,
	publishRoutes func(ctx context.Context, cellID string) error,
) {
	e.removeNode = removeNode
	e.activateNode = activateNode
	e.updateTopo = updateTopo
	e.publishRoutes = publishRoutes
}

// BuildReplacementTx 构造标准替换事务
func (e *CommitEngine) BuildReplacementTx(oldGW, newGW, cellID string) *ReplacementTx {
	tx := &ReplacementTx{
		TxID:       generateTxID(),
		OldGateway: oldGW,
		NewGateway: newGW,
		CellID:     cellID,
		Status:     TxStatusPending,
		CreatedAt:  time.Now(),
	}

	tx.Steps = []TxStep{
		{
			Name: "remove_old_node",
			Action: func(ctx context.Context) error {
				if e.removeNode != nil {
					return e.removeNode(ctx, oldGW)
				}
				return nil
			},
			Rollback: func(ctx context.Context) error {
				if e.activateNode != nil {
					return e.activateNode(ctx, oldGW)
				}
				return nil
			},
		},
		{
			Name: "activate_new_node",
			Action: func(ctx context.Context) error {
				if e.activateNode != nil {
					return e.activateNode(ctx, newGW)
				}
				return nil
			},
			Rollback: func(ctx context.Context) error {
				if e.removeNode != nil {
					return e.removeNode(ctx, newGW)
				}
				return nil
			},
		},
		{
			Name: "update_topology",
			Action: func(ctx context.Context) error {
				if e.updateTopo != nil {
					return e.updateTopo(ctx, cellID)
				}
				return nil
			},
		},
		{
			Name: "publish_routes",
			Action: func(ctx context.Context) error {
				if e.publishRoutes != nil {
					return e.publishRoutes(ctx, cellID)
				}
				return nil
			},
		},
	}

	return tx
}

// ExecuteReplacement 执行替换事务
func (e *CommitEngine) ExecuteReplacement(ctx context.Context, oldGW, newGW, cellID string) error {
	tx := e.BuildReplacementTx(oldGW, newGW, cellID)

	e.mu.Lock()
	e.txLog[tx.TxID] = tx
	e.mu.Unlock()

	log.Printf("[CommitEngine] 开始执行替换事务: txID=%s, %s → %s", tx.TxID, oldGW, newGW)

	if err := e.executeTx(ctx, tx); err != nil {
		return fmt.Errorf("tx %s failed: %w", tx.TxID, err)
	}

	return nil
}

// executeTx 执行事务步骤
func (e *CommitEngine) executeTx(ctx context.Context, tx *ReplacementTx) error {
	for i, step := range tx.Steps {
		log.Printf("[CommitEngine] 执行步骤 %d/%d: %s", i+1, len(tx.Steps), step.Name)
		if err := step.Action(ctx); err != nil {
			tx.Error = fmt.Sprintf("step %s failed: %v", step.Name, err)
			log.Printf("[CommitEngine] ❌ 步骤 %s 失败: %v，开始回滚", step.Name, err)

			// 回滚已执行的步骤
			for j := i - 1; j >= 0; j-- {
				if tx.Steps[j].Rollback != nil && tx.Steps[j].Done {
					if rbErr := tx.Steps[j].Rollback(ctx); rbErr != nil {
						log.Printf("[CommitEngine] ⚠️ 回滚步骤 %s 失败: %v", tx.Steps[j].Name, rbErr)
					}
				}
			}
			tx.Status = TxStatusRolledBack
			return err
		}
		tx.Steps[i].Done = true
	}

	tx.Status = TxStatusCommitted
	log.Printf("[CommitEngine] ✅ 事务 %s 已提交", tx.TxID)
	return nil
}

// RetryTx 重试失败的事务
func (e *CommitEngine) RetryTx(ctx context.Context, txID string) error {
	e.mu.Lock()
	tx, ok := e.txLog[txID]
	e.mu.Unlock()

	if !ok {
		return fmt.Errorf("transaction %s not found", txID)
	}

	if tx.Status == TxStatusCommitted {
		return fmt.Errorf("transaction %s already committed", txID)
	}

	// 重置步骤状态
	for i := range tx.Steps {
		tx.Steps[i].Done = false
	}
	tx.Status = TxStatusPending
	tx.Error = ""

	log.Printf("[CommitEngine] 重试事务: %s", txID)
	return e.executeTx(ctx, tx)
}

// GetTxLog 获取事务日志
func (e *CommitEngine) GetTxLog() map[string]*ReplacementTx {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make(map[string]*ReplacementTx, len(e.txLog))
	for k, v := range e.txLog {
		result[k] = v
	}
	return result
}

func generateTxID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
