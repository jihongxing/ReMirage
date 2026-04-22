// Package mcc - M.C.C. 匿名信令接收器
// 3 跳匿名信令通道，建立绝对安全的指挥链路
package mcc

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	goebpf "github.com/cilium/ebpf"
	"mirage-gateway/pkg/ebpf"
)

// TacticalMode 战术模式
type TacticalMode uint32

const (
	ModeNormal     TacticalMode = 0 // 正常模式
	ModeSleep      TacticalMode = 1 // 休眠模式
	ModeAggressive TacticalMode = 2 // 激进模式
	ModeStealth    TacticalMode = 3 // 隐匿模式
	ModeEvacuate   TacticalMode = 4 // 撤离模式
)

// SignalType 信令类型
type SignalType uint32

const (
	SignalHeartbeat      SignalType = 0 // 心跳
	SignalTacticalUpdate SignalType = 1 // 战术更新
	SignalDomainRotate   SignalType = 2 // 域名轮换
	SignalEmergencyWipe  SignalType = 3 // 紧急自毁
	SignalGlobalPolicy   SignalType = 4 // 全局策略
	SignalIntelSync      SignalType = 5 // 情报同步
)

// TacticalSignal 战术信令
type TacticalSignal struct {
	Type      SignalType   `json:"type"`
	Mode      TacticalMode `json:"mode,omitempty"`
	Timestamp int64        `json:"ts"`
	Nonce     uint64       `json:"nonce"`
	Payload   []byte       `json:"payload,omitempty"`
}

// GlobalPolicy 全局策略（从 M.C.C. 下发）
type GlobalPolicy struct {
	TacticalMode     TacticalMode `json:"tactical_mode"`
	SocialJitter     uint32       `json:"social_jitter"`      // 0-100
	CIDRotationRate  uint32       `json:"cid_rotation_rate"`  // 次/分钟
	FECRedundancy    uint32       `json:"fec_redundancy"`     // 百分比
	StealthFilter    uint32       `json:"stealth_filter"`     // 隐匿模式最低威胁等级
	PaddingDensity   uint32       `json:"padding_density"`    // NPM 填充密度
	DomainRotateHint []string     `json:"domain_rotate_hint"` // 推荐域名
}

// MCCSignalReceiver M.C.C. 信令接收器
type MCCSignalReceiver struct {
	mu sync.RWMutex

	// Tor 隐藏服务
	onionAddr    string
	torListener  net.Listener
	torConnected bool

	// 加密
	gcm   cipher.AEAD
	hwKey []byte // 硬件指纹派生密钥

	// eBPF Map 引用
	globalPolicyMap  *goebpf.Map
	npmConfigMap     *goebpf.Map
	jitterConfigMap  *goebpf.Map
	emergencyCtrlMap *goebpf.Map
	ghostModeMap     *goebpf.Map

	// 回调
	onTacticalUpdate func(TacticalMode)
	onDomainRotate   func([]string)
	onEmergencyWipe  func()

	// 状态
	lastSignal    *TacticalSignal
	lastNonce     uint64
	signalCount   uint64
	currentPolicy *GlobalPolicy

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewMCCSignalReceiver 创建信令接收器
func NewMCCSignalReceiver(hwFingerprint []byte) (*MCCSignalReceiver, error) {
	// 从硬件指纹派生 AES 密钥
	keyHash := sha256.Sum256(hwFingerprint)
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		return nil, fmt.Errorf("创建 AES 密钥失败: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &MCCSignalReceiver{
		gcm:    gcm,
		hwKey:  keyHash[:],
		ctx:    ctx,
		cancel: cancel,
		currentPolicy: &GlobalPolicy{
			TacticalMode:    ModeNormal,
			SocialJitter:    50,
			CIDRotationRate: 5,
			FECRedundancy:   20,
		},
	}, nil
}

// SetEBPFMaps 设置 eBPF Map 引用
func (r *MCCSignalReceiver) SetEBPFMaps(globalPolicy, npmConfig, jitterConfig, emergencyCtrl, ghostMode *goebpf.Map) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.globalPolicyMap = globalPolicy
	r.npmConfigMap = npmConfig
	r.jitterConfigMap = jitterConfig
	r.emergencyCtrlMap = emergencyCtrl
	r.ghostModeMap = ghostMode
}

