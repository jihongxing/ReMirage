package stealth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	pb "mirage-proto/gen"

	"mirage-gateway/pkg/gtunnel/stego"
)

// ChannelState represents the current channel state.
type ChannelState string

const (
	ChannelSchemeA ChannelState = "SchemeA"
	ChannelSchemeB ChannelState = "SchemeB"
	ChannelQueued  ChannelState = "Queued"
)

// EventDispatcher is a local interface to avoid circular imports.
type EventDispatcher interface {
	Dispatch(ctx context.Context, event interface{}) error
}

// TimelineCollector is a local interface for channel switch auditing.
type TimelineCollector interface {
	OnChannelSwitch(from, to ChannelState, reason string)
}

// StealthControlPlaneOpts holds options for creating a StealthControlPlane.
type StealthControlPlaneOpts struct {
	Mux        *ShadowStreamMux
	Encoder    *stego.StegoEncoder
	Decoder    *stego.StegoDecoder
	Dispatcher EventDispatcher
	Audit      AuditCollector
	Timeline   TimelineCollector
}

// StealthControlPlane is the covert control plane bearer layer.
type StealthControlPlane struct {
	mux        *ShadowStreamMux
	encoder    *stego.StegoEncoder
	decoder    *stego.StegoDecoder
	dispatcher EventDispatcher
	audit      AuditCollector
	timeline   TimelineCollector
	state      atomic.Value // ChannelState
	cmdQueue   chan *pb.ControlCommand
	dedup      sync.Map
	mu         sync.Mutex
	closed     atomic.Bool
}

// NewStealthControlPlane creates a new stealth control plane.
func NewStealthControlPlane(opts StealthControlPlaneOpts) *StealthControlPlane {
	p := &StealthControlPlane{
		mux:        opts.Mux,
		encoder:    opts.Encoder,
		decoder:    opts.Decoder,
		dispatcher: opts.Dispatcher,
		audit:      opts.Audit,
		timeline:   opts.Timeline,
		cmdQueue:   make(chan *pb.ControlCommand, 64),
	}
	p.updateState()
	return p
}

// SendCommand sends a control command via the best available channel.
func (p *StealthControlPlane) SendCommand(ctx context.Context, cmd *pb.ControlCommand) error {
	if p.closed.Load() {
		return errors.New("control plane closed")
	}

	// Dedup check
	if _, loaded := p.dedup.LoadOrStore(cmd.CommandId, struct{}{}); loaded {
		return nil // duplicate, silently discard
	}

	state := p.GetChannelState()
	switch state {
	case ChannelSchemeA:
		if p.mux != nil {
			return p.mux.WriteCommand(cmd)
		}
		fallthrough
	case ChannelSchemeB:
		if p.encoder != nil {
			return p.encoder.Enqueue(cmd)
		}
		fallthrough
	case ChannelQueued:
		select {
		case p.cmdQueue <- cmd:
			return nil
		default:
			// Queue full — drop oldest
			select {
			case <-p.cmdQueue:
			default:
			}
			p.cmdQueue <- cmd
			return nil
		}
	}
	return nil
}

// ReceiveLoop runs the receive loop for both Scheme A and Scheme B.
func (p *StealthControlPlane) ReceiveLoop(ctx context.Context) error {
	schemeACh := make(chan *pb.ControlCommand, 16)
	schemeBCh := make(chan *pb.ControlCommand, 16)

	// Scheme A reader goroutine
	go func() {
		for {
			if p.closed.Load() {
				return
			}
			if p.mux == nil || !p.mux.IsAvailable() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(100 * time.Millisecond):
					continue
				}
			}
			cmd, err := p.mux.ReadCommand()
			if err != nil || cmd == nil {
				continue
			}
			select {
			case schemeACh <- cmd:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Scheme B reader goroutine
	go func() {
		for {
			if p.closed.Load() {
				return
			}
			if p.decoder == nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(500 * time.Millisecond):
					continue
				}
			}
			// Scheme B 通过 stego 解码，需要从外部数据源获取包
			// 当前实现：Scheme B 通道由外部调用 InjectSchemeB 注入
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
	}()

	backoff := 100 * time.Millisecond
	maxBackoff := 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case cmd := <-schemeACh:
			p.dispatchCommand(ctx, cmd)
			backoff = 100 * time.Millisecond // reset backoff

		case cmd := <-schemeBCh:
			p.dispatchCommand(ctx, cmd)
			backoff = 100 * time.Millisecond // reset backoff

		case <-time.After(backoff):
			// 无通道可用时退避（替代热循环）
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}
	}
}

