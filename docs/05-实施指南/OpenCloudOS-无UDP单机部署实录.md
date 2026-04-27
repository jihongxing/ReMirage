# OpenCloudOS 无 UDP 单机部署实录

本文记录 2026-04-28 在 OpenCloudOS 单机环境中跑通 ReMirage 的完整部署流程。该环境没有可用 UDP，因此部署目标是 TCP/WSS 降级模式：保留 Gateway、Gateway Bridge、PostgreSQL、Redis、eBPF 数据面和 systemd 自启动，但关闭公开 QUIC/H3 监听。

## 适用范围

已验证环境：

| 项 | 值 |
|----|----|
| 系统 | OpenCloudOS 9 |
| 内核 | 6.6.47-12.oc9.x86_64 |
| Go | go1.25.5 linux/amd64 |
| clang | 17.0.6 |
| Docker Compose | v5.1.3 |
| 部署目录 | /opt/ReMirage |
| 运行用户 | root |
| 网络限制 | 无可用 UDP，仅 TCP/WSS |

目标状态：

| 组件 | 状态 |
|------|------|
| mirage-postgres | Docker 容器，开机自启 |
| mirage-redis | Docker 容器，开机自启 |
| mirage-gateway-bridge | systemd 托管，监听 :50051 和 127.0.0.1:7000 |
| mirage-gateway | systemd 托管，监听 :8443、:50847、127.0.0.1:8081 |
| QUIC/H3 | 关闭，`data_plane.enable_quic: false` |
| Gateway 注册 | ONLINE |
| UDP 监听 | 无 |

## 前置原则

1. 无 UDP 环境必须保持 `data_plane.enable_quic: false`。
2. Gateway 对外承载使用 Chameleon WSS/TCP 降级通道，默认监听 `:8443/api/v2/stream`。
3. 低内存服务器构建 Go 程序时建议开启 swap，并使用 `GOMAXPROCS=1 GOMEMLIMIT=350MiB go build -p 1`。
4. 开发证书脚本生成的证书有效期较短，适合开发验证。生产环境应改为 CSR 或正式证书发行流程。
5. 当前 OpenCloudOS 6.6 内核上，B-DNA 和部分 L1 统计存在可降级问题，见本文“已知非致命告警”。

## 代码与配置要求

部署前应确保仓库已经包含以下改动，或在服务器上临时打等价补丁。

| 文件 | 要求 |
|------|------|
| `deploy/scripts/ebpf-preflight.sh` | `set -e` 下计数器不能使用会返回 1 的 `((VAR++))`，应使用 `((++VAR))` |
| `deploy/scripts/sysctl-tuning.sh` | 同上 |
| `deploy/scripts/udp-qualify.sh` | 同上 |
| `mirage-gateway/cmd/gateway/main.go` | `DataPlane` 配置结构包含 `EnableQUIC bool yaml:"enable_quic"` |
| `mirage-gateway/cmd/gateway/main.go` | QUIC/H3 listener 受 `cfg.DataPlane.EnableQUIC` 控制 |
| `mirage-gateway/configs/gateway.yaml` | `data_plane.enable_quic: false` |
| `mirage-gateway/bpf/npm.c` | 避免在内核态用动态 map-value payload 调用 `bpf_skb_store_bytes` |
| `mirage-gateway/pkg/ebpf/loader.go` | B-DNA 在该内核上可设为非 critical，失败后降级运行 |

关键检查：

```bash
cd /opt/ReMirage

grep -n 'EnableQUIC\|enable_quic\|cfg.DataPlane.EnableQUIC' \
  mirage-gateway/cmd/gateway/main.go \
  mirage-gateway/configs/gateway.yaml

grep -n 'B-DNA 指纹拟态' mirage-gateway/pkg/ebpf/loader.go
```

## 1. 基础环境预检

```bash
uname -r
go version
clang --version | head -1
docker compose version

cd /opt/ReMirage
bash deploy/scripts/ebpf-preflight.sh
```

预期：

```text
环境满足 Mirage-Gateway eBPF 运行要求
```

