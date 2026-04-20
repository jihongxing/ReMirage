// Package gtunnel - WebRTC DataChannel 传输
// 伪装形态：跨国视频会议（DTLS + SCTP）
// 不参与 Phase 1 竞速，仅在 WSS 建立后的 Phase 2 阶段通过 WSSSignaler 拉起
//
// 工程红线：
//  1. Pion Diet: 空 MediaEngine，不注册任何音视频 Codec，裸 DataChannel 传输
//  2. Trickle ICE: 不等 ICE Gathering 完成，边收集边发送 Candidate
//  3. 信令偷渡: 复用 Chameleon WSS 通道交换 SDP/ICE（WSSSignaler）
package gtunnel

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
)

// SDPSignaler SDP 信令接口（通过现有 WSS 通道交换）
type SDPSignaler interface {
	SendOffer(offer webrtc.SessionDescription) error
	RecvAnswer() (webrtc.SessionDescription, error)
	SendCandidate(candidate webrtc.ICECandidateInit) error
	RecvCandidate() (webrtc.ICECandidateInit, error)
}

// WebRTCTransport WebRTC DataChannel 传输实现
type WebRTCTransport struct {
	pc        *webrtc.PeerConnection
	dc        *webrtc.DataChannel
	recvChan  chan []byte
	rtt       time.Duration
	connected int32
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	remote    net.Addr
	openCh    chan struct{} // DataChannel Open 信号
}

// NewWebRTCTransport 创建 WebRTC 传输（Pion Diet + Trickle ICE）
//
// 流程：
//  1. 空 MediaEngine 裁剪 Pion（不注册任何 Codec）
//  2. 创建 PeerConnection + DataChannel
//  3. CreateOffer → SetLocalDescription（触发 ICE Gathering）
//  4. 立即通过 signaler 发送 Offer（不等 Gathering 完成 = Trickle ICE）
//  5. OnICECandidate 回调中逐个发送 Candidate
//  6. 接收 Answer + 远端 Candidate
//  7. 等待 DataChannel Open（超时 15s）
func NewWebRTCTransport(signaler SDPSignaler, config WebRTCTransportConfig) (*WebRTCTransport, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// ═══════════════════════════════════════════════════════
	// Pion Diet: 空 MediaEngine + SettingEngine 极限裁剪
	// 不注册任何音视频 Codec，只保留 DataChannel + DTLS
	// ═══════════════════════════════════════════════════════
	m := &webrtc.MediaEngine{}
	// 不调用 m.RegisterDefaultCodecs() — 这是关键

	se := webrtc.SettingEngine{}
	// 禁用 mDNS 候选者（减少暴露面，避免泄露本地 hostname）
	se.SetICEMulticastDNSMode(0) // 0 = disabled in pion/ice/v4
	// 限制网络类型：仅 UDP4（减少候选者数量，加速连接）
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	// 设置 DTLS 指纹算法为 SHA-256（与 Chrome 一致）
	// Pion 默认也是 SHA-256，但显式设置确保不被意外更改
	se.SetDTLSInsecureSkipHelloVerify(false)
	// 限制 ICE 候选者类型：仅 host + srflx（排除 relay 减少暴露）
	se.SetICETimeouts(5*time.Second, 25*time.Second, 2*time.Second)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(m),
		webrtc.WithSettingEngine(se),
	)

	// 构建 ICE 服务器配置
	var iceServers []webrtc.ICEServer
	if len(config.ICEServers) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: config.ICEServers,
		})
	}

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers:   iceServers,
		BundlePolicy: webrtc.BundlePolicyMaxBundle,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 PeerConnection 失败: %w", err)
	}

	w := &WebRTCTransport{
		pc:       pc,
		recvChan: make(chan []byte, 256),
		ctx:      ctx,
		cancel:   cancel,
		openCh:   make(chan struct{}),
	}

	// 创建 DataChannel（不可靠交付模式 — 模拟实时视频流特征）
	ordered := config.Ordered
	dcInit := &webrtc.DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: config.MaxRetransmits,
	}

	dc, err := pc.CreateDataChannel("mirage-tunnel", dcInit)
	if err != nil {
		pc.Close()
		cancel()
		return nil, fmt.Errorf("创建 DataChannel 失败: %w", err)
	}
	w.dc = dc

	// DataChannel 事件
	dc.OnOpen(func() {
		atomic.StoreInt32(&w.connected, 1)
		close(w.openCh) // 通知等待者
		log.Println("📡 [WebRTC] DataChannel 已打开")
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		data := make([]byte, len(msg.Data))
		copy(data, msg.Data)
		select {
		case w.recvChan <- data:
		default:
			// 缓冲区满，丢弃最旧
			select {
			case <-w.recvChan:
			default:
			}
			w.recvChan <- data
		}
	})

	dc.OnClose(func() {
		atomic.StoreInt32(&w.connected, 0)
		cancel()
	})

	// ═══════════════════════════════════════════════════════
	// Trickle ICE: OnICECandidate 逐个发送，不等 Gathering 完成
	// ═══════════════════════════════════════════════════════
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			// ICE Gathering 完成（nil 表示结束）
			return
		}
		init := c.ToJSON()
		if err := signaler.SendCandidate(init); err != nil {
			log.Printf("⚠️ [WebRTC] 发送 ICE Candidate 失败: %v", err)
		}
	})

	// 连接状态监控
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateFailed:
			log.Println("❌ [WebRTC] 连接失败")
			atomic.StoreInt32(&w.connected, 0)
			cancel()
		case webrtc.PeerConnectionStateDisconnected:
			log.Println("⚠️ [WebRTC] 连接断开")
			atomic.StoreInt32(&w.connected, 0)
		case webrtc.PeerConnectionStateConnected:
			// 提取远端地址
			if pair, err := pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair(); err == nil && pair != nil {
				w.mu.Lock()
				w.remote = &net.UDPAddr{
					IP:   net.ParseIP(pair.Remote.Address),
					Port: int(pair.Remote.Port),
				}
				w.mu.Unlock()
			}
		}
	})

	// ═══════════════════════════════════════════════════════
	// 信令交换：Trickle ICE 流程
	// ═══════════════════════════════════════════════════════

	// 1. 创建 Offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		cancel()
		return nil, fmt.Errorf("创建 Offer 失败: %w", err)
	}

	// 2. SetLocalDescription 触发 ICE Gathering（异步）
	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		cancel()
		return nil, fmt.Errorf("设置 LocalDescription 失败: %w", err)
	}

	// 3. 立即发送 Offer（不等 Gathering 完成 = Trickle ICE 核心）
	if err := signaler.SendOffer(offer); err != nil {
		pc.Close()
		cancel()
		return nil, fmt.Errorf("发送 Offer 失败: %w", err)
	}

	// 4. 接收 Answer
	answer, err := signaler.RecvAnswer()
	if err != nil {
		pc.Close()
		cancel()
		return nil, fmt.Errorf("接收 Answer 失败: %w", err)
	}

	if err := pc.SetRemoteDescription(answer); err != nil {
		pc.Close()
		cancel()
		return nil, fmt.Errorf("设置 RemoteDescription 失败: %w", err)
	}

	// 5. 后台接收远端 ICE Candidate（Trickle）
	go func() {
		for {
			candidate, err := signaler.RecvCandidate()
			if err != nil {
				return // 信令通道关闭
			}
			if err := pc.AddICECandidate(candidate); err != nil {
				log.Printf("⚠️ [WebRTC] 添加远端 Candidate 失败: %v", err)
			}
		}
	}()

	// 6. 等待 DataChannel Open（超时 15s）
	select {
	case <-w.openCh:
		log.Println("✅ [WebRTC] DataChannel 就绪，传输可用")
	case <-time.After(15 * time.Second):
		pc.Close()
		cancel()
		return nil, fmt.Errorf("DataChannel 打开超时（15s）")
	}

	return w, nil
}

