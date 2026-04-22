package events

import (
	"context"
	"sync"
)

// EpochProvider epoch 查询接口（依赖 Spec 4-1 ControlStateManager）
type EpochProvider interface {
	GetLastSuccessfulEpoch(ctx context.Context) (uint64, error)
}

// EventDispatcher 事件分发器
type EventDispatcher interface {
	// Dispatch 分发控制事件到已注册的处理器
	Dispatch(ctx context.Context, event *ControlEvent) error
}

// dispatcherImpl EventDispatcher 实现
type dispatcherImpl struct {
	registry EventRegistry
	dedup    DeduplicationStore
	epoch    EpochProvider
	mu       sync.Mutex // 保护并发 Dispatch
}

// NewEventDispatcher 创建 EventDispatcher 实例
func NewEventDispatcher(registry EventRegistry, dedup DeduplicationStore, epochProvider EpochProvider) EventDispatcher {
	return &dispatcherImpl{
		registry: registry,
		dedup:    dedup,
		epoch:    epochProvider,
	}
}

func (d *dispatcherImpl) Dispatch(ctx context.Context, event *ControlEvent) error {
	// 1. 校验
	if err := event.Validate(); err != nil {
		return err
	}

	// 2. 查询语义属性
	sem := GetSemantics(event.EventType)
	if sem == nil {
		return &ErrInvalidEventType{Value: string(event.EventType)}
	}

	// 3. 幂等去重
	if sem.Idempotent {
		if d.dedup.Contains(event.EventID) {
			return nil
		}
	}

	// 4. epoch 校验
	if sem.CarriesEpoch && d.epoch != nil {
		lastEpoch, err := d.epoch.GetLastSuccessfulEpoch(ctx)
		if err != nil {
			return err
		}
		if event.Epoch < lastEpoch {
			return &ErrEpochStale{
				EventEpoch:   event.Epoch,
				CurrentEpoch: lastEpoch,
			}
		}
	}

	// 5. 路由到处理器
	handler, err := d.registry.GetHandler(event.EventType)
	if err != nil {
		return err
	}

	// 6. 同步/异步分发
	if sem.RequiresAck {
		// 同步等待
		if herr := handler.Handle(ctx, event); herr != nil {
			return &ErrDispatchFailed{
				EventID:   event.EventID,
				EventType: event.EventType,
				Cause:     herr,
			}
		}
		if sem.Idempotent {
			d.dedup.Add(event.EventID)
		}
		return nil
	}

	// 异步执行
	go handler.Handle(ctx, event)
	if sem.Idempotent {
		d.dedup.Add(event.EventID)
	}
	return nil
}
