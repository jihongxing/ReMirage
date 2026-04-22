package events

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── 11.1 并发 Dispatch ──

func TestConcurrentDispatch(t *testing.T) {
	reg := NewEventRegistry()
	h := &mockHandler{et: EventBudgetReject}
	reg.Register(h)
	dedup := NewDeduplicationStore()
	ep := &mockEpochProvider{epoch: 1}
	disp := NewEventDispatcher(reg, dedup, ep)

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev := NewBudgetRejectEvent("concurrent-src", "ref", 0)
			if err := disp.Dispatch(context.Background(), ev); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent dispatch error: %v", err)
	}
}

// ── 11.2 并发 Registry 读写 ──

func TestConcurrentRegistry(t *testing.T) {
	reg := NewEventRegistry()
	var wg sync.WaitGroup

	// register all types first
	for _, et := range AllEventTypes {
		h := &mockHandler{et: et}
		reg.Register(h)
	}

	// concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.ListRegistered()
			for _, et := range AllEventTypes {
				reg.IsRegistered(et)
				reg.GetHandler(et)
			}
		}()
	}
	wg.Wait()
}

// ── 11.3 并发 DeduplicationStore ──

func TestConcurrentDedup(t *testing.T) {
	dedup := NewDeduplicationStore()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			eid := "event-" + string(rune('A'+id%26))
			dedup.Add(eid)
			dedup.Contains(eid)
			dedup.Cleanup()
		}(i)
	}
	wg.Wait()
}

// ── 12.1 端到端集成测试 ──

func TestEndToEndEventFlow(t *testing.T) {
	reg := NewEventRegistry()
	dedup := NewDeduplicationStore()
	ep := &mockEpochProvider{epoch: 5}
	disp := NewEventDispatcher(reg, dedup, ep)

	// 1. 工厂创建
	ev := NewRollbackRequestEvent("commit-engine", "tx-123", 10)
	if ev == nil {
		t.Fatal("factory returned nil")
	}

	// 2. 校验
	if err := ev.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// 3. 注册 Handler
	var handlerCalled atomic.Bool
	h := &mockHandler{
		et: EventRollbackRequest,
		returnFn: func() error {
			handlerCalled.Store(true)
			return nil
		},
	}
	if err := reg.Register(h); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// 4. 分发 → Handler 执行
	if err := disp.Dispatch(context.Background(), ev); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if !handlerCalled.Load() {
		t.Fatal("handler should have been called")
	}

	// 5. 幂等重放（RollbackRequest is idempotent）
	handlerCalled.Store(false)
	if err := disp.Dispatch(context.Background(), ev); err != nil {
		t.Fatalf("idempotent replay should return nil, got: %v", err)
	}
	if handlerCalled.Load() {
		t.Fatal("handler should NOT be called on idempotent replay")
	}

	// 6. epoch 拒绝
	staleEv := NewRollbackRequestEvent("commit-engine", "tx-456", 3) // epoch 3 < current 5
	err := disp.Dispatch(context.Background(), staleEv)
	var se *ErrEpochStale
	if !errors.As(err, &se) {
		t.Fatalf("expected ErrEpochStale, got %v", err)
	}

	// 7. 异步分发测试 (BudgetReject: requires_ack=false)
	asyncDone := make(chan struct{})
	asyncH := &mockHandler{
		et: EventBudgetReject,
		returnFn: func() error {
			close(asyncDone)
			return nil
		},
	}
	reg.Register(asyncH)
	asyncEv := NewBudgetRejectEvent("budget-engine", "budget-ref", 0)
	if err := disp.Dispatch(context.Background(), asyncEv); err != nil {
		t.Fatalf("async dispatch should return nil: %v", err)
	}
	select {
	case <-asyncDone:
		// handler executed asynchronously
	case <-time.After(2 * time.Second):
		t.Fatal("async handler did not execute within timeout")
	}
}
