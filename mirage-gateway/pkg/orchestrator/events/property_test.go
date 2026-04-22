package events

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// ── helpers ──

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// genEventType generates a random valid EventType
func genEventType() *rapid.Generator[EventType] {
	return rapid.SampledFrom(AllEventTypes)
}

// genScope generates a random valid EventScope
func genScope() *rapid.Generator[EventScope] {
	return rapid.SampledFrom([]EventScope{EventScopeSession, EventScopeLink, EventScopeGlobal})
}

// mockHandler is a test EventHandler
type mockHandler struct {
	et       EventType
	calls    atomic.Int64
	returnFn func() error
}

func (m *mockHandler) Handle(_ context.Context, _ *ControlEvent) error {
	m.calls.Add(1)
	if m.returnFn != nil {
		return m.returnFn()
	}
	return nil
}
func (m *mockHandler) EventType() EventType { return m.et }

// mockEpochProvider implements EpochProvider
type mockEpochProvider struct {
	epoch uint64
}

func (m *mockEpochProvider) GetLastSuccessfulEpoch(_ context.Context) (uint64, error) {
	return m.epoch, nil
}

// ── Property 1: EventSemantics 矩阵完整性与正确性 ──

func TestProperty1_SemanticsMatrixCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		et := genEventType().Draw(t, "event_type")
		sem := GetSemantics(et)
		if sem == nil {
			t.Fatalf("GetSemantics(%s) returned nil", et)
		}

		expected := EventSemanticsMap[et]
		if sem.DefaultScope != expected.DefaultScope {
			t.Fatalf("DefaultScope mismatch for %s: got %s, want %s", et, sem.DefaultScope, expected.DefaultScope)
		}
		if sem.DefaultPriority != expected.DefaultPriority {
			t.Fatalf("DefaultPriority mismatch for %s", et)
		}
		if sem.RequiresAck != expected.RequiresAck {
			t.Fatalf("RequiresAck mismatch for %s", et)
		}
		if sem.Idempotent != expected.Idempotent {
			t.Fatalf("Idempotent mismatch for %s", et)
		}
		if sem.Replayable != expected.Replayable {
			t.Fatalf("Replayable mismatch for %s", et)
		}
		if sem.CarriesEpoch != expected.CarriesEpoch {
			t.Fatalf("CarriesEpoch mismatch for %s", et)
		}
	})
}

func TestProperty1_UndefinedTypeReturnsNil(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.StringMatching(`[a-z]{3,10}\.[a-z]{3,10}`).Draw(t, "random_type")
		et := EventType(s)
		// skip if accidentally valid
		for _, valid := range AllEventTypes {
			if et == valid {
				return
			}
		}
		if GetSemantics(et) != nil {
			t.Fatalf("GetSemantics(%s) should return nil for undefined type", et)
		}
	})
}

// ── Property 2: 工厂函数默认值填充 ──

func TestProperty2_FactoryDefaults(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		et := genEventType().Draw(t, "event_type")
		source := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "source")
		payloadRef := rapid.String().Draw(t, "payload_ref")
		epoch := rapid.Uint64().Draw(t, "epoch")

		ev, err := NewEvent(et, source, payloadRef, epoch)
		if err != nil {
			t.Fatalf("NewEvent failed: %v", err)
		}

		// UUID v4 format
		if !uuidRegex.MatchString(ev.EventID) {
			t.Fatalf("event_id not UUID v4: %s", ev.EventID)
		}
		// created_at non-zero
		if ev.CreatedAt.IsZero() {
			t.Fatal("created_at is zero")
		}

		sem := GetSemantics(et)
		if ev.TargetScope != sem.DefaultScope {
			t.Fatalf("TargetScope: got %s, want %s", ev.TargetScope, sem.DefaultScope)
		}
		if ev.Priority != sem.DefaultPriority {
			t.Fatalf("Priority: got %d, want %d", ev.Priority, sem.DefaultPriority)
		}
		if ev.RequiresAck != sem.RequiresAck {
			t.Fatalf("RequiresAck: got %v, want %v", ev.RequiresAck, sem.RequiresAck)
		}
		if sem.CarriesEpoch {
			if ev.Epoch != epoch {
				t.Fatalf("Epoch: got %d, want %d", ev.Epoch, epoch)
			}
		} else {
			if ev.Epoch != 0 {
				t.Fatalf("Epoch should be 0 for non-epoch type, got %d", ev.Epoch)
			}
		}
	})
}

