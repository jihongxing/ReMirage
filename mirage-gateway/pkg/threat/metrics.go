package threat

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// IngressRejectTotal 入口拒绝计数器（按 gateway_id, action 分标签）
	IngressRejectTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_ingress_reject_total",
		Help: "Total number of ingress rejections by action type",
	}, []string{"gateway_id", "action"})

	// HoneypotHitTotal 蜜罐命中计数器
	HoneypotHitTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_honeypot_hit_total",
		Help: "Total number of honeypot hits",
	}, []string{"gateway_id"})

	// BlacklistHitTotal 黑名单命中计数器
	BlacklistHitTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_blacklist_hit_total",
		Help: "Total number of blacklist hits",
	}, []string{"gateway_id"})

	// ThreatEscalationTotal 威胁升级计数器
	ThreatEscalationTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_threat_escalation_total",
		Help: "Total number of threat level escalations",
	}, []string{"gateway_id"})

	// AuthFailureTotal 鉴权失败计数器
	AuthFailureTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_auth_failure_total",
		Help: "Total number of command auth failures",
	}, []string{"gateway_id"})

	// MTLSErrorTotal mTLS 错误计数器
	MTLSErrorTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_mtls_error_total",
		Help: "Total number of mTLS errors",
	}, []string{"gateway_id"})

	// SecurityStateGauge 安全状态机当前状态
	SecurityStateGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mirage_security_state",
		Help: "Current security FSM state (0=Normal,1=Alert,2=HighPressure,3=Isolated,4=Silent)",
	}, []string{"gateway_id"})

	// L1 纵深防御指标（需求 2 速率限制 / 需求 1 ASN / 需求 3 静默）

	// ASNDropTotal ASN 黑名单丢弃计数器
	ASNDropTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_asn_drop_total",
		Help: "Total ASN blocklist drops at XDP layer",
	}, []string{"gateway_id"})

	// RateLimitDropTotal 速率限制丢弃计数器
	RateLimitDropTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_ratelimit_drop_total",
		Help: "Total rate limit drops at XDP layer",
	}, []string{"gateway_id", "trigger_type"})

	// SilentDropTotal 静默响应丢弃计数器
	SilentDropTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_silent_drop_total",
		Help: "Total silent response drops at TC layer",
	}, []string{"gateway_id"})

	// HandshakeTimeoutTotal 握手超时计数器
	HandshakeTimeoutTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_handshake_timeout_total",
		Help: "Total handshake timeouts",
	}, []string{"gateway_id"})

	// ProtocolScanTotal 协议扫描检测计数器
	ProtocolScanTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_protocol_scan_total",
		Help: "Total protocol scan detections",
	}, []string{"gateway_id", "protocol"})

	// BehaviorAnomalyTotal 行为异常检测计数器
	BehaviorAnomalyTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_behavior_anomaly_total",
		Help: "Total behavior anomaly detections",
	}, []string{"gateway_id"})

	// ThreatIntelLookupTotal 威胁情报查询计数器
	ThreatIntelLookupTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_threat_intel_lookup_total",
		Help: "Total threat intel lookups",
	}, []string{"gateway_id", "result"})

	// 多维准入评分器指标

	// admissionScoreHistogram 准入评分分布
	admissionScoreHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "mirage_admission_score",
		Help:    "Distribution of admission scores",
		Buckets: []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
	})

	// admissionActionTotal 准入动作计数（移除 ip 标签避免时间序列膨胀）
	admissionActionTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_admission_action_total",
		Help: "Total admission actions by action type",
	}, []string{"action"})
)

var (
	metricsGatewayID string
	metricsOnce      sync.Once
)

// SetGatewayID 设置指标使用的 Gateway ID
func SetGatewayID(id string) {
	metricsGatewayID = id
}

// GetGatewayID 获取当前 Gateway ID
func GetGatewayID() string {
	return metricsGatewayID
}

// RegisterMetrics 注册所有 Prometheus 指标
func RegisterMetrics() {
	metricsOnce.Do(func() {
		prometheus.MustRegister(
			IngressRejectTotal,
			HoneypotHitTotal,
			BlacklistHitTotal,
			ThreatEscalationTotal,
			AuthFailureTotal,
			MTLSErrorTotal,
			SecurityStateGauge,
			ASNDropTotal,
			RateLimitDropTotal,
			SilentDropTotal,
			HandshakeTimeoutTotal,
			ProtocolScanTotal,
			BehaviorAnomalyTotal,
			ThreatIntelLookupTotal,
			admissionScoreHistogram,
			admissionActionTotal,
		)
	})
}
