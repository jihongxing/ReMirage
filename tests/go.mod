module mirage-gateway/tests

go 1.24.0

require mirage-gateway v0.0.0

replace mirage-gateway => ../mirage-gateway

require (
	github.com/cilium/ebpf v0.12.3
	github.com/fsnotify/fsnotify v1.7.0
	go.etcd.io/bbolt v1.4.3
	go.yaml.in/yaml/v2 v2.4.4
	golang.org/x/sys v0.40.0
	google.golang.org/grpc v1.65.0
	pgregory.net/rapid v1.1.0
)

require (
	golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240528184218-531527333157 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)
