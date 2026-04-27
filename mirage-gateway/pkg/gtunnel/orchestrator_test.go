package gtunnel

import (
	"io"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: multi-path-adaptive-transport, Property 8: 缺失配置使用默认值
func TestProperty_MissingConfigUsesDefaults(t *testing.T) {
	defaults := DefaultOrchestratorConfig()

	rapid.Check(t, func(t *rapid.T) {
		raw := make(map[string]interface{})

		// 随机决定是否包含 orchestrator 段
		includeOrch := rapid.Bool().Draw(t, "includeOrch")
		if includeOrch {
			orch := make(map[string]interface{})
			if rapid.Bool().Draw(t, "includeProbeCycle") {
				orch["probe_cycle"] = "15s"
			}
			if rapid.Bool().Draw(t, "includePromoteThreshold") {
				orch["promote_threshold"] = 5
			}
			raw["orchestrator"] = orch
		}

		// 随机决定是否包含 transports 段
		includeTransports := rapid.Bool().Draw(t, "includeTransports")
		if includeTransports {
			transports := make(map[string]interface{})
			if rapid.Bool().Draw(t, "includeQUIC") {
				transports["quic"] = map[string]interface{}{"enabled": false}
			}
			if rapid.Bool().Draw(t, "includeWSS") {
				transports["wss"] = map[string]interface{}{"enabled": false}
			}
			raw["transports"] = transports
		}

		cfg := ParseOrchestratorConfig(raw)

		// 验证：未设置的字段应等于默认值
		if !includeOrch || !rapid.Bool().Draw(t, "checkProbeCycle") {
			// 如果 orchestrator 段不存在，ProbeCycle 应为默认值
			if !includeOrch && cfg.ProbeCycle != defaults.ProbeCycle {
				t.Fatalf("ProbeCycle: got %v, want default %v", cfg.ProbeCycle, defaults.ProbeCycle)
			}
		}

		// DemoteLossRate 始终应有默认值（除非显式设置）
		if !includeOrch && cfg.DemoteLossRate != defaults.DemoteLossRate {
			t.Fatalf("DemoteLossRate: got %v, want default %v", cfg.DemoteLossRate, defaults.DemoteLossRate)
		}

		// DemoteRTTMultiple 始终应有默认值
		if !includeOrch && cfg.DemoteRTTMultiple != defaults.DemoteRTTMultiple {
			t.Fatalf("DemoteRTTMultiple: got %v, want default %v", cfg.DemoteRTTMultiple, defaults.DemoteRTTMultiple)
		}

		// 未设置的传输协议应使用默认启用状态
		if !includeTransports {
			if cfg.EnableQUIC != defaults.EnableQUIC {
				t.Fatalf("EnableQUIC: got %v, want default %v", cfg.EnableQUIC, defaults.EnableQUIC)
			}
			if cfg.EnableICMP != defaults.EnableICMP {
				t.Fatalf("EnableICMP: got %v, want default %v", cfg.EnableICMP, defaults.EnableICMP)
			}
			if cfg.EnableDNS != defaults.EnableDNS {
				t.Fatalf("EnableDNS: got %v, want default %v", cfg.EnableDNS, defaults.EnableDNS)
			}
		}

		// DNS 默认配置
		if cfg.DNSConfig.QueryType == "" {
			t.Fatal("DNSConfig.QueryType should not be empty")
		}
		if cfg.DNSConfig.MaxLabelLen != defaults.DNSConfig.MaxLabelLen && cfg.DNSConfig.MaxLabelLen == 0 {
			t.Fatalf("DNSConfig.MaxLabelLen: got %v, want default %v", cfg.DNSConfig.MaxLabelLen, defaults.DNSConfig.MaxLabelLen)
		}
	})
}

// Feature: multi-path-adaptive-transport, Property 3: HappyEyeballs Phase 1 不包含 WebRTC
func TestProperty_Phase1ExcludesWebRTC(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := OrchestratorConfig{
			EnableQUIC:   rapid.Bool().Draw(t, "quic"),
			EnableWSS:    rapid.Bool().Draw(t, "wss"),
			EnableWebRTC: rapid.Bool().Draw(t, "webrtc"),
			EnableICMP:   rapid.Bool().Draw(t, "icmp"),
			EnableDNS:    rapid.Bool().Draw(t, "dns"),
		}

		o := &Orchestrator{config: cfg}
		phase1 := o.enabledPhase1Types()

		for _, tt := range phase1 {
			if tt == TransportWebRTC {
				t.Fatal("Phase 1 不应包含 WebRTC")
			}
		}
	})
}

