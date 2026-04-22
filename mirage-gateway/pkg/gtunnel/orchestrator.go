// Package gtunnel - Orchestrator 多路径自适应调度器
// 替代 TransportManager，实现分阶段 HappyEyeballs 竞速、优先级调度、
// 链路审计、Epoch Barrier 双发选收、动态 MTU 通知
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
)

// PriorityLevel 传输协议权重优先级
type PriorityLevel uint8

const (
	PriorityQUIC   PriorityLevel = 0 // 最高
	PriorityWebRTC PriorityLevel = 1
	PriorityWSS    PriorityLevel = 2
	PriorityICMP   PriorityLevel = 3 // 最低（与 DNS 同级）
	PriorityDNS    PriorityLevel = 3
)

// OrchestratorState 调度器状态
type OrchestratorState uint8

const (
	StateOrcProbing   OrchestratorState = 0 // 探测中
	StateOrcActive    OrchestratorState = 1 // 活跃
	StateOrcDegrading OrchestratorState = 2 // 降格中
	StateOrcPromoting OrchestratorState = 3 // 升格中
)

// ManagedPath 受管路径
type ManagedPath struct {
	Conn         TransportConn
	Priority     PriorityLevel
	Type         TransportType
	Enabled      bool
	Available    bool
	ProbeSuccess int // 连续探测成功次数
	BaselineRTT  time.Duration
	Phase        int // 竞速阶段：1=Phase1(无依赖), 2=Phase2(有依赖)
}

// WebRTCTransportConfig WebRTC 传输配置
type WebRTCTransportConfig struct {
	ICEServers     []string // STUN/TURN 服务器 URL 列表
	Ordered        bool     // DataChannel 是否有序，默认 false
	MaxRetransmits *uint16  // 最大重传次数，nil = 不可靠模式
}

// ICMPTransportConfig ICMP 传输配置
type ICMPTransportConfig struct {
	TargetIP   net.IP // 目标 IP
	GatewayIP  net.IP // 网关 IP
	Identifier uint16 // ICMP 会话标识
	MaxPayload int    // 单包最大 Payload，默认 1024
}

// DNSTransportConfig DNS 传输配置
type DNSTransportConfig struct {
	Domain      string // 权威域名
	Resolver    string // DNS 服务器地址
	QueryType   string // "TXT" 或 "CNAME"
	MaxLabelLen int    // 子域名最大长度，默认 63
}

// OrchestratorConfig 调度器配置
type OrchestratorConfig struct {
	// 协议启用开关
	EnableQUIC   bool
	EnableWSS    bool
	EnableWebRTC bool
	EnableICMP   bool
	EnableDNS    bool

	// 探测参数
	ProbeCycle       time.Duration // 默认 30s
	ProbeCycleLevel3 time.Duration // Level 3 时缩短为 15s
	PromoteThreshold int           // 连续成功次数，默认 3

	// 降格阈值
	DemoteLossRate    float64 // 默认 0.30
	DemoteRTTMultiple float64 // 默认 2.0

	// 双发选收
	DualSendDuration time.Duration // 默认 100ms

	// 各协议独立配置
	QUICConfig   TransportConfig
	WSSConfig    ChameleonDialConfig
	WebRTCConfig WebRTCTransportConfig
	ICMPConfig   ICMPTransportConfig
	DNSConfig    DNSTransportConfig
}

// DefaultOrchestratorConfig 默认调度器配置
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		EnableQUIC:        true,
		EnableWSS:         true,
		EnableWebRTC:      true,
		EnableICMP:        false,
		EnableDNS:         false,
		ProbeCycle:        30 * time.Second,
		ProbeCycleLevel3:  15 * time.Second,
		PromoteThreshold:  3,
		DemoteLossRate:    0.30,
		DemoteRTTMultiple: 2.0,
		DualSendDuration:  100 * time.Millisecond,
		ICMPConfig: ICMPTransportConfig{
			MaxPayload: 1024,
		},
		DNSConfig: DNSTransportConfig{
			QueryType:   "TXT",
			MaxLabelLen: 63,
		},
	}
}