// SetCallbacks 设置回调
func (r *MCCSignalReceiver) SetCallbacks(onTactical func(TacticalMode), onDomain func([]string), onWipe func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onTacticalUpdate = onTactical
	r.onDomainRotate = onDomain
	r.onEmergencyWipe = onWipe
}

// Start 启动信令接收器
func (r *MCCSignalReceiver) Start(onionAddr string) error {
	r.mu.Lock()
	r.onionAddr = onionAddr
	r.mu.Unlock()

	// 启动 Tor 监听
	r.wg.Add(1)
	go r.torListenLoop()

	// 启动心跳检测
	r.wg.Add(1)
	go r.heartbeatLoop()

	log.Printf("📡 M.C.C. 信令接收器已启动 (onion: %s)", onionAddr)
	return nil
}

// Stop 停止信令接收器
func (r *MCCSignalReceiver) Stop() {
	r.cancel()
	if r.torListener != nil {
		r.torListener.Close()
	}
	r.wg.Wait()
	log.Println("🛑 M.C.C. 信令接收器已停止")
}

// torListenLoop Tor 隐藏服务监听循环
func (r *MCCSignalReceiver) torListenLoop() {
	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		// 尝试连接 Tor SOCKS 代理
		conn, err := r.connectTorHiddenService()
		if err != nil {
			log.Printf("⚠️  Tor 连接失败: %v，5秒后重试", err)
			time.Sleep(5 * time.Second)
			continue
		}

		r.mu.Lock()
		r.torConnected = true
		r.mu.Unlock()

		// 处理信令
		r.handleConnection(conn)

		r.mu.Lock()
		r.torConnected = false
		r.mu.Unlock()
	}
}

// connectTorHiddenService 连接 Tor 隐藏服务
func (r *MCCSignalReceiver) connectTorHiddenService() (net.Conn, error) {
	// 通过 Tor SOCKS5 代理连接
	dialer, err := net.DialTimeout("tcp", "127.0.0.1:9050", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("连接 Tor SOCKS 失败: %w", err)
	}

	// SOCKS5 握手
	if err := r.socks5Handshake(dialer, r.onionAddr); err != nil {
		dialer.Close()
		return nil, err
	}

	return dialer, nil
}

// socks5Handshake SOCKS5 握手
func (r *MCCSignalReceiver) socks5Handshake(conn net.Conn, target string) error {
	// 1. 发送认证方法
	conn.Write([]byte{0x05, 0x01, 0x00}) // SOCKS5, 1 method, no auth

	// 2. 读取响应
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		return errors.New("SOCKS5 认证失败")
	}

	// 3. 发送连接请求（域名类型）
	req := []byte{0x05, 0x01, 0x00, 0x03} // SOCKS5, CONNECT, reserved, domain
	req = append(req, byte(len(target)))
	req = append(req, []byte(target)...)
	req = append(req, 0x00, 0x50) // port 80

	conn.Write(req)

	// 4. 读取响应
	resp = make([]byte, 10)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	if resp[1] != 0x00 {
		return fmt.Errorf("SOCKS5 连接失败: %d", resp[1])
	}

	return nil
}

// handleConnection 处理连接
func (r *MCCSignalReceiver) handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 4096)
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		if n < 12 { // 最小长度：nonce(8) + gcm.NonceSize()
			continue
		}

		// 解密信令
		signal, err := r.decryptSignal(buf[:n])
		if err != nil {
			log.Printf("⚠️  信令解密失败: %v", err)
			continue
		}

		// 处理信令
		r.processSignal(signal)
	}
}

// decryptSignal 解密信令
func (r *MCCSignalReceiver) decryptSignal(ciphertext []byte) (*TacticalSignal, error) {
	if len(ciphertext) < r.gcm.NonceSize() {
		return nil, errors.New("密文太短")
	}

	nonce := ciphertext[:r.gcm.NonceSize()]
	ciphertext = ciphertext[r.gcm.NonceSize():]

	plaintext, err := r.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	var signal TacticalSignal
	if err := json.Unmarshal(plaintext, &signal); err != nil {
		return nil, err
	}

	// 防重放检查
	if signal.Nonce <= r.lastNonce {
		return nil, errors.New("重放攻击检测")
	}

	return &signal, nil
}