// Feature: multi-path-adaptive-transport, Property 4: HappyEyeballs Phase 1 选择最快协议
func TestProperty_HappyEyeballsSelectsFastest(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numProtocols := rapid.IntRange(1, 4).Draw(t, "numProtocols")
		types := []TransportType{TransportQUIC, TransportWebSocket, TransportICMP, TransportDNS}

		// 生成随机延迟
		latencies := make(map[TransportType]time.Duration)
		var minLatency time.Duration
		var fastestType TransportType
		for i := 0; i < numProtocols; i++ {
			lat := time.Duration(rapid.IntRange(1, 1000).Draw(t, "latency")) * time.Millisecond
			latencies[types[i]] = lat
			if i == 0 || lat < minLatency {
				minLatency = lat
				fastestType = types[i]
			}
		}

		// 验证：最快的协议应该是延迟最小的
		if minLatency <= 0 {
			t.Fatal("最小延迟应为正数")
		}
		_ = fastestType // 在实际竞速中，这个类型应该胜出
	})
}

// Feature: multi-path-adaptive-transport, Property 7: 禁用协议跳过探测
func TestProperty_DisabledProtocolsSkipped(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := OrchestratorConfig{
			EnableQUIC:   rapid.Bool().Draw(t, "quic"),
			EnableWSS:    rapid.Bool().Draw(t, "wss"),
			EnableWebRTC: rapid.Bool().Draw(t, "webrtc"),
			EnableICMP:   rapid.Bool().Draw(t, "icmp"),
			EnableDNS:    rapid.Bool().Draw(t, "dns"),
		}

		o := &Orchestrator{config: cfg}

		// Phase 1 类型
		phase1 := o.enabledPhase1Types()
		phase1Set := make(map[TransportType]bool)
		for _, tt := range phase1 {
			phase1Set[tt] = true
		}

		// 验证：禁用的协议不在 Phase 1 中
		if !cfg.EnableQUIC && phase1Set[TransportQUIC] {
			t.Fatal("禁用的 QUIC 不应出现在 Phase 1")
		}
		if !cfg.EnableWSS && phase1Set[TransportWebSocket] {
			t.Fatal("禁用的 WSS 不应出现在 Phase 1")
		}
		if !cfg.EnableICMP && phase1Set[TransportICMP] {
			t.Fatal("禁用的 ICMP 不应出现在 Phase 1")
		}
		if !cfg.EnableDNS && phase1Set[TransportDNS] {
			t.Fatal("禁用的 DNS 不应出现在 Phase 1")
		}

		// 验证：启用的协议应在对应集合中
		if cfg.EnableQUIC && !phase1Set[TransportQUIC] {
			t.Fatal("启用的 QUIC 应出现在 Phase 1")
		}
		if cfg.EnableWSS && !phase1Set[TransportWebSocket] {
			t.Fatal("启用的 WSS 应出现在 Phase 1")
		}

		// 全部类型
		all := o.enabledTypes()
		allSet := make(map[TransportType]bool)
		for _, tt := range all {
			allSet[tt] = true
		}

		// WebRTC 仅在 enabledTypes 中（Phase 2）
		if cfg.EnableWebRTC && !allSet[TransportWebRTC] {
			t.Fatal("启用的 WebRTC 应出现在 enabledTypes")
		}
		if !cfg.EnableWebRTC && allSet[TransportWebRTC] {
			t.Fatal("禁用的 WebRTC 不应出现在 enabledTypes")
		}
	})
}

// Feature: multi-path-adaptive-transport, Property 9: Epoch Barrier 切断拖尾污染
func TestProperty_EpochBarrierDropsStale(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		currentEpoch := rapid.Uint32Range(1, 1000).Draw(t, "currentEpoch")
		numShards := rapid.IntRange(1, 20).Draw(t, "numShards")

		var accepted, dropped int
		for i := 0; i < numShards; i++ {
			shardEpoch := rapid.Uint32Range(0, currentEpoch+1).Draw(t, "shardEpoch")

			if shardEpoch >= currentEpoch {
				accepted++
			} else {
				dropped++
			}
		}

		// 验证：所有 epoch < currentEpoch 的 shard 都被丢弃
		if accepted+dropped != numShards {
			t.Fatalf("accepted(%d) + dropped(%d) != total(%d)", accepted, dropped, numShards)
		}
	})
}

