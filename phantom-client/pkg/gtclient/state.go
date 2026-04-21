package gtclient

// ConnState represents the connection state of the GTunnel client.
type ConnState int32

const (
	StateInit          ConnState = iota // 初始化
	StateBootstrapping                  // 正在探测 bootstrap 节点
	StateConnected                      // 已连接
	StateDegraded                       // 连接质量下降
	StateReconnecting                   // 正在重连
	StateExhausted                      // 所有重连策略耗尽
	StateStopped                        // 已停止
)

var stateNames = [...]string{
	"Init",
	"Bootstrapping",
	"Connected",
	"Degraded",
	"Reconnecting",
	"Exhausted",
	"Stopped",
}

func (s ConnState) String() string {
	if int(s) >= 0 && int(s) < len(stateNames) {
		return stateNames[s]
	}
	return "Unknown"
}