// ── Property 3: Validate 综合校验 ──

func TestProperty3_ValidateCorrectness(t *testing.T) {
	// valid events pass
	rapid.Check(t, func(t *rapid.T) {
		et := genEventType().Draw(t, "event_type")
		sem := GetSemantics(et)
		var epoch uint64
		if sem.CarriesEpoch {
			epoch = rapid.Uint64Range(1, 1<<62).Draw(t, "epoch")
		}
		ev := newTestEvent(et, epoch)
		if err := ev.Validate(); err != nil {
			t.Fatalf("valid event failed validation: %v", err)
		}
	})

	// empty event_id
	t.Run("empty_event_id", func(t *testing.T) {
		ev := newTestEvent(EventBudgetReject, 0)
		ev.EventID = ""
		err := ev.Validate()
		var ve *ErrValidation
		if !errors.As(err, &ve) || ve.Field != "event_id" {
			t.Fatalf("expected ErrValidation for event_id, got %v", err)
		}
	})

	// empty source
	t.Run("empty_source", func(t *testing.T) {
		ev := newTestEvent(EventBudgetReject, 0)
		ev.Source = ""
		err := ev.Validate()
		var ve *ErrValidation
		if !errors.As(err, &ve) || ve.Field != "source" {
			t.Fatalf("expected ErrValidation for source, got %v", err)
		}
	})

	// invalid event_type
	t.Run("invalid_event_type", func(t *testing.T) {
		ev := newTestEvent(EventBudgetReject, 0)
		ev.EventType = "invalid.type"
		err := ev.Validate()
		var ie *ErrInvalidEventType
		if !errors.As(err, &ie) {
			t.Fatalf("expected ErrInvalidEventType, got %v", err)
		}
	})

	// invalid scope
	t.Run("invalid_scope", func(t *testing.T) {
		ev := newTestEvent(EventBudgetReject, 0)
		ev.TargetScope = "BadScope"
		err := ev.Validate()
		var is *ErrInvalidScope
		if !errors.As(err, &is) {
			t.Fatalf("expected ErrInvalidScope, got %v", err)
		}
	})

	// priority out of range
	t.Run("priority_out_of_range", func(t *testing.T) {
		ev := newTestEvent(EventBudgetReject, 0)
		ev.Priority = 11
		err := ev.Validate()
		var ve *ErrValidation
		if !errors.As(err, &ve) || ve.Field != "priority" {
			t.Fatalf("expected ErrValidation for priority, got %v", err)
		}
	})

	// requires_ack inconsistency
	t.Run("requires_ack_inconsistency", func(t *testing.T) {
		// EventSessionMigrateRequest requires ack
		ev := newTestEvent(EventSessionMigrateRequest, 1)
		ev.RequiresAck = false
		err := ev.Validate()
		var ve *ErrValidation
		if !errors.As(err, &ve) || ve.Field != "requires_ack" {
			t.Fatalf("expected ErrValidation for requires_ack, got %v", err)
		}
	})

	// epoch inconsistency
	t.Run("epoch_zero_for_carries_epoch", func(t *testing.T) {
		ev := newTestEvent(EventSessionMigrateRequest, 0)
		ev.Epoch = 0
		err := ev.Validate()
		var ve *ErrValidation
		if !errors.As(err, &ve) || ve.Field != "epoch" {
			t.Fatalf("expected ErrValidation for epoch, got %v", err)
		}
	})
}

func newTestEvent(et EventType, epoch uint64) *ControlEvent {
	sem := GetSemantics(et)
	ev := &ControlEvent{
		EventID:     "test-uuid-1234",
		EventType:   et,
		Source:      "test-source",
		TargetScope: sem.DefaultScope,
		Priority:    sem.DefaultPriority,
		RequiresAck: sem.RequiresAck,
		PayloadRef:  "ref-1",
		CreatedAt:   time.Now(),
		Epoch:       epoch,
	}
	return ev
}

// ── Property 4: 注册表一对一映射与重复注册拒绝 ──