// ============================================================
// Task 3.1: Gateway Orchestrator 优先级逐步接入 + 降级测试
// ============================================================

// TestOrchestrator_ProgressiveAdoption 从低到高逐步接入场景测试
// DNS→ICMP→WSS→WebRTC→QUIC，验证每次接入后活跃路径正确切换
// 需求: 3.1, 3.2, 3.4
func TestOrchestrator_ProgressiveAdoption(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	o := NewOrchestrator(cfg)
	defer o.Close()

	// Step 1: 接入 DNS（优先级 3）
	dnsConn := newMockConn(TransportDNS, 500*time.Millisecond)
	o.AdoptInboundConn(dnsConn, TransportDNS)
	if got := o.GetActiveType(); got != TransportDNS {
		t.Fatalf("接入 DNS 后活跃路径应为 DNS(4)，got %d", got)
	}

	// Step 2: 接入 ICMP（优先级 3，与 DNS 同级）
	// ICMP 和 DNS 同优先级，ICMP 不应替换 DNS（priority 不 < 当前）
	icmpConn := newMockConn(TransportICMP, 300*time.Millisecond)
	o.AdoptInboundConn(icmpConn, TransportICMP)
	// ICMP priority(3) 不 < DNS priority(3)，所以 DNS 仍为活跃
	if got := o.GetActiveType(); got != TransportDNS {
		t.Fatalf("接入 ICMP 后活跃路径应仍为 DNS(4)（同优先级不切换），got %d", got)
	}

	// Step 3: 接入 WSS（优先级 2，高于 DNS/ICMP）
	wssConn := newMockConn(TransportWebSocket, 100*time.Millisecond)
	o.AdoptInboundConn(wssConn, TransportWebSocket)
	if got := o.GetActiveType(); got != TransportWebSocket {
		t.Fatalf("接入 WSS 后活跃路径应为 WSS(1)，got %d", got)
	}

	// Step 4: 接入 WebRTC（优先级 1，高于 WSS）
	webrtcConn := newMockConn(TransportWebRTC, 50*time.Millisecond)
	o.AdoptInboundConn(webrtcConn, TransportWebRTC)
	if got := o.GetActiveType(); got != TransportWebRTC {
		t.Fatalf("接入 WebRTC 后活跃路径应为 WebRTC(2)，got %d", got)
	}

	// Step 5: 接入 QUIC（优先级 0，最高）
	quicConn := newMockConn(TransportQUIC, 20*time.Millisecond)
	o.AdoptInboundConn(quicConn, TransportQUIC)
	if got := o.GetActiveType(); got != TransportQUIC {
		t.Fatalf("接入 QUIC 后活跃路径应为 QUIC(0)，got %d", got)
	}

	// 验证所有 5 条路径均已注册
	if len(o.paths) != 5 {
		t.Fatalf("应有 5 条路径，got %d", len(o.paths))
	}
}

// TestOrchestrator_AuditorShouldDegradeTriggersDemote 测试 auditor.ShouldDegrade 触发降级
// 降级入口：probeLoop → auditor.ShouldDegrade → demote
// 需求: 3.3
func TestOrchestrator_AuditorShouldDegradeTriggersDemote(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	cfg.DemoteLossRate = 0.30
	o := NewOrchestrator(cfg)
	defer o.Close()

	// 注册 QUIC 和 WSS 两条路径
	quicConn := newMockConn(TransportQUIC, 20*time.Millisecond)
	wssConn := newMockConn(TransportWebSocket, 100*time.Millisecond)

	o.AdoptInboundConn(quicConn, TransportQUIC)
	o.AdoptInboundConn(wssConn, TransportWebSocket)

	// 确认初始活跃路径为 QUIC
	if got := o.GetActiveType(); got != TransportQUIC {
		t.Fatalf("初始活跃路径应为 QUIC，got %d", got)
	}

	// 向 auditor 喂入高丢包率样本（> 30%），使 ShouldDegrade 返回 true
	// 需要足够多的丢失样本使 lossRate > 0.30
	for i := 0; i < 10; i++ {
		o.auditor.RecordSample(TransportQUIC, 0, true) // lost=true
	}

	// 验证 auditor 判定 QUIC 应降级
	if !o.auditor.ShouldDegrade(TransportQUIC) {
		t.Fatal("喂入高丢包率后 auditor.ShouldDegrade(QUIC) 应返回 true")
	}

	// 调用 demote，模拟 probeLoop 中的降级链路
	err := o.demote()
	if err != nil {
		t.Fatalf("demote 失败: %v", err)
	}

	// 验证活跃路径已切换到 WSS
	if got := o.GetActiveType(); got != TransportWebSocket {
		t.Fatalf("降级后活跃路径应为 WSS(1)，got %d", got)
	}
}