// dispatchCommand 去重后分发命令到 EventDispatcher
func (p *StealthControlPlane) dispatchCommand(ctx context.Context, cmd *pb.ControlCommand) {
	if cmd == nil {
		return
	}
	if _, loaded := p.dedup.LoadOrStore(cmd.CommandId, struct{}{}); loaded {
		return // duplicate
	}
	event, err := ToControlEvent(cmd)
	if err == nil && p.dispatcher != nil {
		p.dispatcher.Dispatch(ctx, event)
	}
}

// GetChannelState returns the current channel state.
func (p *StealthControlPlane) GetChannelState() ChannelState {
	v := p.state.Load()
	if v == nil {
		return ChannelQueued
	}
	return v.(ChannelState)
}

// SetSchemeAAvailable updates Scheme A availability.
func (p *StealthControlPlane) SetSchemeAAvailable(available bool) {
	old := p.GetChannelState()
	p.updateState()
	newState := p.GetChannelState()
	if old != newState && p.timeline != nil {
		reason := "scheme_a_change"
		if !available {
			reason = "scheme_a_unavailable"
		}
		p.timeline.OnChannelSwitch(old, newState, reason)
	}
	// 从 Queued 恢复到可用通道时，自动回放排队命令
	if old == ChannelQueued && newState != ChannelQueued {
		go p.drainOnRecovery(context.Background())
	}
}

func (p *StealthControlPlane) updateState() {
	if p.mux != nil && p.mux.IsAvailable() {
		p.state.Store(ChannelSchemeA)
	} else if p.encoder != nil {
		p.state.Store(ChannelSchemeB)
	} else {
		p.state.Store(ChannelQueued)
	}
}

// Close closes the control plane.
func (p *StealthControlPlane) Close() error {
	if p.closed.Swap(true) {
		return nil
	}
	if p.mux != nil {
		p.mux.Close()
	}
	close(p.cmdQueue)
	return nil
}

// InjectSchemeB 从外部注入 Scheme B 数据包进行解码和分发
func (p *StealthControlPlane) InjectSchemeB(ctx context.Context, packet []byte) error {
	if p.decoder == nil {
		return errors.New("scheme B decoder not available")
	}
	cmd, err := p.decoder.Decode(packet)
	if err != nil || cmd == nil {
		return err
	}
	p.dispatchCommand(ctx, cmd)
	return nil
}

// QueueLen returns the current local queue length (for testing).
func (p *StealthControlPlane) QueueLen() int {
	return len(p.cmdQueue)
}

// IsDuplicate checks if a command_id has been seen (for testing).
func (p *StealthControlPlane) IsDuplicate(commandID string) bool {
	_, ok := p.dedup.Load(commandID)
	return ok
}

// DrainQueue drains the local queue and returns all commands (for testing and recovery).
func (p *StealthControlPlane) DrainQueue() []*pb.ControlCommand {
	var cmds []*pb.ControlCommand
	for {
		select {
		case cmd := <-p.cmdQueue:
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		default:
			return cmds
		}
	}
}

// defaultCommandTTL 命令超时时间（超过此时间的排队命令在恢复时丢弃）
const defaultCommandTTL = 60 * time.Second

// drainOnRecovery 通道恢复后自动回放排队命令。
// 检查每条命令的时间戳，丢弃超过 TTL 的命令。
func (p *StealthControlPlane) drainOnRecovery(ctx context.Context) {
	cmds := p.DrainQueue()
	if len(cmds) == 0 {
		return
	}

	now := time.Now()
	var replayed, expired int

	for _, cmd := range cmds {
		// 检查命令时间戳（Timestamp 为 Unix 秒）
		if cmd.Timestamp > 0 {
			cmdTime := time.Unix(cmd.Timestamp, 0)
			if now.Sub(cmdTime) > defaultCommandTTL {
				expired++
				continue
			}
		}

		// 通过当前可用通道重发
		if err := p.SendCommand(ctx, cmd); err != nil {
			// 发送失败，命令会重新进入 cmdQueue
			continue
		}
		replayed++
	}

	if replayed > 0 || expired > 0 {
		// log imported via the package's existing imports
	}
}