// Orchestrator 多路径自适应调度器
type Orchestrator struct {
	mu         sync.RWMutex
	activePath *ManagedPath
	paths      map[TransportType]*ManagedPath
	auditor    *LinkAuditor
	fec        *FECProcessor
	mpBuffer   *MultiPathBuffer
	config     OrchestratorConfig
	state      OrchestratorState
	epoch      uint32
	wssConn    TransportConn // WSS 连接引用，用于 WebRTC Phase 2 信令
	dialFuncs  map[TransportType]func(ctx context.Context) (TransportConn, error)

	onPacketRecv  func(data []byte)
	onStateChange func(old, new OrchestratorState)
	stopCh        chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewOrchestrator 创建调度器
func NewOrchestrator(config OrchestratorConfig) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	thresholds := AuditThresholds{
		MaxLossRate:    config.DemoteLossRate,
		MaxRTTMultiple: config.DemoteRTTMultiple,
		WindowSize:     config.ProbeCycle,
	}
	return &Orchestrator{
		paths:   make(map[TransportType]*ManagedPath),
		auditor: NewLinkAuditor(thresholds),
		fec:     NewFECProcessor(),
		config:  config,
		state:   StateOrcProbing,
		stopCh:  make(chan struct{}),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// SetPacketCallback 设置收包回调
func (o *Orchestrator) SetPacketCallback(cb func(data []byte)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onPacketRecv = cb
}

// SetStateCallback 设置状态变更回调
func (o *Orchestrator) SetStateCallback(cb func(old, new OrchestratorState)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onStateChange = cb
}

// GetState 获取当前状态
func (o *Orchestrator) GetState() OrchestratorState {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.state
}

// GetActiveType 获取当前活跃路径类型
func (o *Orchestrator) GetActiveType() TransportType {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.activePath != nil {
		return o.activePath.Type
	}
	return TransportQUIC // 默认
}

// GetEpoch 获取当前 epoch
func (o *Orchestrator) GetEpoch() uint32 {
	return atomic.LoadUint32(&o.epoch)
}

// nextEpoch 递增并返回新 Epoch
func (o *Orchestrator) nextEpoch() uint32 {
	return atomic.AddUint32(&o.epoch, 1)
}

// setState 内部状态切换
func (o *Orchestrator) setState(newState OrchestratorState) {
	old := o.state
	o.state = newState
	if o.onStateChange != nil && old != newState {
		go o.onStateChange(old, newState)
	}
}

// notifyFECMTU 路径切换时通知 FEC 调整分片大小
func (o *Orchestrator) notifyFECMTU(conn TransportConn) {
	if conn == nil {
		return
	}
	maxSize := conn.MaxDatagramSize()
	// 调整 FEC 分片大小：减去 ShardHeader 开销
	headerOverhead := 24 // ShardHeader 大小（含 Epoch）
	newShardSize := maxSize - headerOverhead
	if newShardSize < 32 {
		newShardSize = 32
	}
	if newShardSize > ShardSize {
		newShardSize = ShardSize
	}
	o.fec.shardSize = newShardSize
}

// Send 通过当前活跃通道发送数据
func (o *Orchestrator) Send(data []byte) error {
	o.mu.RLock()
	path := o.activePath
	o.mu.RUnlock()

	if path == nil || path.Conn == nil {
		return io.ErrClosedPipe
	}
	return path.Conn.Send(data)
}

// Recv 从当前活跃通道接收数据
func (o *Orchestrator) Recv() ([]byte, error) {
	o.mu.RLock()
	path := o.activePath
	o.mu.RUnlock()

	if path == nil || path.Conn == nil {
		return nil, io.ErrClosedPipe
	}
	return path.Conn.Recv()
}

// Close 关闭所有连接
func (o *Orchestrator) Close() error {
	o.cancel()
	close(o.stopCh)

	o.mu.Lock()
	defer o.mu.Unlock()

	for _, p := range o.paths {
		if p.Conn != nil {
			p.Conn.Close()
		}
	}
	o.setState(StateOrcProbing)
	return nil
}

// Start 启动调度器：分阶段 HappyEyeballs 探测
func (o *Orchestrator) Start(ctx context.Context) error {
	if err := o.happyEyeballsPhase1(ctx); err != nil {
		return fmt.Errorf("Phase 1 竞速失败: %w", err)
	}

	// Phase 2: 后台拉起 WebRTC（如果 WSS 可用）
	go o.happyEyeballsPhase2(ctx)

	// 启动心跳探测循环
	go o.probeLoop(ctx)

	// 启动收包循环：从活跃路径持续读取数据并触发 onPacketRecv 回调
	go o.receiveLoop(ctx)

	return nil
}

// receiveLoop 从当前活跃路径持续读取数据，触发 onPacketRecv 回调。
// 路径切换时自动跟随 activePath。
func (o *Orchestrator) receiveLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		default:
		}

		data, err := o.Recv()
		if err != nil {
			// activePath 为空或连接断开，短暂等待后重试
			time.Sleep(100 * time.Millisecond)
			continue
		}

		o.mu.RLock()
		cb := o.onPacketRecv
		o.mu.RUnlock()

		if cb != nil {
			cb(data)
		}
	}
}

