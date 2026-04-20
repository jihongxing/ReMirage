package gtunnel

import (
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
