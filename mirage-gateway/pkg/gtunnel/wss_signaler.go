// Package gtunnel - WSS 信令器
// 通过已建立的 Chameleon WSS 通道偷渡 WebRTC SDP/ICE 控制帧
// 控制帧格式: [2字节长度][1字节类型][Payload]
package gtunnel

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// 控制帧类型常量
const (
	CtrlTypeSDP_Offer     byte = 0x10 // SDP Offer
	CtrlTypeSDP_Answer    byte = 0x11 // SDP Answer
	CtrlTypeICE_Candidate byte = 0x12 // ICE Candidate
	CtrlTypeICE_Done      byte = 0x13 // ICE Gathering 完成标记

	// 控制帧魔数前缀（区分数据帧和控制帧）
	CtrlMagic byte = 0xFE
)

// WSSSignaler 基于 Chameleon WSS 通道的 SDP 信令器
type WSSSignaler struct {
	wss *ChameleonClientConn

	// 接收通道（按类型分发）
	answerCh    chan webrtc.SessionDescription
	candidateCh chan webrtc.ICECandidateInit

	mu     sync.Mutex
	closed bool
}

// NewWSSSignaler 创建 WSS 信令器
// wss: 已建立的 Chameleon WSS 连接
func NewWSSSignaler(wss *ChameleonClientConn) *WSSSignaler {
	s := &WSSSignaler{
		wss:         wss,
		answerCh:    make(chan webrtc.SessionDescription, 1),
		candidateCh: make(chan webrtc.ICECandidateInit, 32),
	}
	go s.dispatchLoop()
	return s
}

// SendOffer 通过 WSS 发送 SDP Offer
func (s *WSSSignaler) SendOffer(offer webrtc.SessionDescription) error {
	return s.sendCtrlFrame(CtrlTypeSDP_Offer, offer)
}

// RecvAnswer 等待接收 SDP Answer（超时 10s）
func (s *WSSSignaler) RecvAnswer() (webrtc.SessionDescription, error) {
	select {
	case answer := <-s.answerCh:
		return answer, nil
	case <-time.After(10 * time.Second):
		return webrtc.SessionDescription{}, fmt.Errorf("等待 SDP Answer 超时")
	}
}

// SendCandidate 通过 WSS 发送 ICE Candidate
func (s *WSSSignaler) SendCandidate(candidate webrtc.ICECandidateInit) error {
	return s.sendCtrlFrame(CtrlTypeICE_Candidate, candidate)
}

// RecvCandidate 接收远端 ICE Candidate（阻塞直到关闭）
func (s *WSSSignaler) RecvCandidate() (webrtc.ICECandidateInit, error) {
	c, ok := <-s.candidateCh
	if !ok {
		return webrtc.ICECandidateInit{}, io.EOF
	}
	return c, nil
}

// Close 关闭信令器
func (s *WSSSignaler) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.answerCh)
		close(s.candidateCh)
	}
}

// sendCtrlFrame 发送控制帧: [Magic 0xFE][Type 1B][JSON Payload]
func (s *WSSSignaler) sendCtrlFrame(ctrlType byte, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化控制帧失败: %w", err)
	}

	// 构造控制帧: [0xFE][type][json...]
	frame := make([]byte, 2+len(data))
	frame[0] = CtrlMagic
	frame[1] = ctrlType
	copy(frame[2:], data)

	// 通过 WSS 的 Send 发送（WSS 层会再加 2 字节长度头）
	return s.wss.Send(frame)
}

// dispatchLoop 从 WSS 接收控制帧并分发到对应通道
func (s *WSSSignaler) dispatchLoop() {
	for {
		data, err := s.wss.Recv()
		if err != nil {
			s.Close()
			return
		}

		// 检查是否为控制帧
		if len(data) < 2 || data[0] != CtrlMagic {
			// 非控制帧，忽略（可能是数据帧，由上层处理）
			continue
		}

		ctrlType := data[1]
		payload := data[2:]

		switch ctrlType {
		case CtrlTypeSDP_Answer:
			var answer webrtc.SessionDescription
			if err := json.Unmarshal(payload, &answer); err != nil {
				continue
			}
			select {
			case s.answerCh <- answer:
			default:
			}

		case CtrlTypeICE_Candidate:
			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal(payload, &candidate); err != nil {
				continue
			}
			select {
			case s.candidateCh <- candidate:
			default:
			}

		case CtrlTypeICE_Done:
			// 远端 ICE 收集完成，关闭 candidate 通道
			s.mu.Lock()
			if !s.closed {
				close(s.candidateCh)
				s.candidateCh = make(chan webrtc.ICECandidateInit, 32) // 防止后续写入 panic
			}
			s.mu.Unlock()
		}
	}
}