// enabledPhase1Types 返回 Phase 1 中启用的协议类型（排除 WebRTC）
func (o *Orchestrator) enabledPhase1Types() []TransportType {
	var types []TransportType
	if o.config.EnableQUIC {
		types = append(types, TransportQUIC)
	}
	if o.config.EnableWSS {
		types = append(types, TransportWebSocket)
	}
	if o.config.EnableICMP {
		types = append(types, TransportICMP)
	}
	if o.config.EnableDNS {
		types = append(types, TransportDNS)
	}
	return types
}

// enabledTypes 返回所有启用的协议类型
func (o *Orchestrator) enabledTypes() []TransportType {
	types := o.enabledPhase1Types()
	if o.config.EnableWebRTC {
		types = append(types, TransportWebRTC)
	}
	return types
}

// DialFunc 拨号函数类型，用于 HappyEyeballs 竞速
type DialFunc func(ctx context.Context, t TransportType) (TransportConn, error)

// dialResult HappyEyeballs 竞速结果
type dialResult struct {
	conn      TransportConn
	transport TransportType
	err       error
	latency   time.Duration
}

// happyEyeballsPhase1 Phase 1: 并发竞速探测无信令依赖的协议
func (o *Orchestrator) happyEyeballsPhase1(ctx context.Context) error {
	phase1Types := o.enabledPhase1Types()
	if len(phase1Types) == 0 {
		return fmt.Errorf("无可用的 Phase 1 传输协议")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resultCh := make(chan dialResult, len(phase1Types))

	for _, tt := range phase1Types {
		go func(t TransportType) {
			start := time.Now()
			conn, err := o.dialTransport(ctx, t)
			resultCh <- dialResult{
				conn:      conn,
				transport: t,
				err:       err,
				latency:   time.Since(start),
			}
		}(tt)
	}

	// 等待第一个成功的连接
	var firstResult *dialResult
	remaining := len(phase1Types)
	for remaining > 0 {
		select {
		case r := <-resultCh:
			remaining--
			if r.err == nil && firstResult == nil {
				firstResult = &r
				// 记录所有成功的连接
				o.mu.Lock()
				priority := typeToPriority(r.transport)
				o.paths[r.transport] = &ManagedPath{
					Conn:        r.conn,
					Priority:    priority,
					Type:        r.transport,
					Enabled:     true,
					Available:   true,
					BaselineRTT: r.conn.RTT(),
					Phase:       1,
				}
				o.activePath = o.paths[r.transport]
				o.setState(StateOrcActive)
				o.notifyFECMTU(r.conn)

				// 记录 WSS 连接引用
				if r.transport == TransportWebSocket {
					o.wssConn = r.conn
				}
				o.mu.Unlock()

				log.Printf("🚀 [Orchestrator] Phase 1 竞速胜出: %d (延迟 %v)", r.transport, r.latency)
			} else if r.err == nil {
				// 其他成功的连接也保存为备用
				o.mu.Lock()
				priority := typeToPriority(r.transport)
				o.paths[r.transport] = &ManagedPath{
					Conn:        r.conn,
					Priority:    priority,
					Type:        r.transport,
					Enabled:     true,
					Available:   true,
					BaselineRTT: r.conn.RTT(),
					Phase:       1,
				}
				if r.transport == TransportWebSocket {
					o.wssConn = r.conn
				}
				o.mu.Unlock()
			}
		case <-ctx.Done():
			if firstResult != nil {
				return nil
			}
			return fmt.Errorf("Phase 1 超时，无可用连接")
		}
	}

	if firstResult == nil {
		return fmt.Errorf("Phase 1 所有协议连接失败")
	}
	return nil
}

// happyEyeballsPhase2 Phase 2: WSS 建立后后台拉起 WebRTC
func (o *Orchestrator) happyEyeballsPhase2(ctx context.Context) {
	if !o.config.EnableWebRTC {
		return
	}

	o.mu.RLock()
	wss := o.wssConn
	o.mu.RUnlock()

	if wss == nil {
		log.Println("⚠️  [Orchestrator] Phase 2: WSS 不可用，跳过 WebRTC")
		return
	}

	conn, err := o.dialTransport(ctx, TransportWebRTC)
	if err != nil {
		log.Printf("⚠️  [Orchestrator] Phase 2: WebRTC 打洞失败: %v", err)
		return
	}

	o.mu.Lock()
	o.paths[TransportWebRTC] = &ManagedPath{
		Conn:        conn,
		Priority:    PriorityWebRTC,
		Type:        TransportWebRTC,
		Enabled:     true,
		Available:   true,
		BaselineRTT: conn.RTT(),
		Phase:       2,
	}

	// 如果 WebRTC 优先级高于当前活跃路径，触发升格
	if o.activePath != nil && PriorityWebRTC < o.activePath.Priority {
		o.mu.Unlock()
		o.promote(TransportWebRTC)
		return
	}
	o.mu.Unlock()

	log.Println("✅ [Orchestrator] Phase 2: WebRTC 就绪（备用）")
}

// dialTransport 拨号指定传输协议
func (o *Orchestrator) dialTransport(ctx context.Context, t TransportType) (TransportConn, error) {
	switch t {
	case TransportWebRTC:
		return o.dialWebRTC(ctx)
	case TransportDNS:
		return o.dialDNS(ctx)
	default:
		if fn, ok := o.dialFuncs[t]; ok {
			return fn(ctx)
		}
		return nil, fmt.Errorf("transport %d 未注册拨号函数", t)
	}
}

// RegisterDialFunc 注册拨号函数
func (o *Orchestrator) RegisterDialFunc(t TransportType, fn func(ctx context.Context) (TransportConn, error)) {
	if o.dialFuncs == nil {
		o.dialFuncs = make(map[TransportType]func(ctx context.Context) (TransportConn, error))
	}
	o.dialFuncs[t] = fn
}

// AdoptInboundConn 接受一个外部已建立的入站连接，注册为受管路径。
// 用于 Gateway 服务端模式：Listener 接受客户端连接后注入 Orchestrator。
// 同类型替换时：关闭旧连接，新连接无条件成为活跃路径。
func (o *Orchestrator) AdoptInboundConn(conn TransportConn, t TransportType) {
	o.mu.Lock()
	defer o.mu.Unlock()

	wasActive := false
	// 关闭同类型旧连接
	if old, exists := o.paths[t]; exists && old.Conn != nil {
		if o.activePath == old {
			wasActive = true
		}
		old.Conn.Close()
	}

	priority := typeToPriority(t)
	path := &ManagedPath{
		Conn:      conn,
		Priority:  priority,
		Type:      t,
		Enabled:   true,
		Available: true,
		Phase:     1,
	}
	o.paths[t] = path

	// 无条件切换为活跃路径的条件：
	// 1. 没有活跃路径
	// 2. 新路径优先级更高
	// 3. 同类型替换（旧的就是活跃路径）
	if o.activePath == nil || priority < o.activePath.Priority || wasActive {
		o.activePath = path
		o.setState(StateOrcActive)
		o.notifyFECMTU(conn)
		log.Printf("🔗 [Orchestrator] 入站连接已接管为活跃路径: type=%d", t)
	} else {
		log.Printf("🔗 [Orchestrator] 入站连接已注册为备用路径: type=%d", t)
	}

	if t == TransportWebSocket {
		o.wssConn = conn
	}
}

// StartPassive 启动被动模式调度器（不执行 HappyEyeballs 竞速）。
// 用于 Gateway 服务端：不主动拨号，只管理通过 AdoptInboundConn 注入的连接。
// 启动 probeLoop 和 receiveLoop。
func (o *Orchestrator) StartPassive(ctx context.Context) {
	go o.probeLoop(ctx)
	go o.receiveLoop(ctx)
	log.Println("🔗 [Orchestrator] 被动模式已启动（probeLoop + receiveLoop）")
}

// FeedInboundPacket 将入站数据包喂入指定类型路径的适配器。
// 按 clientID 精确路由：只喂给匹配的适配器，避免切换窗口内喂错连接。
func (o *Orchestrator) FeedInboundPacket(t TransportType, clientID string, data []byte) {
	o.mu.RLock()
	path, ok := o.paths[t]
	o.mu.RUnlock()

	if !ok || path.Conn == nil {
		return
	}

	adapter, isAdapter := path.Conn.(*ChameleonServerConnAdapter)
	if !isAdapter {
		return
	}

	// 精确匹配：只喂给 clientID 一致的适配器
	if adapter.ClientID() != clientID {
		return
	}

	adapter.FeedPacket(data)
}

// dialWebRTC 通过 WSS 信令通道拉起 WebRTC DataChannel
func (o *Orchestrator) dialWebRTC(_ context.Context) (TransportConn, error) {
	o.mu.RLock()
	wss := o.wssConn
	o.mu.RUnlock()

	if wss == nil {
		return nil, fmt.Errorf("WebRTC 依赖 WSS 信令通道，但 WSS 不可用")
	}

	// 将 TransportConn 断言为 ChameleonClientConn 以创建信令器
	chameleon, ok := wss.(*ChameleonClientConn)
	if !ok {
		return nil, fmt.Errorf("WSS 连接类型不匹配，无法创建信令器")
	}

	signaler := NewWSSSignaler(chameleon)
	conn, err := NewWebRTCTransport(signaler, o.config.WebRTCConfig)
	if err != nil {
		signaler.Close()
		return nil, fmt.Errorf("WebRTC 握手失败: %w", err)
	}

	return conn, nil
}

// dialDNS 拉起 DNS Tunnel 传输
func (o *Orchestrator) dialDNS(_ context.Context) (TransportConn, error) {
	conn, err := NewDNSTransport(o.config.DNSConfig)
	if err != nil {
		return nil, fmt.Errorf("DNS Tunnel 创建失败: %w", err)
	}
	return conn, nil
}

// typeToPriority 传输类型到优先级映射
func typeToPriority(t TransportType) PriorityLevel {
	switch t {
	case TransportQUIC:
		return PriorityQUIC
	case TransportWebRTC:
		return PriorityWebRTC
	case TransportWebSocket:
		return PriorityWSS
	case TransportICMP:
		return PriorityICMP
	case TransportDNS:
		return PriorityDNS
	default:
		return PriorityDNS
	}
}

// demote 降格到下一可用优先级路径
func (o *Orchestrator) demote() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.activePath == nil {
		return fmt.Errorf("无活跃路径")
	}

	o.setState(StateOrcDegrading)
	currentPriority := o.activePath.Priority
	newEpoch := o.nextEpoch()

	// 查找下一可用路径（优先级更低）
	var bestPath *ManagedPath
	for _, p := range o.paths {
		if p.Available && p.Enabled && p.Priority > currentPriority {
			if bestPath == nil || p.Priority < bestPath.Priority {
				bestPath = p
			}
		}
	}

	if bestPath == nil {
		o.setState(StateOrcActive)
		return fmt.Errorf("无可用降格路径")
	}

	log.Printf("⬇️  [Orchestrator] 降格: %d → %d (epoch=%d)", o.activePath.Type, bestPath.Type, newEpoch)

	// 标记旧路径不可用
	o.activePath.Available = false
	o.activePath = bestPath
	o.notifyFECMTU(bestPath.Conn)
	o.setState(StateOrcActive)

	_ = newEpoch // Epoch 已通过 nextEpoch() 递增，序列化时自动使用
	return nil
}

