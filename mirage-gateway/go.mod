module mirage-gateway

go 1.24.0

require (
	github.com/cilium/ebpf v0.12.3
	github.com/fsnotify/fsnotify v1.7.0

	// netlink + netns: TC clsact qdisc 创建 & BPF filter 挂载（loader.go）
	// Linux 专用，Windows/macOS 上 IDE 会报红但不影响交叉编译
	// 请勿在 go mod tidy 时误删，否则 eBPF TC 挂载链路断裂
	github.com/vishvananda/netlink v1.1.0
	github.com/vishvananda/netns v0.0.4 // indirect; netlink 的传递依赖，必须显式声明
	go.etcd.io/bbolt v1.4.3
	go.yaml.in/yaml/v2 v2.4.4
	golang.org/x/sys v0.40.0
	google.golang.org/grpc v1.65.0
	pgregory.net/rapid v1.1.0
)

require github.com/quic-go/quic-go v0.59.0

require (
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240528184218-531527333157 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)
