package raft

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// mockGovChecker 用于测试的政府 IP 检查器
type mockGovChecker struct {
	govIPs map[string]bool
}

func (m *mockGovChecker) IsGovernmentIP(ip string) bool {
	return m.govIPs[ip]
}

// Feature: mirage-os-completion, Property 10: 威胁等级触发退位
// **Validates: Requirements 7.2**
func TestProperty_ThreatLevelStepDown(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		level := rapid.IntRange(0, 10).Draw(t, "level")
		isLeader := rapid.Bool().Draw(t, "is_leader")

		result := ShouldStepDown(level, isLeader)
		expected := level >= 8 && isLeader

		if result != expected {
			t.Fatalf("ShouldStepDown(level=%d, isLeader=%v) = %v, expected %v",
				level, isLeader, result, expected)
		}
	})
}

// Feature: mirage-os-completion, Property 11: 综合威胁等级计算
// **Validates: Requirements 7.3**
func TestProperty_CalculateOverallThreat(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		now := time.Now()
		numIndicators := rapid.IntRange(0, 20).Draw(t, "num_indicators")
		indicators := make([]ThreatIndicator, numIndicators)

		for i := range indicators {
			severity := ThreatLevel(rapid.IntRange(0, 9).Draw(t, "severity"))
			// 随机时间：最近 10 分钟内
			minutesAgo := rapid.IntRange(0, 10).Draw(t, "minutes_ago")
			indicators[i] = ThreatIndicator{
				Type:      "test",
				Severity:  severity,
				Timestamp: now.Add(-time.Duration(minutesAgo) * time.Minute),
			}
		}

		result := CalculateOverallThreat(indicators, now, 5*time.Minute)

		// 手动计算期望值
		expected := ThreatLevelNone
		for _, ind := range indicators {
			if now.Sub(ind.Timestamp) <= 5*time.Minute {
				if ind.Severity > expected {
					expected = ind.Severity
				}
			}
		}

		if result != expected {
			t.Fatalf("CalculateOverallThreat = %d, expected %d", result, expected)
		}
	})
}

// Feature: mirage-os-completion, Property 12: 政府审计网络检测
// **Validates: Requirements 8.1, 8.3**
func TestProperty_DetectGovernmentAudit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numConns := rapid.IntRange(0, 10).Draw(t, "num_conns")
		numGovIPs := rapid.IntRange(0, 5).Draw(t, "num_gov_ips")

		govIPs := make(map[string]bool)
		for i := 0; i < numGovIPs; i++ {
			ip := rapid.StringMatching(`10\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "gov_ip")
			govIPs[ip] = true
		}

		connections := make(map[string]int)
		hasGovConnection := false
		for i := 0; i < numConns; i++ {
			ip := rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "conn_ip")
			connections[ip] = rapid.IntRange(1, 100).Draw(t, "count")
			if govIPs[ip] {
				hasGovConnection = true
			}
		}

		hasUnplanned := rapid.Bool().Draw(t, "has_unplanned")

		numHops := rapid.IntRange(0, 5).Draw(t, "num_hops")
		hops := make([]RouteHop, numHops)
		hasHopAnomaly := false
		for i := range hops {
			baseline := rapid.IntRange(1, 20).Draw(t, "baseline_hops")
			current := rapid.IntRange(1, 20).Draw(t, "current_hops")
			hops[i] = RouteHop{
				Target:       "target",
				BaselineHops: baseline,
				CurrentHops:  current,
			}
			diff := current - baseline
			if diff < 0 {
				diff = -diff
			}
			if diff > 2 {
				hasHopAnomaly = true
			}
		}

		checker := &mockGovChecker{govIPs: govIPs}
		result := DetectGovernmentAudit(connections, checker, hasUnplanned, hops)
		expected := hasGovConnection || hasUnplanned || hasHopAnomaly

		if result != expected {
			t.Fatalf("DetectGovernmentAudit = %v, expected %v (govConn=%v, unplanned=%v, hopAnomaly=%v)",
				result, expected, hasGovConnection, hasUnplanned, hasHopAnomaly)
		}
	})
}

// Feature: mirage-os-completion, Property 13: DDoS 攻击检测
// **Validates: Requirements 9.1, 9.2, 9.3**
func TestProperty_DetectDDoS(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		baseline := rapid.Uint64Range(0, 1_000_000).Draw(t, "baseline")
		current := rapid.Uint64Range(0, 10_000_000).Draw(t, "current")
		synRate := rapid.Uint64Range(0, 100_000).Draw(t, "syn_rate")
		udpRate := rapid.Uint64Range(0, 200_000).Draw(t, "udp_rate")

		result := DetectDDoS(baseline, current, synRate, udpRate)

		expected := false
		if baseline > 0 && current > baseline*5 {
			expected = true
		}
		if synRate > 10000 {
			expected = true
		}
		if udpRate > 50000 {
			expected = true
		}

		if result != expected {
			t.Fatalf("DetectDDoS(baseline=%d, current=%d, syn=%d, udp=%d) = %v, expected %v",
				baseline, current, synRate, udpRate, result, expected)
		}
	})
}

// Feature: mirage-os-completion, Property 14: 异常流量检测
// **Validates: Requirements 10.1, 10.2, 10.3**
func TestProperty_DetectAnomalousTraffic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mean := rapid.Float64Range(100, 10000).Draw(t, "mean")
		stddev := rapid.Float64Range(1, 1000).Draw(t, "stddev")
		currentTraffic := rapid.Float64Range(0, 50000).Draw(t, "current")

		numIPs := rapid.IntRange(0, 10).Draw(t, "num_ips")
		connByIP := make(map[string]int)
		hasSingleIPOver1000 := false
		for i := 0; i < numIPs; i++ {
			ip := rapid.StringMatching(`192\.168\.\d{1,3}\.\d{1,3}`).Draw(t, "ip")
			count := rapid.IntRange(1, 2000).Draw(t, "conn_count")
			connByIP[ip] = count
			if count > 1000 {
				hasSingleIPOver1000 = true
			}
		}

		expectedCountries := map[string]bool{"US": true, "CH": true, "SG": true}
		numSources := rapid.IntRange(0, 5).Draw(t, "num_sources")
		geoSources := make([]ConnectionSource, numSources)
		unexpectedCount := 0
		for i := range geoSources {
			country := rapid.SampledFrom([]string{"US", "CH", "SG", "RU", "CN", "IR"}).Draw(t, "country")
			count := rapid.IntRange(1, 500).Draw(t, "src_count")
			geoSources[i] = ConnectionSource{Country: country, Count: count}
			if !expectedCountries[country] {
				unexpectedCount += count
			}
		}

		threshold := 100
		baseline := TrafficBaseline{Mean: mean, StdDev: stddev}
		result := DetectAnomalousTraffic(baseline, currentTraffic, connByIP, geoSources, expectedCountries, threshold)

		// 手动计算期望
		deviation := currentTraffic - mean
		if deviation < 0 {
			deviation = -deviation
		}
		has3Sigma := stddev > 0 && deviation > 3*stddev
		hasGeoAnomaly := unexpectedCount > threshold

		expected := has3Sigma || hasSingleIPOver1000 || hasGeoAnomaly

		if result != expected {
			t.Fatalf("DetectAnomalousTraffic = %v, expected %v (3σ=%v, singleIP=%v, geo=%v)",
				result, expected, has3Sigma, hasSingleIPOver1000, hasGeoAnomaly)
		}
	})
}
