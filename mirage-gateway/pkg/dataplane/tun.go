package dataplane

// TUNConfig TUN 设备配置。
type TUNConfig struct {
	DeviceName string // 设备名，默认 "mirage0"
	MTU        int    // MTU，默认 1400
	Subnet     string // TUN 子网地址，默认 "10.7.0.1/24"（Gateway 侧用 .1）
}

// DefaultTUNConfig 默认配置。
func DefaultTUNConfig() TUNConfig {
	return TUNConfig{
		DeviceName: "mirage0",
		MTU:        1400,
		Subnet:     "10.7.0.1/24",
	}
}