// promote 升格到指定高优先级路径
func (o *Orchestrator) promote(target TransportType) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	targetPath, ok := o.paths[target]
	if !ok || !targetPath.Available || !targetPath.Enabled {
		return fmt.Errorf("目标路径 %d 不可用", target)
	}

	if o.activePath != nil && targetPath.Priority >= o.activePath.Priority {
		return fmt.Errorf("目标路径优先级不高于当前路径")
	}

	o.setState(StateOrcPromoting)
	newEpoch := o.nextEpoch()

	log.Printf("⬆️  [Orchestrator] 升格: %d → %d (epoch=%d)",
		o.activePath.Type, target, newEpoch)

	o.activePath = targetPath
	o.notifyFECMTU(targetPath.Conn)
	o.setState(StateOrcActive)

	return nil
}

// probeLoop 心跳探测循环
func (o *Orchestrator) probeLoop(ctx context.Context) {
	for {
		// 根据当前路径级别选择探测周期
		o.mu.RLock()
		cycle := o.config.ProbeCycle
		if o.activePath != nil && o.activePath.Priority >= PriorityICMP {
			cycle = o.config.ProbeCycleLevel3
		}
		o.mu.RUnlock()

		select {
		case <-time.After(cycle):
			o.probeAllPaths()
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		}
	}
}