// processSignal 处理信令
func (r *MCCSignalReceiver) processSignal(signal *TacticalSignal) {
	r.mu.Lock()
	r.lastSignal = signal
	r.lastNonce = signal.Nonce
	r.signalCount++

	onTactical := r.onTacticalUpdate
	onDomain := r.onDomainRotate
	onWipe := r.onEmergencyWipe
	r.mu.Unlock()

	log.Printf("📨 收到 M.C.C. 信令: type=%d, mode=%d", signal.Type, signal.Mode)

	switch signal.Type {
	case SignalHeartbeat:
		// 心跳，无需处理

	case SignalTacticalUpdate:
		r.handleTacticalUpdate(signal.Mode)
		if onTactical != nil {
			onTactical(signal.Mode)
		}

	case SignalDomainRotate:
		var domains []string
		if err := json.Unmarshal(signal.Payload, &domains); err == nil {
			if onDomain != nil {
				onDomain(domains)
			}
		}

	case SignalEmergencyWipe:
		log.Println("🚨 收到紧急自毁指令！")
		r.triggerEmergencyWipe()
		if onWipe != nil {
			onWipe()
		}

	case SignalGlobalPolicy:
		var policy GlobalPolicy
		if err := json.Unmarshal(signal.Payload, &policy); err == nil {
			r.applyGlobalPolicy(&policy)
		}

	case SignalIntelSync:
		// 情报同步，转发给 Cortex
		log.Println("📊 收到情报同步信令")
	}
}

// handleTacticalUpdate 处理战术更新
func (r *MCCSignalReceiver) handleTacticalUpdate(mode TacticalMode) {
	r.mu.Lock()
	r.currentPolicy.TacticalMode = mode
	r.mu.Unlock()

	// 更新 eBPF Map
	r.syncGlobalPolicyToEBPF()

	// 根据模式调整参数
	switch mode {
	case ModeSleep:
		log.Println("😴 进入休眠模式：降低活动频率")
		r.updateGhostMode(1)

	case ModeAggressive:
		log.Println("⚔️  进入激进模式：增强防御")
		r.updateGhostMode(0)

	case ModeStealth:
		log.Println("👻 进入隐匿模式：最大化伪装")
		r.updateGhostMode(1)

	case ModeEvacuate:
		log.Println("🏃 进入撤离模式：准备转移")
		r.updateGhostMode(1)

	default:
		log.Println("✅ 恢复正常模式")
		r.updateGhostMode(0)
	}
}

// applyGlobalPolicy 应用全局策略
func (r *MCCSignalReceiver) applyGlobalPolicy(policy *GlobalPolicy) {
	r.mu.Lock()
	r.currentPolicy = policy
	r.mu.Unlock()

	log.Printf("📋 应用全局策略: mode=%d, jitter=%d, fec=%d%%",
		policy.TacticalMode, policy.SocialJitter, policy.FECRedundancy)

	// 同步到所有 eBPF Map
	r.syncGlobalPolicyToEBPF()
	r.syncNPMConfig(policy.PaddingDensity)
	r.syncJitterConfig(policy.SocialJitter)
}

// syncGlobalPolicyToEBPF 同步全局策略到 eBPF
func (r *MCCSignalReceiver) syncGlobalPolicyToEBPF() {
	if r.globalPolicyMap == nil {
		return
	}

	r.mu.RLock()
	policy := r.currentPolicy
	r.mu.RUnlock()

	// 构造 eBPF 结构体
	type ebpfGlobalPolicy struct {
		TacticalMode    uint32
		SocialJitter    uint32
		CIDRotationRate uint32
		FECRedundancy   uint32
		StealthFilter   uint32
		Timestamp       uint64
	}

	ebpfPolicy := ebpfGlobalPolicy{
		TacticalMode:    uint32(policy.TacticalMode),
		SocialJitter:    policy.SocialJitter,
		CIDRotationRate: policy.CIDRotationRate,
		FECRedundancy:   policy.FECRedundancy,
		StealthFilter:   policy.StealthFilter,
		Timestamp:       uint64(time.Now().UnixNano()),
	}

	key := uint32(0)
	if err := r.globalPolicyMap.Put(&key, &ebpfPolicy); err != nil {
		log.Printf("⚠️  更新 global_policy_map 失败: %v", err)
	}
}