可接受警告：

```text
RLIMIT_MEMLOCK: 8192 KB
非特权 BPF 未禁用
docker0 / bridge / veth 仅支持 XDP generic
```

## 2. 低内存构建准备

如果服务器内存接近 2 GiB 或更低，先创建临时 swap：

```bash
free -h

fallocate -l 2G /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=2048
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile

grep -q '^/swapfile ' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
free -h
```

说明：swap 用于构建期稳定性。高安全生产模式应另行评估是否允许长期启用 swap。

## 3. 构建 Mirage-Gateway

```bash
cd /opt/ReMirage/mirage-gateway

make clean
make all

ls -lah bin/mirage-gateway bpf/*.o
```

预期：

```text
bin/mirage-gateway
bpf/bdna.o
bpf/chameleon.o
bpf/h3_shaper.o
bpf/icmp_tunnel.o
bpf/jitter.o
bpf/l1_defense.o
bpf/l1_silent.o
bpf/npm.o
bpf/phantom.o
bpf/sock_hijack.o
bpf/sockmap.o
```

## 4. 生成开发证书并写入 Gateway 配置

```bash
cd /opt/ReMirage

bash deploy/certs/gen_root_ca.sh /etc/mirage/certs
bash deploy/certs/gen_gateway_cert.sh gw-dev /etc/mirage/certs
bash deploy/certs/gen_os_cert.sh /etc/mirage/certs

cp mirage-gateway/configs/gateway.yaml \
  mirage-gateway/configs/gateway.yaml.bak.$(date +%s)

perl -pi -e 's|cert_file: ""|cert_file: "/etc/mirage/certs/gateway.crt"|g; s|key_file: ""|key_file: "/etc/mirage/certs/gateway.key"|g; s|ca_file: ""|ca_file: "/etc/mirage/certs/ca.crt"|g; s|endpoint: "10\.99\.0\.10:50051"|endpoint: "127.0.0.1:50051"|g' \
  mirage-gateway/configs/gateway.yaml

SECRET="$(openssl rand -hex 32)"
perl -0pi -e "s|command_secret: \".*?\"|command_secret: \"$SECRET\"|" \
  mirage-gateway/configs/gateway.yaml

grep -nE 'endpoint:|enable_quic:|command_secret:|cert_file:|key_file:|ca_file:' \
  mirage-gateway/configs/gateway.yaml
```

必须确认：

```yaml
mcc:
  endpoint: "127.0.0.1:50051"

data_plane:
  enable_quic: false
```

## 5. 启动 PostgreSQL 和 Redis

```bash
cd /opt/ReMirage/mirage-os

docker compose -f docker-compose.dev.yml up -d
docker update --restart unless-stopped mirage-postgres mirage-redis
```

Docker Compose 可能提示 `version` 字段过时，该提示不影响运行。

## 6. 初始化开发数据库表

当正式迁移脚本不可用或不完整时，可用以下 SQL 初始化 Gateway Bridge 所需的最小表集合：