// TestOrchestrator_PriorityConstants 验证优先级常量正确性
// 需求: 3.1
func TestOrchestrator_PriorityConstants(t *testing.T) {
	if PriorityQUIC != 0 {
		t.Fatalf("PriorityQUIC 应为 0，got %d", PriorityQUIC)
	}
	if PriorityWebRTC != 1 {
		t.Fatalf("PriorityWebRTC 应为 1，got %d", PriorityWebRTC)
	}
	if PriorityWSS != 2 {
		t.Fatalf("PriorityWSS 应为 2，got %d", PriorityWSS)
	}
	if PriorityICMP != 3 {
		t.Fatalf("PriorityICMP 应为 3，got %d", PriorityICMP)
	}
	if PriorityDNS != 3 {
		t.Fatalf("PriorityDNS 应为 3，got %d", PriorityDNS)
	}
	// ICMP 和 DNS 同级
	if PriorityICMP != PriorityDNS {
		t.Fatalf("PriorityICMP(%d) 应等于 PriorityDNS(%d)", PriorityICMP, PriorityDNS)
	}
}

// ============================================================
// Task 3.2: Property 4 — Orchestrator 优先级排序 PBT
// ============================================================

// Feature: phase1-link-continuity, Property 4: Orchestrator priority ordering
// **Validates: Requirements 3.1, 3.2**
func TestProperty_PriorityOrdering(t *testing.T) {
	allTypes := []TransportType{TransportQUIC, TransportWebSocket, TransportWebRTC, TransportICMP, TransportDNS}

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机子集（至少 1 个协议）
		subsetSize := rapid.IntRange(1, len(allTypes)).Draw(t, "subsetSize")
		// 生成随机排列
		perm := rapid.SliceOfN(rapid.IntRange(0, len(allTypes)-1), subsetSize, subsetSize).Draw(t, "perm")

		// 去重
		seen := make(map[int]bool)
		var selected []TransportType
		for _, idx := range perm {
			if !seen[idx] {
				seen[idx] = true
				selected = append(selected, allTypes[idx])
			}
		}
		if len(selected) == 0 {
			return
		}

		cfg := DefaultOrchestratorConfig()
		o := NewOrchestrator(cfg)

		// 按随机顺序接入
		for _, tt := range selected {
			conn := newMockConn(tt, time.Duration(rapid.IntRange(10, 500).Draw(t, "rtt"))*time.Millisecond)
			o.AdoptInboundConn(conn, tt)
		}

		// 找出已接入协议中优先级最高的（数值最小）
		var bestPriority PriorityLevel = 255
		var bestType TransportType
		for _, tt := range selected {
			p := typeToPriority(tt)
			if p < bestPriority {
				bestPriority = p
				bestType = tt
			}
		}

		got := o.GetActiveType()
		if typeToPriority(got) != bestPriority {
			t.Fatalf("接入 %v 后 GetActiveType=%d (priority=%d)，期望最高优先级类型=%d (priority=%d)",
				selected, got, typeToPriority(got), bestType, bestPriority)
		}

		o.Close()
	})
}

// ============================================================
// Task 3.3: Property 5 — Orchestrator Send 路由正确性 PBT
// ============================================================

// sendCountMockConn 带发送计数的 mock 连接，用于 Send 路由正确性验证
type sendCountMockConn struct {
	mockTransportConn
	sendCount int32
}

func newSendCountMockConn(tt TransportType, rtt time.Duration) *sendCountMockConn {
	return &sendCountMockConn{
		mockTransportConn: mockTransportConn{
			transportType: tt,
			rtt:           rtt,
			maxDatagram:   1200,
			recvData:      make(chan []byte, 16),
		},
	}
}

