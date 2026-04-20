// Package gtunnel - WebRTC 网关侧应答器
// 接收客户端通过 WSS 偷渡的 SDP Offer，生成 Answer 并建立 DataChannel
// 同样使用 Pion Diet 裁剪，只保留 DataChannel + DTLS
package gtunnel

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
)

// WebRTCAnswerer 网关侧 WebRTC 应答器
type WebRTCAnswerer struct {
	pc       *webrtc.PeerConnection
	dc       *webrtc.DataChannel
	recvChan chan []byte
	rtt      time.Duration
	mu       sync.RWMutex
	remote   net.Addr

	connected int32
	openCh    chan struct{}

	// 通过 WSS 回传控制帧的回调
	sendCtrlFrame func(ctrlType byte, payload []byte) error

	config WebRTCTransportConfig
}

// NewWebRTCAnswerer 创建网关侧应答器
// sendCtrl: 回传控制帧到客户端的函数（通过 WSS ServerConn）
func NewWebRTCAnswerer(config WebRTCTransportConfig, sendCtrl func(ctrlType byte, payload []byte) error) *WebRTCAnswerer {
	return &WebRTCAnswerer{
		recvChan:      make(chan []byte, 256),
		openCh:        make(chan struct{}),
		sendCtrlFrame: sendCtrl,
		config:        config,
	}
}

// HandleOffer 处理客户端 SDP Offer，生成 Answer 并返回
func (a *WebRTCAnswerer) HandleOffer(offerJSON []byte) error {
	var offer webrtc.SessionDescription
	if err := json.Unmarshal(offerJSON, &offer); err != nil {
		return fmt.Errorf("解析 SDP Offer 失败: %w", err)
	}

	// Pion Diet: 空 MediaEngine
	m := &webrtc.MediaEngine{}
	se := webrtc.SettingEngine{}
	se.SetICEMulticastDNSMode(0)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(m),
		webrtc.WithSettingEngine(se),
	)

	var iceServers []webrtc.ICEServer
	if len(a.config.ICEServers) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: a.config.ICEServers,
		})
	}

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers:   iceServers,
		BundlePolicy: webrtc.BundlePolicyMaxBundle,
	})
	if err != nil {
		return fmt.Errorf("创建 PeerConnection 失败: %w", err)
	}
	a.pc = pc

	// Trickle ICE: 逐个发送本地 Candidate
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		data, err := json.Marshal(c.ToJSON())
		if err != nil {
			return
		}
		a.sendCtrlFrame(CtrlTypeICE_Candidate, data)
	})

	// 监听远端创建的 DataChannel
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		a.dc = dc

		dc.OnOpen(func() {
			atomic.StoreInt32(&a.connected, 1)
			close(a.openCh)
			log.Printf("📡 [WebRTC-GW] DataChannel '%s' 已打开", dc.Label())
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			data := make([]byte, len(msg.Data))
			copy(data, msg.Data)
			select {
			case a.recvChan <- data:
			default:
				select {
				case <-a.recvChan:
				default:
				}
				a.recvChan <- data
			}
		})

		dc.OnClose(func() {
			atomic.StoreInt32(&a.connected, 0)
		})
	})

	// 连接状态
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			if pair, err := pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair(); err == nil && pair != nil {
				a.mu.Lock()
				a.remote = &net.UDPAddr{
					IP:   net.ParseIP(pair.Remote.Address),
					Port: int(pair.Remote.Port),
				}
				a.mu.Unlock()
			}
		}
	})

	// 设置远端 Offer
	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		return fmt.Errorf("设置 RemoteDescription 失败: %w", err)
	}

	// 创建 Answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return fmt.Errorf("创建 Answer 失败: %w", err)
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return fmt.Errorf("设置 LocalDescription 失败: %w", err)
	}

	// 通过 WSS 回传 Answer
	answerData, _ := json.Marshal(answer)
	if err := a.sendCtrlFrame(CtrlTypeSDP_Answer, answerData); err != nil {
		pc.Close()
		return fmt.Errorf("发送 Answer 失败: %w", err)
	}

	return nil
}

// HandleRemoteCandidate 处理客户端发来的 ICE Candidate
func (a *WebRTCAnswerer) HandleRemoteCandidate(candidateJSON []byte) error {
	if a.pc == nil {
		return fmt.Errorf("PeerConnection 未初始化")
	}
	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal(candidateJSON, &candidate); err != nil {
		return fmt.Errorf("解析 ICE Candidate 失败: %w", err)
	}
	return a.pc.AddICECandidate(candidate)
}

// WaitReady 等待 DataChannel 就绪（超时 15s）
func (a *WebRTCAnswerer) WaitReady() error {
	select {
	case <-a.openCh:
		return nil
	case <-time.After(15 * time.Second):
		return fmt.Errorf("DataChannel 打开超时")
	}
}

// Send 发送数据
func (a *WebRTCAnswerer) Send(data []byte) error {
	if atomic.LoadInt32(&a.connected) == 0 {
		return fmt.Errorf("DataChannel 未连接")
	}
	return a.dc.Send(data)
}

// Recv 接收数据
func (a *WebRTCAnswerer) Recv() ([]byte, error) {
	data, ok := <-a.recvChan
	if !ok {
		return nil, fmt.Errorf("接收通道已关闭")
	}
	return data, nil
}

// Close 关闭
func (a *WebRTCAnswerer) Close() error {
	if a.dc != nil {
		a.dc.Close()
	}
	if a.pc != nil {
		return a.pc.Close()
	}
	return nil
}

// IsConnected 是否已连接
func (a *WebRTCAnswerer) IsConnected() bool {
	return atomic.LoadInt32(&a.connected) == 1
}
