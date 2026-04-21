module mirage-gateway

go 1.25.0

require (
	github.com/cilium/ebpf v0.12.3
	github.com/fsnotify/fsnotify v1.7.0
	github.com/prometheus/client_golang v1.19.0

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

require github.com/gorilla/websocket v1.5.3

require github.com/refraction-networking/utls v1.6.7

require (
	github.com/miekg/dns v1.1.72
	github.com/pion/webrtc/v4 v4.0.5
	golang.org/x/crypto v0.46.0
	golang.org/x/time v0.15.0
)

require (
	github.com/andybalholm/brotli v1.0.6 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/circl v1.3.7 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	github.com/pion/datachannel v1.5.9 // indirect
	github.com/pion/dtls/v3 v3.0.4 // indirect
	github.com/pion/ice/v4 v4.0.3 // indirect
	github.com/pion/interceptor v0.1.37 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns/v2 v2.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.14 // indirect
	github.com/pion/rtp v1.8.9 // indirect
	github.com/pion/sctp v1.8.34 // indirect
	github.com/pion/sdp/v3 v3.0.9 // indirect
	github.com/pion/srtp/v3 v3.0.4 // indirect
	github.com/pion/stun/v3 v3.0.0 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/pion/turn/v4 v4.0.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240528184218-531527333157 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

require mirage-proto v0.0.0

replace mirage-proto => ../mirage-proto
