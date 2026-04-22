package entitlement

import "time"

// ServiceClassPolicy 服务等级对应的运行时行为参数
type ServiceClassPolicy struct {
	ReconnBackoffBase   time.Duration // 重连退避基数
	ReconnBackoffMax    time.Duration // 重连退避上限
	ResonanceEnabled    bool          // 绝境发现是否启用
	HeartbeatInterval   time.Duration // 心跳频率
	TopoRefreshInterval time.Duration // 拓扑刷新频率
}

// PolicyForClass returns the runtime behavior params for the given ServiceClass.
func PolicyForClass(class ServiceClass) *ServiceClassPolicy {
	switch class {
	case ClassPlatinum:
		return &ServiceClassPolicy{
			ReconnBackoffBase:   2 * time.Second,
			ReconnBackoffMax:    60 * time.Second,
			ResonanceEnabled:    true,
			HeartbeatInterval:   15 * time.Second,
			TopoRefreshInterval: 5 * time.Minute,
		}
	case ClassDiamond:
		return &ServiceClassPolicy{
			ReconnBackoffBase:   1 * time.Second,
			ReconnBackoffMax:    30 * time.Second,
			ResonanceEnabled:    true,
			HeartbeatInterval:   10 * time.Second,
			TopoRefreshInterval: 2 * time.Minute,
		}
	default: // ClassStandard
		return &ServiceClassPolicy{
			ReconnBackoffBase:   5 * time.Second,
			ReconnBackoffMax:    120 * time.Second,
			ResonanceEnabled:    false,
			HeartbeatInterval:   30 * time.Second,
			TopoRefreshInterval: 10 * time.Minute,
		}
	}
}