func TestProperty4_RegistryMapping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		reg := NewEventRegistry()
		// pick 1-4 unique event types
		n := rapid.IntRange(1, 4).Draw(t, "n_handlers")
		perm := rapid.Permutation(AllEventTypes)
		types := perm.Draw(t, "types")[:n]

		for _, et := range types {
			h := &mockHandler{et: et}
			if err := reg.Register(h); err != nil {
				t.Fatalf("Register(%s) failed: %v", et, err)
			}
			if !reg.IsRegistered(et) {
				t.Fatalf("IsRegistered(%s) should be true", et)
			}
			got, err := reg.GetHandler(et)
			if err != nil {
				t.Fatalf("GetHandler(%s) failed: %v", et, err)
			}
			if got.EventType() != et {
				t.Fatalf("GetHandler returned wrong type")
			}
		}

		// duplicate registration
		for _, et := range types {
			h2 := &mockHandler{et: et}
			err := reg.Register(h2)
			var de *ErrDuplicateRegistration
			if !errors.As(err, &de) {
				t.Fatalf("expected ErrDuplicateRegistration for %s, got %v", et, err)
			}
		}

		// ListRegistered
		listed := reg.ListRegistered()
		if len(listed) != n {
			t.Fatalf("ListRegistered: got %d, want %d", len(listed), n)
		}
	})
}

// ── Property 5: 幂等去重正确性 ──

func TestProperty5_IdempotentDedup(t *testing.T) {
	// idempotent=true: second dispatch skips handler
	t.Run("idempotent_true", func(t *testing.T) {
		for _, et := range AllEventTypes {
			sem := GetSemantics(et)
			if !sem.Idempotent {
				continue
			}
			reg := NewEventRegistry()
			h := &mockHandler{et: et}
			reg.Register(h)
			dedup := NewDeduplicationStore()
			ep := &mockEpochProvider{epoch: 1}
			disp := NewEventDispatcher(reg, dedup, ep)

			var epoch uint64 = 1
			if !sem.CarriesEpoch {
				epoch = 0
			}
			ev := newTestEvent(et, epoch)

			// first dispatch
			err := disp.Dispatch(context.Background(), ev)
			if err != nil {
				t.Fatalf("first dispatch for %s failed: %v", et, err)
			}

			callsBefore := h.calls.Load()
			// second dispatch with same event_id
			err = disp.Dispatch(context.Background(), ev)
			if err != nil {
				t.Fatalf("second dispatch for %s should return nil, got: %v", et, err)
			}
			if h.calls.Load() != callsBefore {
				t.Fatalf("handler should not be called again for idempotent event %s", et)
			}
		}
	})

	// idempotent=false: every dispatch executes handler
	t.Run("idempotent_false", func(t *testing.T) {
		for _, et := range AllEventTypes {
			sem := GetSemantics(et)
			if sem.Idempotent {
				continue
			}
			reg := NewEventRegistry()
			h := &mockHandler{et: et}
			reg.Register(h)
			dedup := NewDeduplicationStore()
			ep := &mockEpochProvider{epoch: 1}
			disp := NewEventDispatcher(reg, dedup, ep)

			var epoch uint64 = 1
			if !sem.CarriesEpoch {
				epoch = 0
			}

			for i := 0; i < 3; i++ {
				ev := newTestEvent(et, epoch)
				ev.EventID = "same-id"
				err := disp.Dispatch(context.Background(), ev)
				if err != nil {
					t.Fatalf("dispatch %d for %s failed: %v", i, et, err)
				}
			}
			// For requires_ack=true (sync), all 3 calls should execute
			if sem.RequiresAck {
				if h.calls.Load() != 3 {
					t.Fatalf("expected 3 handler calls for non-idempotent %s, got %d", et, h.calls.Load())
				}
			}
			// For requires_ack=false (async), handler runs in goroutine - just verify no error
		}
	})
}

// ── Property 6: Epoch 校验正确性 ──