```bash
docker exec -i mirage-postgres psql -U postgres -d mirage_os <<'SQL'
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS cells (
  id TEXT PRIMARY KEY,
  cost_multiplier NUMERIC(20,8) DEFAULT 1.0
);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  cell_id TEXT,
  remaining_quota NUMERIC(20,8) DEFAULT 0,
  total_consumed NUMERIC(20,8) DEFAULT 0,
  is_active BOOLEAN DEFAULT true,
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS gateways (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  gateway_id TEXT UNIQUE NOT NULL,
  cell_id TEXT,
  ip_address TEXT,
  status TEXT DEFAULT 'OFFLINE',
  ebpf_loaded BOOLEAN DEFAULT false,
  last_heartbeat_at TIMESTAMPTZ,
  current_threat_level INT DEFAULT 0,
  active_connections INT DEFAULT 0,
  memory_bytes BIGINT DEFAULT 0,
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  downlink_addr TEXT,
  version TEXT,
  max_sessions INT DEFAULT 0,
  active_sessions INT DEFAULT 0
);

CREATE TABLE IF NOT EXISTS gateway_sessions (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  gateway_id TEXT,
  status TEXT DEFAULT 'active',
  disconnected_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS billing_logs (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  user_id TEXT,
  gateway_id TEXT,
  business_bytes BIGINT DEFAULT 0,
  defense_bytes BIGINT DEFAULT 0,
  business_cost NUMERIC(20,8) DEFAULT 0,
  defense_cost NUMERIC(20,8) DEFAULT 0,
  total_cost NUMERIC(20,8) DEFAULT 0,
  period_seconds INT DEFAULT 0,
  session_id TEXT,
  sequence_number BIGINT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS threat_intel (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  source_ip TEXT,
  source_port INT,
  threat_type TEXT,
  severity INT DEFAULT 0,
  hit_count INT DEFAULT 1,
  is_banned BOOLEAN DEFAULT false,
  first_seen TIMESTAMPTZ DEFAULT NOW(),
  last_seen TIMESTAMPTZ DEFAULT NOW(),
  reported_by_gateway TEXT,
  ttl_seconds INT DEFAULT 3600,
  expires_at TIMESTAMPTZ,
  source TEXT DEFAULT 'auto'
);
SQL
```

## 7. 写入 Mirage-OS Bridge 配置

```bash
mkdir -p /etc/mirage /var/lib/mirage/raft

cat >/etc/mirage/mirage-os.yaml <<'YAML'
grpc:
  port: 50051
  tls_enabled: true
  cert_file: "/etc/mirage/certs/os.crt"
  key_file: "/etc/mirage/certs/os.key"
  ca_file: "/etc/mirage/certs/ca.crt"
  allowed_cns:
    - "mirage-gateway-gw-dev"

rest:
  addr: "127.0.0.1:7000"
  internal_secret: "dev_internal_secret"

database:
  dsn: "postgres://postgres:postgres@127.0.0.1:5432/mirage_os?sslmode=disable"

redis:
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

quota:
  business_price_per_gb: 0.10
  defense_price_per_gb: 0.05

intel:
  ban_threshold: 100
  cleanup_days: 30
  cleanup_min_hits: 10

raft:
  node_id: ""
  data_dir: "/var/lib/mirage/raft"
YAML
```

## 8. 构建 Gateway Bridge

低内存服务器建议限制并行度：

```bash
cd /opt/ReMirage/mirage-os/gateway-bridge
mkdir -p bin

GOMAXPROCS=1 GOMEMLIMIT=350MiB go build -p 1 -o bin/gateway-bridge ./cmd/bridge/main.go

ls -lah bin/gateway-bridge
```

## 9. 前台冒烟验证

先启动 bridge：

```bash
cd /opt/ReMirage/mirage-os/gateway-bridge
CONFIG_PATH=/etc/mirage/mirage-os.yaml MIRAGE_ENV=development ./bin/gateway-bridge
```

另开终端启动 Gateway：

```bash
cd /opt/ReMirage/mirage-gateway

tc qdisc del dev eth0 clsact 2>/dev/null || true
ip link set dev eth0 xdp off 2>/dev/null || true
ip link set dev eth0 xdpgeneric off 2>/dev/null || true

MIRAGE_ENV=development ./bin/mirage-gateway -config configs/gateway.yaml
```

预期关键日志：

```text
[INFO] gRPC server listening on :50051 (TLS=true)
[Registry] Gateway gw-... 注册成功
QUIC/H3 bearer listener disabled: data_plane.enable_quic=false
[gRPC Client] Gateway 注册成功
健康检查端点: http://127.0.0.1:8081/healthz
Mirage-Gateway 启动完成
```

前台验证完成后，用 `Ctrl+C` 停掉两个进程，再进入 systemd 托管。

## 10. 创建 systemd 服务

Gateway Bridge：