// probeAllPaths 探测所有路径
func (o *Orchestrator) probeAllPaths() {
	o.mu.RLock()
	currentType := TransportType(255)
	if o.activePath != nil {
		currentType = o.activePath.Type
	}
	o.mu.RUnlock()

	for tt, p := range o.paths {
		if !p.Enabled || tt == currentType {
			continue
		}

		// 简单探测：尝试发送一个小包
		if p.Conn != nil {
			start := time.Now()
			err := p.Conn.Send([]byte{0x00}) // probe packet
			rtt := time.Since(start)

			o.mu.Lock()
			if err == nil {
				p.Available = true
				p.ProbeSuccess++
				o.auditor.RecordSample(tt, rtt, false)

				// 检查是否满足升格条件
				if p.ProbeSuccess >= o.config.PromoteThreshold &&
					o.activePath != nil && p.Priority < o.activePath.Priority {
					target := tt
					o.mu.Unlock()
					if err := o.promote(target); err != nil {
						log.Printf("⚠️  [Orchestrator] 升格失败: %v", err)
						o.mu.Lock()
						p.ProbeSuccess = 0 // 重置计数器
						o.mu.Unlock()
					}
					continue
				}
			} else {
				p.Available = false
				p.ProbeSuccess = 0
				o.auditor.RecordSample(tt, 0, true)
			}
			o.mu.Unlock()
		}
	}

	// 检查当前路径是否需要降格
	if o.auditor.ShouldDegrade(currentType) {
		if err := o.demote(); err != nil {
			log.Printf("⚠️  [Orchestrator] 降格失败: %v", err)
		}
	}
}