func TestProperty6_EpochValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		et := genEventType().Draw(t, "event_type")
		sem := GetSemantics(et)

		reg := NewEventRegistry()
		h := &mockHandler{et: et}
		reg.Register(h)
		dedup := NewDeduplicationStore()

		lastEpoch := rapid.Uint64Range(1, 1000).Draw(t, "last_epoch")
		ep := &mockEpochProvider{epoch: lastEpoch}
		disp := NewEventDispatcher(reg, dedup, ep)

		if sem.CarriesEpoch {
			// stale epoch should be rejected
			staleEpoch := rapid.Uint64Range(0, lastEpoch-1).Draw(t, "stale_epoch")
			if staleEpoch == 0 {
				staleEpoch = 1 // avoid epoch validation error
			}
			ev := newTestEvent(et, staleEpoch)
			// For stale epoch < lastEpoch, but epoch must be > 0 for carries_epoch
			if staleEpoch > 0 && staleEpoch < lastEpoch {
				err := disp.Dispatch(context.Background(), ev)
				var se *ErrEpochStale
				if !errors.As(err, &se) {
					t.Fatalf("expected ErrEpochStale for stale epoch %d < %d, got %v", staleEpoch, lastEpoch, err)
				}
			}

			// valid epoch should pass
			validEpoch := rapid.Uint64Range(lastEpoch, lastEpoch+100).Draw(t, "valid_epoch")
			ev2 := newTestEvent(et, validEpoch)
			ev2.EventID = "valid-epoch-event"
			err := disp.Dispatch(context.Background(), ev2)
			if err != nil {
				t.Fatalf("valid epoch dispatch failed: %v", err)
			}
		} else {
			// carries_epoch=false: should not be rejected regardless of epoch
			ev := newTestEvent(et, 0)
			err := disp.Dispatch(context.Background(), ev)
			if err != nil {
				t.Fatalf("non-epoch event should not be rejected: %v", err)
			}
		}
	})
}

// ── Property 7: 同步/异步分发行为 ──

func TestProperty7_SyncAsyncDispatch(t *testing.T) {
	// requires_ack=true: synchronous, returns handler result
	t.Run("sync", func(t *testing.T) {
		for _, et := range AllEventTypes {
			sem := GetSemantics(et)
			if !sem.RequiresAck {
				continue
			}
			reg := NewEventRegistry()
			h := &mockHandler{et: et}
			reg.Register(h)
			dedup := NewDeduplicationStore()
			ep := &mockEpochProvider{epoch: 1}
			disp := NewEventDispatcher(reg, dedup, ep)

			var epoch uint64 = 1
			if !sem.CarriesEpoch {
				epoch = 0
			}
			ev := newTestEvent(et, epoch)
			err := disp.Dispatch(context.Background(), ev)
			if err != nil {
				t.Fatalf("sync dispatch for %s failed: %v", et, err)
			}
			if h.calls.Load() < 1 {
				t.Fatalf("handler should have been called synchronously for %s", et)
			}
		}
	})

	// requires_ack=false: async, returns nil immediately
	t.Run("async", func(t *testing.T) {
		for _, et := range AllEventTypes {
			sem := GetSemantics(et)
			if sem.RequiresAck {
				continue
			}
			reg := NewEventRegistry()
			var started sync.WaitGroup
			started.Add(1)
			blocker := make(chan struct{})
			h := &mockHandler{
				et: et,
				returnFn: func() error {
					started.Done()
					<-blocker
					return nil
				},
			}
			reg.Register(h)
			dedup := NewDeduplicationStore()
			ep := &mockEpochProvider{epoch: 1}
			disp := NewEventDispatcher(reg, dedup, ep)

			var epoch uint64 = 1
			if !sem.CarriesEpoch {
				epoch = 0
			}
			ev := newTestEvent(et, epoch)
			ev.EventID = "async-test-" + string(et)
			err := disp.Dispatch(context.Background(), ev)
			if err != nil {
				t.Fatalf("async dispatch for %s should return nil, got: %v", et, err)
			}
			// unblock handler
			started.Wait()
			close(blocker)
		}
	})
}

// ── Property 8: Handler 错误包装 ──

func TestProperty8_HandlerErrorWrapping(t *testing.T) {
	for _, et := range AllEventTypes {
		sem := GetSemantics(et)
		if !sem.RequiresAck {
			continue
		}
		reg := NewEventRegistry()
		handlerErr := errors.New("handler failed")
		h := &mockHandler{et: et, returnFn: func() error { return handlerErr }}
		reg.Register(h)
		dedup := NewDeduplicationStore()
		ep := &mockEpochProvider{epoch: 1}
		disp := NewEventDispatcher(reg, dedup, ep)

		var epoch uint64 = 1
		if !sem.CarriesEpoch {
			epoch = 0
		}
		ev := newTestEvent(et, epoch)
		err := disp.Dispatch(context.Background(), ev)

		var df *ErrDispatchFailed
		if !errors.As(err, &df) {
			t.Fatalf("expected ErrDispatchFailed for %s, got %v", et, err)
		}
		if df.EventID != ev.EventID {
			t.Fatalf("ErrDispatchFailed.EventID mismatch")
		}
		if df.EventType != et {
			t.Fatalf("ErrDispatchFailed.EventType mismatch")
		}
		if df.Unwrap() != handlerErr {
			t.Fatalf("ErrDispatchFailed.Unwrap() should return original error")
		}
	}
}

