module mirage-os/gateway-bridge

go 1.25.0

require (
	github.com/hashicorp/raft v1.7.1
	github.com/hashicorp/raft-boltdb/v2 v2.3.0
	github.com/lib/pq v1.10.9
	github.com/redis/go-redis/v9 v9.7.0
	golang.org/x/sys v0.40.0
	google.golang.org/grpc v1.68.1
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
	pgregory.net/rapid v1.1.0
)

require (
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/hashicorp/go-hclog v1.6.2 // indirect
	github.com/hashicorp/go-immutable-radix v1.0.0 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.2 // indirect
	github.com/hashicorp/golang-lru v0.5.0 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/rogpeppe/go-internal v1.8.1 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.etcd.io/bbolt v1.3.5 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241206012308-a4fef0638583 // indirect
)

require (
	github.com/alicebob/miniredis/v2 v2.37.0
	mirage-proto v0.0.0
)

replace mirage-proto => ../../mirage-proto