```bash
cat >/etc/systemd/system/mirage-gateway-bridge.service <<'EOF'
[Unit]
Description=Mirage Gateway Bridge
After=network-online.target docker.service
Wants=network-online.target docker.service

[Service]
Type=simple
WorkingDirectory=/opt/ReMirage/mirage-os/gateway-bridge
Environment=CONFIG_PATH=/etc/mirage/mirage-os.yaml
Environment=MIRAGE_ENV=development
ExecStartPre=/bin/bash -lc 'docker start mirage-postgres mirage-redis >/dev/null 2>&1 || true'
ExecStartPre=/bin/bash -lc 'for i in {1..30}; do docker exec mirage-postgres pg_isready -U postgres -d mirage_os && exit 0; sleep 1; done; exit 1'
ExecStart=/opt/ReMirage/mirage-os/gateway-bridge/bin/gateway-bridge
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
```

Gateway：

```bash
cat >/etc/systemd/system/mirage-gateway.service <<'EOF'
[Unit]
Description=Mirage Gateway
After=network-online.target mirage-gateway-bridge.service
Wants=network-online.target mirage-gateway-bridge.service

[Service]
Type=simple
WorkingDirectory=/opt/ReMirage/mirage-gateway
Environment=MIRAGE_ENV=development
Environment=MIRAGE_TMPFS_MODE=1
Environment=GOMAXPROCS=2
ExecStartPre=/bin/bash -lc 'tc qdisc del dev eth0 clsact 2>/dev/null || true'
ExecStartPre=/bin/bash -lc 'ip link set dev eth0 xdp off 2>/dev/null || true'
ExecStartPre=/bin/bash -lc 'ip link set dev eth0 xdpgeneric off 2>/dev/null || true'
ExecStart=/opt/ReMirage/mirage-gateway/bin/mirage-gateway -config /opt/ReMirage/mirage-gateway/configs/gateway.yaml
Restart=always
RestartSec=5
LimitNOFILE=65535
LimitMEMLOCK=infinity
AmbientCapabilities=CAP_BPF CAP_NET_ADMIN CAP_SYS_ADMIN CAP_PERFMON
CapabilityBoundingSet=CAP_BPF CAP_NET_ADMIN CAP_SYS_ADMIN CAP_PERFMON

[Install]
WantedBy=multi-user.target
EOF
```

启用：

```bash
systemctl daemon-reload
systemctl enable --now mirage-gateway-bridge
sleep 3
systemctl enable --now mirage-gateway
```

## 11. 验收命令

```bash
systemctl status mirage-gateway-bridge --no-pager -l
systemctl status mirage-gateway --no-pager -l

ss -lntp | grep -E ':(50051|7000|8443|50847|8081)\b'
curl -s http://127.0.0.1:8081/healthz; echo
grep -n 'enable_quic:' /opt/ReMirage/mirage-gateway/configs/gateway.yaml

ss -lunp | grep -E 'mirage|gateway|:8443|:443|:50051|:50847' || echo "OK: Mirage 没有 UDP 监听"

docker exec -i mirage-postgres psql -U postgres -d mirage_os <<'SQL'
SELECT gateway_id, status, downlink_addr, last_heartbeat_at, ebpf_loaded, active_connections
FROM gateways
ORDER BY updated_at DESC
LIMIT 5;
SQL
```

通过标准：

```text
mirage-gateway-bridge.service: active (running)
mirage-gateway.service: active (running)
curl healthz: OK
enable_quic: false
UDP 监听: OK: Mirage 没有 UDP 监听
DB: gateway status = ONLINE, ebpf_loaded = t
```

已验证输出示例：

```text
gw-a99266abd945 | ONLINE | 0.0.0.0:50847 | ... | t | 0
```

## 12. eBPF load 与 runtime attach 证据

完整环境验收不应只停留在编译成功。建议按以下顺序收集 eBPF 证据：

