package threat

// IngressAction 入口处置动作
type IngressAction int

const (
	ActionPass     IngressAction = iota // 放行
	ActionObserve                       // 观察（记录但不阻断）
	ActionThrottle                      // 限速
	ActionTrap                          // 引流蜜罐
	ActionDrop                          // 静默丢弃
)

// String 返回动作名称
func (a IngressAction) String() string {
	switch a {
	case ActionPass:
		return "PASS"
	case ActionObserve:
		return "OBSERVE"
	case ActionThrottle:
		return "THROTTLE"
	case ActionTrap:
		return "TRAP"
	case ActionDrop:
		return "DROP"
	default:
		return "UNKNOWN"
	}
}