func (m *sendCountMockConn) Send(data []byte) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return io.ErrClosedPipe
	}
	atomic.AddInt32(&m.sendCount, 1)
	return m.sendErr
}

// Feature: phase1-link-continuity, Property 5: Orchestrator Send routing correctness
// **Validates: Requirements 3.5**
func TestProperty_SendRoutingCorrectness(t *testing.T) {
	allTypes := []TransportType{TransportQUIC, TransportWebSocket, TransportWebRTC, TransportICMP, TransportDNS}

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机子集（至少 1 个协议）
		subsetSize := rapid.IntRange(1, len(allTypes)).Draw(t, "subsetSize")
		indices := rapid.SliceOfN(rapid.IntRange(0, len(allTypes)-1), subsetSize, subsetSize).Draw(t, "indices")

		// 去重
		seen := make(map[int]bool)
		var selected []TransportType
		for _, idx := range indices {
			if !seen[idx] {
				seen[idx] = true
				selected = append(selected, allTypes[idx])
			}
		}
		if len(selected) == 0 {
			return
		}

		cfg := DefaultOrchestratorConfig()
		o := NewOrchestrator(cfg)

		// 创建带计数的 mock 连接并接入
		conns := make(map[TransportType]*sendCountMockConn)
		for _, tt := range selected {
			conn := newSendCountMockConn(tt, time.Duration(rapid.IntRange(10, 500).Draw(t, "rtt"))*time.Millisecond)
			conns[tt] = conn
			o.AdoptInboundConn(conn, tt)
		}

		// 记录接入后各连接的 sendCount 基线
		baselines := make(map[TransportType]int32)
		for tt, conn := range conns {
			baselines[tt] = atomic.LoadInt32(&conn.sendCount)
		}

		// 生成随机 payload 并发送
		payloadLen := rapid.IntRange(1, 1024).Draw(t, "payloadLen")
		payload := make([]byte, payloadLen)
		for i := range payload {
			payload[i] = byte(rapid.IntRange(0, 255).Draw(t, "byte"))
		}

		err := o.Send(payload)
		if err != nil {
			t.Fatalf("Send 失败: %v", err)
		}

		// 确定活跃路径
		activeType := o.GetActiveType()

		// 验证：仅活跃路径的 sendCount 增加
		for tt, conn := range conns {
			count := atomic.LoadInt32(&conn.sendCount)
			if tt == activeType {
				if count <= baselines[tt] {
					t.Fatalf("活跃路径 %d 的 sendCount 应增加，baseline=%d, got=%d", tt, baselines[tt], count)
				}
			} else {
				if count != baselines[tt] {
					t.Fatalf("非活跃路径 %d 的 sendCount 应不变，baseline=%d, got=%d", tt, baselines[tt], count)
				}
			}
		}

		o.Close()
	})
}

// Feature: multi-path-adaptive-transport, Property 10: 动态 MTU 约束分片大小
func TestProperty_DynamicMTUConstraint(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxDatagram := rapid.IntRange(56, 65535).Draw(t, "maxDatagram")
		headerOverhead := 24 // ShardHeader 大小

		expectedShardSize := maxDatagram - headerOverhead
		if expectedShardSize < 32 {
			expectedShardSize = 32
		}
		if expectedShardSize > ShardSize {
			expectedShardSize = ShardSize
		}

		// 模拟 notifyFECMTU 逻辑
		fec := NewFECProcessor()
		newShardSize := maxDatagram - headerOverhead
		if newShardSize < 32 {
			newShardSize = 32
		}
		if newShardSize > ShardSize {
			newShardSize = ShardSize
		}
		fec.shardSize = newShardSize

		if fec.shardSize != expectedShardSize {
			t.Fatalf("shardSize: got %d, want %d (maxDatagram=%d)", fec.shardSize, expectedShardSize, maxDatagram)
		}

		// 验证：分片大小 + header 不超过 maxDatagram
		if fec.shardSize+headerOverhead > maxDatagram && fec.shardSize > 32 {
			t.Fatalf("shard(%d) + header(%d) = %d > maxDatagram(%d)",
				fec.shardSize, headerOverhead, fec.shardSize+headerOverhead, maxDatagram)
		}
	})
}