// ParseOrchestratorConfig 从 YAML map 解析配置，缺失字段使用默认值
func ParseOrchestratorConfig(raw map[string]interface{}) OrchestratorConfig {
	cfg := DefaultOrchestratorConfig()

	if orch, ok := raw["orchestrator"].(map[string]interface{}); ok {
		if v, ok := orch["probe_cycle"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				cfg.ProbeCycle = d
			}
		}
		if v, ok := orch["probe_cycle_level3"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				cfg.ProbeCycleLevel3 = d
			}
		}
		if v, ok := orch["promote_threshold"].(int); ok {
			cfg.PromoteThreshold = v
		}
		if v, ok := orch["demote_loss_rate"].(float64); ok {
			cfg.DemoteLossRate = v
		}
		if v, ok := orch["demote_rtt_multiple"].(float64); ok {
			cfg.DemoteRTTMultiple = v
		}
		if v, ok := orch["dual_send_duration"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				cfg.DualSendDuration = d
			}
		}
	}

	if transports, ok := raw["transports"].(map[string]interface{}); ok {
		if q, ok := transports["quic"].(map[string]interface{}); ok {
			if v, ok := q["enabled"].(bool); ok {
				cfg.EnableQUIC = v
			}
		}
		if w, ok := transports["wss"].(map[string]interface{}); ok {
			if v, ok := w["enabled"].(bool); ok {
				cfg.EnableWSS = v
			}
		}
		if wr, ok := transports["webrtc"].(map[string]interface{}); ok {
			if v, ok := wr["enabled"].(bool); ok {
				cfg.EnableWebRTC = v
			}
		}
		if ic, ok := transports["icmp"].(map[string]interface{}); ok {
			if v, ok := ic["enabled"].(bool); ok {
				cfg.EnableICMP = v
			}
		}
		if dn, ok := transports["dns"].(map[string]interface{}); ok {
			if v, ok := dn["enabled"].(bool); ok {
				cfg.EnableDNS = v
			}
			if v, ok := dn["domain"].(string); ok {
				cfg.DNSConfig.Domain = v
			}
			if v, ok := dn["resolver"].(string); ok {
				cfg.DNSConfig.Resolver = v
			}
			if v, ok := dn["query_type"].(string); ok {
				cfg.DNSConfig.QueryType = v
			}
		}
	}

	return cfg
}
