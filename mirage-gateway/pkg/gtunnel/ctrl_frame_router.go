// Package gtunnel - 控制帧路由器
// 在网关侧 ChameleonListener 的 readLoop 中拦截 WebRTC 信令控制帧
// 将 SDP Offer / ICE Candidate 路由到 WebRTCAnswerer
package gtunnel

import (
	"log"
	"sync"
)

// CtrlFrameRouter 控制帧路由器（每个客户端连接一个实例）
type CtrlFrameRouter struct {
	mu       sync.RWMutex
	answerer *WebRTCAnswerer
	conn     *ChameleonServerConn
	config   WebRTCTransportConfig

	// WebRTC 数据包回调（DataChannel 收到的数据注入主流程）
	onWebRTCPacket func(clientID string, data []byte)
}

// NewCtrlFrameRouter 创建控制帧路由器
func NewCtrlFrameRouter(conn *ChameleonServerConn, webrtcConfig WebRTCTransportConfig) *CtrlFrameRouter {
	return &CtrlFrameRouter{
		conn:   conn,
		config: webrtcConfig,
	}
}

// SetWebRTCPacketCallback 设置 WebRTC 数据包回调
func (r *CtrlFrameRouter) SetWebRTCPacketCallback(cb func(clientID string, data []byte)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onWebRTCPacket = cb
}

// IsControlFrame 判断数据是否为控制帧
func IsControlFrame(data []byte) bool {
	return len(data) >= 2 && data[0] == CtrlMagic
}

// HandleControlFrame 处理控制帧，返回 true 表示已消费（不应作为数据帧处理）
func (r *CtrlFrameRouter) HandleControlFrame(data []byte) bool {
	if !IsControlFrame(data) {
		return false
	}

	ctrlType := data[1]
	payload := data[2:]

	switch ctrlType {
	case CtrlTypeSDP_Offer:
		r.handleSDPOffer(payload)
	case CtrlTypeICE_Candidate:
		r.handleICECandidate(payload)
	default:
		log.Printf("⚠️ [CtrlRouter] 未知控制帧类型: 0x%02x", ctrlType)
	}

	return true
}

// handleSDPOffer 处理 SDP Offer → 创建 Answerer 并回传 Answer
func (r *CtrlFrameRouter) handleSDPOffer(offerJSON []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 创建发送控制帧的回调（通过 WSS ServerConn 回传）
	sendCtrl := func(ctrlType byte, payload []byte) error {
		frame := make([]byte, 2+len(payload))
		frame[0] = CtrlMagic
		frame[1] = ctrlType
		copy(frame[2:], payload)
		return r.conn.Send(frame)
	}

	// 创建 Answerer
	answerer := NewWebRTCAnswerer(r.config, sendCtrl)
	r.answerer = answerer

	// 处理 Offer
	if err := answerer.HandleOffer(offerJSON); err != nil {
		log.Printf("❌ [CtrlRouter] 处理 SDP Offer 失败: %v", err)
		answerer.Close()
		r.answerer = nil
		return
	}

	log.Printf("✅ [CtrlRouter] SDP Answer 已回传，等待 DataChannel 打开")

	// 后台等待 DataChannel 就绪，然后启动数据转发
	go r.forwardWebRTCData()
}

// handleICECandidate 处理远端 ICE Candidate
func (r *CtrlFrameRouter) handleICECandidate(candidateJSON []byte) {
	r.mu.RLock()
	answerer := r.answerer
	r.mu.RUnlock()

	if answerer == nil {
		log.Println("⚠️ [CtrlRouter] 收到 ICE Candidate 但 Answerer 未就绪")
		return
	}

	if err := answerer.HandleRemoteCandidate(candidateJSON); err != nil {
		log.Printf("⚠️ [CtrlRouter] 添加远端 Candidate 失败: %v", err)
	}
}

// forwardWebRTCData 等待 DataChannel 就绪后转发数据
func (r *CtrlFrameRouter) forwardWebRTCData() {
	r.mu.RLock()
	answerer := r.answerer
	r.mu.RUnlock()

	if answerer == nil {
		return
	}

	if err := answerer.WaitReady(); err != nil {
		log.Printf("⚠️ [CtrlRouter] WebRTC DataChannel 未就绪: %v", err)
		return
	}

	log.Println("📡 [CtrlRouter] WebRTC DataChannel 就绪，开始转发数据")

	// 持续从 DataChannel 读取数据并注入主流程
	for {
		data, err := answerer.Recv()
		if err != nil {
			return
		}

		r.mu.RLock()
		cb := r.onWebRTCPacket
		r.mu.RUnlock()

		if cb != nil {
			cb(r.conn.ClientID(), data)
		}
	}
}

// GetAnswerer 获取 WebRTC Answerer（用于向客户端发送数据）
func (r *CtrlFrameRouter) GetAnswerer() *WebRTCAnswerer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.answerer
}

// Close 关闭路由器
func (r *CtrlFrameRouter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.answerer != nil {
		r.answerer.Close()
		r.answerer = nil
	}
}