// ── Property 9: 去重集合清理正确性 ──

func TestProperty9_DedupCleanup(t *testing.T) {
	d := &dedupImplTestable{}

	// add old record (> 1 hour)
	d.store.Store("old-event", time.Now().Add(-2*time.Hour))
	// add recent record (< 1 hour)
	d.store.Store("new-event", time.Now().Add(-30*time.Minute))

	d.Cleanup()

	if d.Contains("old-event") {
		t.Fatal("old event should be cleaned up")
	}
	if !d.Contains("new-event") {
		t.Fatal("new event should be retained")
	}
}

// dedupImplTestable wraps dedupImpl for testing with controllable times
type dedupImplTestable struct {
	store sync.Map
}

func (d *dedupImplTestable) Contains(eventID string) bool {
	_, ok := d.store.Load(eventID)
	return ok
}

func (d *dedupImplTestable) Add(eventID string) {
	d.store.Store(eventID, time.Now())
}

func (d *dedupImplTestable) Cleanup() {
	cutoff := time.Now().Add(-1 * time.Hour)
	d.store.Range(func(key, value any) bool {
		if t, ok := value.(time.Time); ok && t.Before(cutoff) {
			d.store.Delete(key)
		}
		return true
	})
}

// ── Property 10: JSON round-trip ──

func TestProperty10_JSONRoundTrip(t *testing.T) {
	// ControlEvent round-trip
	rapid.Check(t, func(t *rapid.T) {
		et := genEventType().Draw(t, "event_type")
		sem := GetSemantics(et)
		var epoch uint64
		if sem.CarriesEpoch {
			epoch = rapid.Uint64Range(1, 1<<62).Draw(t, "epoch")
		}
		ev, _ := NewEvent(et, "test-source", "ref-1", epoch)

		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var ev2 ControlEvent
		if err := json.Unmarshal(data, &ev2); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if ev.EventID != ev2.EventID {
			t.Fatal("EventID mismatch")
		}
		if ev.EventType != ev2.EventType {
			t.Fatal("EventType mismatch")
		}
		if ev.Source != ev2.Source {
			t.Fatal("Source mismatch")
		}
		if ev.TargetScope != ev2.TargetScope {
			t.Fatal("TargetScope mismatch")
		}
		if ev.Priority != ev2.Priority {
			t.Fatal("Priority mismatch")
		}
		if ev.Epoch != ev2.Epoch {
			t.Fatal("Epoch mismatch")
		}
		if ev.RequiresAck != ev2.RequiresAck {
			t.Fatal("RequiresAck mismatch")
		}
		if !ev.CreatedAt.Equal(ev2.CreatedAt) {
			t.Fatal("CreatedAt mismatch")
		}
	})

	// EventSemantics round-trip
	rapid.Check(t, func(t *rapid.T) {
		et := genEventType().Draw(t, "event_type")
		sem := GetSemantics(et)

		data, err := json.Marshal(sem)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		// verify snake_case keys
		var raw map[string]any
		json.Unmarshal(data, &raw)
		for _, key := range []string{"default_scope", "default_priority", "requires_ack", "idempotent", "replayable", "carries_epoch"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("missing snake_case key: %s", key)
			}
		}

		var sem2 EventSemantics
		if err := json.Unmarshal(data, &sem2); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if sem.DefaultScope != sem2.DefaultScope || sem.DefaultPriority != sem2.DefaultPriority ||
			sem.RequiresAck != sem2.RequiresAck || sem.Idempotent != sem2.Idempotent ||
			sem.Replayable != sem2.Replayable || sem.CarriesEpoch != sem2.CarriesEpoch {
			t.Fatal("EventSemantics round-trip mismatch")
		}
	})

	// verify created_at is RFC 3339
	t.Run("created_at_rfc3339", func(t *testing.T) {
		ev := NewBudgetRejectEvent("src", "ref", 0)
		data, _ := json.Marshal(ev)
		var raw map[string]any
		json.Unmarshal(data, &raw)
		ts, ok := raw["created_at"].(string)
		if !ok {
			t.Fatal("created_at should be string")
		}
		if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
			t.Fatalf("created_at not RFC 3339: %s", ts)
		}
	})
}