```bash
cd /opt/ReMirage/mirage-gateway

# runtime attach 会走真实 Loader，包括 cgroup sockops。
# 为避免和正在运行的 Gateway 抢 attach 点，先停 Gateway，bridge 可保持运行。
sudo systemctl stop mirage-gateway

# 1. 编译并要求所有对象通过 verifier load。
sudo REQUIRE_EBPF_LOAD=1 ./scripts/smoke-ebpf-load.sh

# 2. 使用临时 veth 验证 Go Loader 的真实 attach/detach 路径。
sudo ./scripts/smoke-ebpf-runtime-attach.sh

sudo systemctl start mirage-gateway
```

如果正在收敛 B-DNA verifier 问题，可以开启严格 map 检查：

```bash
sudo systemctl stop mirage-gateway
sudo REQUIRE_BDNA_MAPS=1 ./scripts/smoke-ebpf-runtime-attach.sh
sudo systemctl start mirage-gateway
```

验收解释：

| 结果 | 含义 |
|------|------|
| `smoke-ebpf-load.sh` 通过 | `.o` 不只是能编译，也能进入 verifier |
| `smoke-ebpf-runtime-attach.sh` 通过 | Go Loader 能创建临时 veth，完成 TC/XDP/cgroup attach，再 detach 清理 |
| `npm_target_distribution_map` 可见 | NPM MIMIC 分布 map 已被控制面发现 |
| B-DNA maps 可见 | `conn_profile_map`、`profile_select_map`、`profile_count_map` 已被控制面发现 |
| B-DNA maps 缺失但非严格模式通过 | 当前仍是 B-DNA 降级运行，主链可用，但 B-DNA 证据未闭环 |

## 13. 重启验证

```bash
reboot
```

重连后执行：

```bash
systemctl status mirage-gateway-bridge --no-pager -l
systemctl status mirage-gateway --no-pager -l
ss -lntp | grep -E ':(50051|7000|8443|50847|8081)\b'
curl -s http://127.0.0.1:8081/healthz; echo
grep -n 'enable_quic:' /opt/ReMirage/mirage-gateway/configs/gateway.yaml
ss -lunp | grep -E 'mirage|gateway|:8443|:443|:50051|:50847' || echo "OK: Mirage 没有 UDP 监听"
```

## 已知非致命告警

### B-DNA verifier 复杂度

现象：

```text
加载 B-DNA 指纹拟态 (TC) 失败（降级运行）
The sequence of 8193 jumps is too complex
fingerprint_map not found
```

处理：

在该内核上将 B-DNA 标记为非 critical，Gateway 可降级运行。后续应重构 `bdna.c` 中过复杂的 verifier 路径，而不是长期依赖降级。

### L1 per-cpu map 读取

现象：

```text
[L1Monitor] 读取 l1_stats_map 失败: per-cpu value requires pointer to slice
```

处理：

该问题影响统计读取日志，不影响 Gateway 注册、TCP/WSS 通道和主流程。后续应在 Go 侧按 per-cpu map 语义传入 slice。

### 证书 tmpfs 告警

现象：

```text
[ScorchedEarth] 安全告警: 证书路径 /etc/mirage/certs/... 不在 tmpfs 中
```

处理：

开发验证可接受。生产环境建议将证书放入 `/var/mirage/certs` 并挂载 tmpfs，或使用正式密钥注入流程。

## 常用维护命令

```bash
systemctl status mirage-gateway-bridge mirage-gateway --no-pager -l
journalctl -u mirage-gateway-bridge -n 100 --no-pager
journalctl -u mirage-gateway -n 120 --no-pager
curl -s http://127.0.0.1:8081/healthz; echo

docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
docker exec -i mirage-postgres psql -U postgres -d mirage_os -c '\dt'

ss -lntp | grep -E ':(50051|7000|8443|50847|8081)\b'
ss -lunp | grep -E 'mirage|gateway|:8443|:443|:50051|:50847' || echo "OK: Mirage 没有 UDP 监听"
```

## 当前验收结论模板

```text
Mirage-OS bridge: OK
Mirage-Gateway: OK
Gateway DB status: ONLINE
eBPF loaded: true
TCP/WSS mode: OK
QUIC/UDP: disabled
UDP listeners: none
systemd autostart: OK
healthz: OK
```
