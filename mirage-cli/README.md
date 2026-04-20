# Mirage CLI 使用说明

Mirage Gateway 管理工具 — 融合网关状态查询、隧道控制、认证签名、系统诊断。

## 安装

```bash
# 从源码编译
cd mirage-cli
go build -o mirage-cli .

# 或使用构建脚本（交叉编译全平台）
bash build.sh
# 产物在 bin/ 目录：linux-amd64/arm64, windows-amd64, darwin-amd64/arm64
```

## 全局参数

```
-g, --gateway string   Gateway 健康检查地址 (默认 "127.0.0.1:9090")
-c, --config string    配置文件路径 (默认 /etc/mirage/gateway.yaml)
```

---

## 命令一览

| 命令 | 说明 |
|------|------|
| `version` | 显示版本信息 |
| `status` | 查询 Gateway 运行状态 |
| `tunnel status` | 查看 G-Tunnel 隧道状态 |
| `tunnel paths` | 列出所有传输路径 |
| `threat summary` | 威胁摘要 |
| `threat list` | 列出最近威胁事件 |
| `threat blacklist` | 查看当前黑名单 |
| `quota` | 配额与流量查询 |
| `config show` | 显示当前配置 |
| `config protocols` | 显示协议启用状态 |
| `config defense` | 显示防御策略参数 |
| `diag all` | 完整诊断报告 |
| `diag ebpf` | eBPF 程序诊断 |
| `diag conn` | 连接诊断 |
| `keygen` | 生成 Ed25519 密钥对 |
| `sign <challenge>` | 对挑战码进行硬件签名 |

---

## 详细用法

### version

```bash
mirage-cli version
```

输出版本号、构建时间、Git Commit、Go 版本、OS/Arch。

---

### status

```bash
mirage-cli status
mirage-cli status -g 192.168.1.100:9090
```

通过 Gateway 健康检查端点获取实时状态：
- 运行状态（online/degraded/emergency）
- eBPF 加载状态
- gRPC 上行/下行连接状态
- 威胁等级
- 活跃连接数
- 内存占用

---

### tunnel

#### tunnel status

```bash
mirage-cli tunnel status
```

显示 G-Tunnel 多路径隧道的完整状态：
- 各传输路径（QUIC/WSS/WebRTC/ICMP/DNS）的状态、RTT、丢包率
- FEC 前向纠错统计（冗余率、已恢复包数）
- 实时吞吐量（上行/下行）

#### tunnel paths

```bash
mirage-cli tunnel paths
```

以表格形式列出所有传输路径的详细信息：

```
Level  Type     Status     RTT        Loss     Endpoint
─────────────────────────────────────────────────────────────────
L0     quic     active     12ms       0.1%     gateway.example.com:443
L1     webrtc   standby    45ms       0.3%     via STUN
L2     wss      standby    38ms       0.0%     wss://gateway.example.com:443
L3     icmp     degraded   120ms      5.2%     198.51.100.1
L4     dns      dead       —          —        t.example.com
```

路径优先级：L0 QUIC → L1 WebRTC → L2 WSS → L3 ICMP/DNS

---

### threat

#### threat summary

```bash
mirage-cli threat summary
```

显示威胁态势摘要：
- 当前威胁等级
- 总事件数 / 24h 事件数
- 封禁 IP 数
- 主要威胁类型
- Cortex 感知中枢 / Phantom 欺骗引擎状态

#### threat list

```bash
mirage-cli threat list
mirage-cli threat list -n 50    # 显示最近 50 条
```

列出最近的威胁事件，包含时间、类型、来源 IP:Port、严重度、处置动作。

威胁类型：
- `ACTIVE_PROBING` — 主动探测
- `REPLAY_ATTACK` — 重放攻击
- `TIMING_ATTACK` — 时序攻击
- `DPI_DETECTION` — 深度包检测
- `JA4_SCAN` — JA4 指纹扫描
- `SNI_PROBE` — SNI 探测

#### threat blacklist

```bash
mirage-cli threat blacklist
```

输出当前黑名单（JSON 格式），包含 CIDR、过期时间、来源（local/global）。

---

### quota

```bash
mirage-cli quota
```

显示配额使用情况：

```
🟡 配额使用: 72.3%
  [██████████████████████░░░░░░░░] 7.2 GB / 10.0 GB
  剩余:       2.8 GB
  业务流量:   5.1 GB
  防御流量:   2.1 GB
  防御开销:   29.2%
  到期时间:   2026-05-20
```

字段说明：
- 业务流量：用户实际数据传输
- 防御流量：NPM Padding + VPC 噪声 + FEC 冗余等防御协议产生的额外流量
- 防御开销：防御流量 / 总流量，反映隐蔽性代价

---

### config

#### config show

```bash
mirage-cli config show
mirage-cli config show -c /path/to/gateway.yaml
```