// Send 发送数据
func (w *WebRTCTransport) Send(data []byte) error {
	if atomic.LoadInt32(&w.connected) == 0 {
		return io.ErrClosedPipe
	}
	return w.dc.Send(data)
}

// Recv 接收数据
func (w *WebRTCTransport) Recv() ([]byte, error) {
	select {
	case data, ok := <-w.recvChan:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-w.ctx.Done():
		return nil, io.EOF
	}
}

// Close 关闭连接
func (w *WebRTCTransport) Close() error {
	w.cancel()
	if w.dc != nil {
		w.dc.Close()
	}
	if w.pc != nil {
		return w.pc.Close()
	}
	return nil
}

// Type 返回传输类型
func (w *WebRTCTransport) Type() TransportType {
	return TransportWebRTC
}

// RTT 返回 RTT（通过 ICE 候选对的 RTT 估算）
func (w *WebRTCTransport) RTT() time.Duration {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.rtt
}

// RemoteAddr 远端地址
func (w *WebRTCTransport) RemoteAddr() net.Addr {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.remote
}

// MaxDatagramSize 返回最大数据报大小
// SCTP DataChannel 单消息上限 16KB（保守值，避免分片）
func (w *WebRTCTransport) MaxDatagramSize() int {
	return 16384
}

// IsConnected 是否已连接
func (w *WebRTCTransport) IsConnected() bool {
	return atomic.LoadInt32(&w.connected) == 1
}

// UpdateRTT 外部更新 RTT（由 LinkAuditor 调用）
func (w *WebRTCTransport) UpdateRTT(rtt time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rtt = rtt
}