// syncNPMConfig 同步 NPM 配置
func (r *MCCSignalReceiver) syncNPMConfig(paddingDensity uint32) {
	if r.npmConfigMap == nil {
		return
	}

	config := ebpf.NewDefaultNPMConfig(paddingDensity)

	key := uint32(0)
	if err := r.npmConfigMap.Put(&key, &config); err != nil {
		log.Printf("⚠️  更新 npm_config_map 失败: %v", err)
	}
}

// syncJitterConfig 同步 Jitter 配置
func (r *MCCSignalReceiver) syncJitterConfig(socialJitter uint32) {
	if r.jitterConfigMap == nil {
		return
	}

	type jitterConfig struct {
		Enabled     uint32
		MeanIATUs   uint32
		StddevIATUs uint32
		TemplateID  uint32
	}

	// 根据社交抖动调整参数
	meanIAT := uint32(10000) // 10ms 基准
	stddev := meanIAT * socialJitter / 100

	config := jitterConfig{
		Enabled:     1,
		MeanIATUs:   meanIAT,
		StddevIATUs: stddev,
		TemplateID:  0,
	}

	key := uint32(0)
	if err := r.jitterConfigMap.Put(&key, &config); err != nil {
		log.Printf("⚠️  更新 jitter_config_map 失败: %v", err)
	}
}

// updateGhostMode 更新 Ghost Mode
func (r *MCCSignalReceiver) updateGhostMode(enabled uint32) {
	if r.ghostModeMap == nil {
		return
	}

	key := uint32(0)
	if err := r.ghostModeMap.Put(&key, &enabled); err != nil {
		log.Printf("⚠️  更新 ghost_mode_map 失败: %v", err)
	}
}

// triggerEmergencyWipe 触发紧急自毁
func (r *MCCSignalReceiver) triggerEmergencyWipe() {
	if r.emergencyCtrlMap == nil {
		return
	}

	key := uint32(0)
	value := uint32(0xDEADBEEF) // 自毁魔数
	if err := r.emergencyCtrlMap.Put(&key, &value); err != nil {
		log.Printf("⚠️  触发紧急自毁失败: %v", err)
	}
}

// heartbeatLoop 心跳检测循环
func (r *MCCSignalReceiver) heartbeatLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	missedHeartbeats := 0

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.mu.RLock()
			lastSignal := r.lastSignal
			connected := r.torConnected
			r.mu.RUnlock()

			if !connected {
				missedHeartbeats++
				if missedHeartbeats > 5 {
					log.Println("⚠️  M.C.C. 连接丢失超过 5 次，进入自主模式")
				}
				continue
			}

			// 检查最后信令时间
			if lastSignal != nil {
				age := time.Since(time.Unix(lastSignal.Timestamp, 0))
				if age > 5*time.Minute {
					log.Println("⚠️  M.C.C. 心跳超时，可能被切断")
					missedHeartbeats++
				} else {
					missedHeartbeats = 0
				}
			}
		}
	}
}

// GetCurrentPolicy 获取当前策略
func (r *MCCSignalReceiver) GetCurrentPolicy() *GlobalPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentPolicy
}

// GetStatus 获取状态
func (r *MCCSignalReceiver) GetStatus() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{
		"connected":    r.torConnected,
		"onion_addr":   r.onionAddr,
		"signal_count": r.signalCount,
		"last_nonce":   r.lastNonce,
		"mode":         r.currentPolicy.TacticalMode,
	}
}

// SendBurnReport 发送域名战死报告（通过 Tor 回传）
func (r *MCCSignalReceiver) SendBurnReport(domain, reason string) error {
	report := map[string]interface{}{
		"type":      "domain_burned",
		"domain":    domain,
		"reason":    reason,
		"timestamp": time.Now().Unix(),
	}

	data, _ := json.Marshal(report)
	encrypted, err := r.encryptData(data)
	if err != nil {
		return err
	}

	// 通过 Tor 发送（简化实现）
	conn, err := r.connectTorHiddenService()
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write(encrypted)
	return err
}

// encryptData 加密数据
func (r *MCCSignalReceiver) encryptData(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, r.gcm.NonceSize())
	if _, err := io.ReadFull(nil, nonce); err != nil {
		// 使用时间戳作为 nonce
		binary.BigEndian.PutUint64(nonce, uint64(time.Now().UnixNano()))
	}
	return r.gcm.Seal(nonce, nonce, plaintext, nil), nil
}
