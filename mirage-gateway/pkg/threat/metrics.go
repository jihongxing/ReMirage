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
		)
	})
}