显示完整的 Gateway 配置文件（YAML 格式化输出）。优先读取本地文件，失败则从 Gateway API 获取。

#### config protocols

```bash
mirage-cli config protocols
```

显示六大协议的启用状态：

```
协议状态:
  ✅ NPM          流量伪装 (XDP)
  ✅ B-DNA        行为识别 (TC)
  ✅ Jitter-Lite  时域扰动 (TC)
  ✅ VPC          噪声注入 (TC)
  ✅ G-Tunnel     多路径传输
  ❌ G-Switch     域名转生
```

#### config defense

```bash
mirage-cli config defense
```

显示当前防御策略参数（JSON 格式）：防御等级、Jitter 均值/标准差、噪声强度、Padding 率、B-DNA 模板 ID 等。

---

### diag

#### diag all

```bash
mirage-cli diag all
```

生成完整诊断报告：
- 系统信息（OS/Arch/Kernel）
- Gateway 连接状态
- eBPF 程序挂载状态（XDP/TC/Sockops/sk_msg）
- 网络接口状态

#### diag ebpf

```bash
mirage-cli diag ebpf
```

详细列出所有 eBPF 程序和 Map：

```
eBPF 程序:
名称                   类型       状态       执行次数
─────────────────────────────────────────────────────────
npm_xdp_main           xdp        attached   1284923
bdna_tc                tc         attached   892341
jitter_tc              tc         attached   892341
vpc_noise_tc           tc         attached   892341
phantom_tc             tc         attached   445123
chameleon_tc           tc         attached   445123
h3_shaper_tc           tc         attached   445123
sockmap_ops            sockops    attached   12893
sockmap_msg            sk_msg     attached   12893

eBPF Maps (43):
名称                     类型         Key    Val    MaxEntries
─────────────────────────────────────────────────────────────────
threat_events            ringbuf      0      0      1048576
traffic_stats            percpu_hash  4      16     65536
quota_map                array        4      8      1
...
```

#### diag conn

```bash
mirage-cli diag conn
```

连接诊断：测试 Gateway 健康检查延迟、gRPC 上下行状态。

---

### keygen

```bash
mirage-cli keygen
```

生成 Ed25519 密钥对：
- 私钥保存到 `~/.mirage/private.key`（权限 0600）
- 输出公钥 hex（需提交给 Mirage-OS 注册）
- 输出公钥指纹（前 16 字节 hex）

⚠️ 私钥一旦丢失无法恢复，请妥善备份。

---

### sign

```bash
mirage-cli sign "<challenge>"
```

对 Mirage-OS 下发的挑战码进行 Ed25519 签名。

**认证流程：**

1. 用户在 Web Console 发起登录
2. Mirage-OS 生成挑战码：`mirage-auth:<username>:<timestamp>:<random>`
3. 用户在本地执行 `mirage-cli sign "<challenge>"`
4. 将输出的签名 hex 粘贴到 Web Console
5. OS 用注册的公钥验签，通过即完成认证

**私钥加载优先级：**
1. Gateway Unix Socket (`/var/run/mirage-gateway.sock`) — 适用于 Gateway 同机部署
2. 本地文件 (`~/.mirage/private.key`) — 通用场景

---

## 典型使用场景

### 场景 1：日常监控

```bash
# 快速查看 Gateway 是否正常
mirage-cli status

# 检查隧道连通性
mirage-cli tunnel status

# 查看配额剩余
mirage-cli quota
```

### 场景 2：威胁排查

```bash
# 查看威胁态势
mirage-cli threat summary

# 列出最近事件
mirage-cli threat list -n 50

# 检查黑名单
mirage-cli threat blacklist
```

### 场景 3：故障诊断

```bash
# 完整诊断
mirage-cli diag all

# eBPF 是否正常挂载
mirage-cli diag ebpf

# 连接是否通畅
mirage-cli diag conn
```

### 场景 4：首次部署

```bash
# 1. 生成密钥对
mirage-cli keygen

# 2. 将公钥注册到 Mirage-OS
#    (通过 Web Console 或 API)

# 3. 验证 Gateway 连接
mirage-cli status -g <gateway_ip>:9090

# 4. 确认协议状态
mirage-cli config protocols
```

---

## 远程 Gateway 管理

默认连接本地 Gateway，通过 `-g` 参数可管理远程节点：

```bash
mirage-cli status -g 10.0.1.50:9090
mirage-cli tunnel status -g 10.0.1.50:9090
mirage-cli diag all -g 10.0.1.50:9090
```

⚠️ 健康检查端口（默认 9090）仅应暴露在内网，不要对公网开放。

---

## 退出码

| 退出码 | 含义 |
|--------|------|
| 0 | 成功 |
| 1 | 命令执行失败（参数错误、连接失败等） |
