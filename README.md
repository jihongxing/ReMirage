# Mirage Project

下一代网络隐身基础设施。内核级流量隐匿 + 分布式控制中心 + 自愈客户端。

eBPF 数据面实现零拷贝包处理（< 1ms 延迟），Go 控制面编排六维协议协同防御，构建一套在物理网络层"不可见、杀不死"的隐形通信系统。

---

## 这是什么

Mirage 是一套完整的网络通信隐身系统。它解决的核心问题是：**如何在高度审查的网络环境中，让通信流量完全不可识别、不可阻断、不可追踪。**

传统代理工具（VPN、Shadowsocks、V2Ray 等）的根本缺陷在于——它们只解决了"加密"问题，却没有解决"隐身"问题。审查者不需要解密你的内容，只需要通过流量特征（包长分布、时间间隔、TLS 指纹、协议行为）判断"这不是正常流量"，然后一刀切断。

Mirage 的设计哲学完全不同：**不是让流量"看起来加密"，而是让流量"看起来根本不存在"。**

---

## 它能做什么

### 让你的流量在物理层面消失

- **流量伪装**：在网卡驱动层（XDP）直接修改数据包形态，让每个包的长度分布与正常 HTTPS 浏览完全一致
- **指纹拟态**：重写 TCP/TLS/QUIC 握手指纹，让审查设备的 DPI 引擎认为你是 Chrome 浏览器在看 YouTube
- **时间隐身**：在内核时间戳层面重塑数据包的发送间隔，消除机器通信特有的规律性节奏
- **背景噪声**：模拟物理光缆的自然抖动和噪声，让统计分析模型无法区分真实流量与背景噪音

### 让你的连接杀不死

- **多路径生存**：同时维持 5 种传输协议（QUIC/WebRTC/WebSocket/ICMP/DNS），任何一种被封锁都能在毫秒内切换到下一种
- **域名转生**：当域名被污染时，系统自动切换到预热的备用域名，用户无感知
- **信令共振**：当所有 IP 都被封锁时，客户端通过 DNS over HTTPS、GitHub Gist、Mastodon 等去中心化通道自动发现新的入口节点，实现"焦土复活"
- **自愈重连**：三级降级策略（路由表 → 引导池 → 信令共振），确保在最极端的网络封锁下仍能恢复连接

### 让你的痕迹无法追溯

- **零磁盘痕迹**：所有敏感数据运行在内存文件系统中，断电即物理消失
- **自毁机制**：心跳超时后自动执行密钥擦除 + eBPF Map 清空 + 进程退出，不留任何可取证的残留
- **匿名计费**：Monero 加密货币支付，无法关联用户身份与支付记录
- **反取证**：禁用 core dump、内存锁定（mlock）、反调试检测、符号混淆编译

---

## 适用场景

### 高价值通信保护

为需要绝对通信安全的场景提供基础设施级保护。适用于在高审查网络环境中需要可靠、隐蔽通信通道的用户群体。

### 对抗深度包检测（DPI）

当面对国家级 DPI 设备（如基于机器学习的流量分类器）时，Mirage 在内核层面重塑流量的统计特征，使其在数学上与正常流量不可区分。这不是简单的协议伪装，而是从包长、时间间隔、指纹、噪声四个维度同时进行物理层面的流量变形。

### 对抗主动探测

审查者会主动连接可疑服务器来验证其是否为代理。Mirage 的应对：
- 蜜罐迷宫：对探测者展示一个完整的伪装网站
- 影子流量：生成与真实用户行为一致的诱饵流量
- 静默拒绝：mTLS 握手失败时不返回任何错误信息，对探测者表现为"端口无响应"

### 对抗 IP 封锁

当 Gateway 的 IP 被加入黑名单时：
1. 客户端自动切换到备用 Gateway（毫秒级）
2. 如果所有已知 IP 都被封锁，触发信令共振发现机制
3. 通过加密的去中心化通道获取新的 Gateway 地址
4. 整个过程对用户透明，无需手动干预

### 对抗协议封锁

当特定协议被封锁时（如 UDP 被全面阻断）：
1. G-Tunnel Orchestrator 检测到 QUIC 通道失效
2. 自动降级到 WebRTC（伪装为视频会议）或 WSS（伪装为网页浏览）
3. 极端情况下降级到 ICMP（伪装为 Ping）或 DNS Tunnel
4. 网络恢复后自动升格回高性能通道

---

## 与传统方案的区别

| 维度 | 传统代理 | Mirage |
|------|---------|--------|
| 工作层级 | 用户态应用层 | 内核态 eBPF (XDP/TC) |
| 延迟开销 | 5-50ms | < 1ms |
| 流量特征 | 可被 DPI 识别 | 物理层面不可区分 |
| 指纹伪装 | 无或简单模拟 | 内核态 TCP/TLS/QUIC 全栈重写 |
| 抗封锁 | 单协议，封即死 | 5 种协议自动降级 |
| IP 被封 | 需手动换节点 | 信令共振自动复活 |
| 取证痕迹 | 配置文件/日志残留 | 内存运行 + 自毁机制 |
| 计费隐私 | 信用卡/支付宝可追踪 | Monero 匿名支付 |
| 架构 | 单点代理 | 分布式蜂窝 + Raft 集群 |

---

## 系统架构

```
                    ┌─────────────────────────────────────────────┐
                    │              Mirage-OS 控制中心               │
                    │                                             │
                    │  ┌─────────┐ ┌──────────┐ ┌─────────────┐  │
                    │  │  Raft   │ │ Billing  │ │  Web Console │  │
                    │  │ 3-Node  │ │  Monero  │ │   React+WS  │  │
                    │  └────┬────┘ └────┬─────┘ └──────┬──────┘  │
                    │       └───────────┼──────────────┘          │
                    │            Gateway Bridge (Go)               │
                    │     gRPC Server · Quota · Intel · Raft FSM  │
                    └──────────────────┬──────────────────────────┘
                                       │ gRPC / mTLS (双向认证)
          ┌────────────────────────────┼────────────────────────────┐
          │                            │                            │
┌─────────▼──────────┐      ┌─────────▼──────────┐      ┌─────────▼──────────┐
│   Gateway Alpha    │      │   Gateway Bravo    │      │   Gateway N ...    │
│                    │      │    (温储备)         │      │                    │
│ ┌────────────────┐ │      └────────────────────┘      └────────────────────┘
│ │  Go 控制面     │ │
│ │                │ │
│ │ Strategy Engine│ │      ┌─────────────────────────────────────────────┐
│ │ G-Tunnel Orch. │ │      │  六维协议协同矩阵                            │
│ │ G-Switch M.C.C.│ │      │                                             │
│ │ Cortex Threat  │ │      │  空间维: NPM 流量伪装 (XDP 零拷贝 Padding)   │
│ │ Phantom Decoy  │ │      │  指纹维: B-DNA 行为拟态 (TCP/QUIC/TLS 重写)  │
│ │ gRPC Client    │ │      │  时间维: Jitter-Lite 时域扰动 (纳秒级 IAT)   │
│ └───────┬────────┘ │      │  背景维: VPC 噪声注入 (光缆抖动模拟)         │
│         │eBPF Map  │      │  存活维: G-Tunnel 多路径 (QUIC+WSS+WebRTC)  │
│         │Ring Buf  │      │  转生维: G-Switch 域名转生 (M.C.C. 信令)     │
│ ┌───────▼────────┐ │      └─────────────────────────────────────────────┘
│ │  C 数据面      │ │
│ │  eBPF XDP/TC   │ │
│ │                │ │
│ │ npm.o  bdna.o  │ │
│ │ jitter.o vpc.o │ │
│ │ phantom.o      │ │
│ │ sockmap.o      │ │
│ │ h3_shaper.o    │ │
│ └────────────────┘ │
└─────────┬──────────┘
          │ QUIC H3 / mTLS / FEC
          │ (多路径: QUIC → WSS → WebRTC → ICMP → DNS)
┌─────────▼──────────────────────────────────────────────┐
│                  Phantom Client                         │
│                                                        │
│  TUN 设备 · G-Tunnel QUIC · FEC 纠错 · Kill Switch    │
│  信令共振发现器 · 内存安全防护 · 证书钉扎              │
└────────────────────────────────────────────────────────┘
```

---

## 核心能力

### eBPF 数据面（C · 内核态）

| 程序 | Hook 点 | 能力 |
|------|---------|------|
| `npm.o` | XDP | 零拷贝流量填充，随机 Padding 注入，包长分布拟态 |
| `bdna.o` | TC | TCP Window/TTL/Options 重写，JA4 指纹伪装 |
| `jitter.o` | TC | skb->tstamp 纳秒级控制，IAT 分布重塑 |
| `chameleon.o` | TC | QUIC/TLS ClientHello 指纹变形 |
| `phantom.o` | TC | 蜜罐诱导 + 影子流量生成 |
| `h3_shaper.o` | TC | HTTP/3 流量整形，BBR v3 辅助 |
| `sockmap.o` | Sockops/sk_msg | 内核态零拷贝转发，绕过用户态 |

**性能指标**：延迟增加 < 1ms · CPU < 5% · 内存 < 50MB · 零拷贝率 > 95%

### Go 控制面

| 模块 | 职责 |
|------|------|
| `pkg/strategy/` | 策略引擎：防御等级联动、B-DNA 模板切换、成本计算 |
| `pkg/gtunnel/` | G-Tunnel 多路径：Orchestrator 自适应调度、FEC 纠错、CID 轮换 |
| `pkg/gswitch/` | G-Switch 域名转生：M.C.C. 信令、Raft 一致性、信令加密 |
| `pkg/cortex/` | Cortex 威胁感知：行为分析、上下文持久化、威胁总线 |
| `pkg/phantom/` | Phantom 影子引擎：蜜罐迷宫、LLM 幽灵、自毁序列 |
| `pkg/threat/` | 威胁编排：事件聚合、响应联动、黑名单同步 |
| `pkg/security/` | 安全加固：mTLS、Ed25519 影子认证、RAM Shield、反调试 |
| `pkg/ebpf/` | eBPF 管理：Loader、Monitor、Ring Buffer、计费引擎 |
| `pkg/api/` | gRPC 通信：心跳上报、策略下发、配额熔断 |
| `pkg/health/` | 健康检测：应用模拟、回声逃逸、反馈环路 |

### G-Tunnel 多路径传输

五级传输协议降级矩阵，任何单一协议被封锁均可自动切换：

```
L0: QUIC H3 (UDP 443)     ← 主通道，最高性能
L1: WebRTC DataChannel     ← 伪装为视频会议
L2: WSS (TCP 443)          ← 伪装为 WebSocket
L3: ICMP Tunnel            ← 伪装为 Ping 诊断
L4: DNS Tunnel             ← 最后手段，极低带宽
```

- HappyEyeballs 并发探测，Epoch Barrier 双发选收
- FEC 前向纠错（Reed-Solomon 8+4），丢包 33% 仍可恢复
- CID 轮换 + Overlap Sampling 抗流量关联

### 信令共振（Resonance）

当所有 Gateway IP 被封锁时，客户端通过去中心化通道"重组复活"：

```
OS 发布加密信令 ──→ ┌─ DNS TXT (Cloudflare DoH)
                    ├─ GitHub Gist (伪装监控数据)
                    └─ Mastodon Toot (去中心化社交)

Client 并发竞速 ──→ First-Win-Cancels-All
                    → Base64 解码 → X25519 ECDH 解密
                    → Ed25519 验签 → 反重放校验
                    → 获取新 Gateway IP → 满血复活
```

### Mirage-OS 控制中心

| 组件 | 技术栈 | 职责 |
|------|--------|------|
| Gateway Bridge | Go | gRPC 双向通信、配额熔断、黑名单分发、Raft FSM |
| API Server | NestJS + Prisma | 用户/蜂窝/计费/域名/威胁 CRUD |
| Web Console | React + TailwindCSS | 实时仪表盘、WebSocket 推送 |
| WS Gateway | Go | WebSocket 实时事件广播 |
| Billing | Go + Monero RPC | XMR 充值确认、汇率转换、配额分配 |
| Raft Cluster | hashicorp/raft | 3 节点一致性、配额/黑名单/策略复制 |

### Phantom Client

| 能力 | 实现 |
|------|------|
| TUN 隧道 | Windows (Wintun) + Linux |
| G-Tunnel QUIC | quic-go + mTLS 证书钉扎 |
| FEC 纠错 | Reed-Solomon 编解码 + Overlap 重组 |
| Kill Switch | 系统路由劫持，隧道断开时阻断所有流量 |
| 信令共振发现 | DoH + Gist + Mastodon 三通道竞速 |
| 内存安全 | mlock + SecureBuffer + 退出时擦除 |
| 三级重连 | RouteTable → Bootstrap Pool → 信令共振 |

---

## 安全机制

| 层级 | 机制 |
|------|------|
| 传输层 | mTLS 双向认证 + 证书钉扎 (SHA-256 leaf pin) |
| 认证层 | Ed25519 挑战-响应 + 硬件指纹绑定 |
| 信令层 | Sign-then-Encrypt (Ed25519 + X25519 ECDH + ChaCha20-Poly1305) |
| 内存层 | RAM Shield (mlock + 禁用 core dump + swap 检测) |
| 反调试 | ptrace 检测 + /proc/self/status 监控 |
| 自毁 | Dead Man's Switch (心跳超时 → eBPF Map 清空 → 密钥擦除 → 进程退出) |
| 反逆向 | garble 混淆编译 (抹除所有符号/路径) |
| 物理隔离 | tmpfs 内存文件系统 + 只读根 + 断电即消失 |

---

## 系统要求

| 组件 | 要求 |
|------|------|
| 内核 | Linux ≥ 5.15（生产）/ ≥ 4.19（降级模式） |
| 编译器 | clang ≥ 14 (eBPF target) |
| Go | ≥ 1.24 |
| Node.js | ≥ 18 (API Server) |
| Docker | Docker Engine + Compose v2 |
| 数据库 | PostgreSQL 15 + Redis 7 |

---

## 构建

### Gateway（需要 Linux 环境）

```bash
cd mirage-gateway

# 完整构建（eBPF + Go）
make all

# 仅编译 eBPF 数据面
make bpf

# 混淆编译（生产部署，抹除所有符号）
make go-obfuscated

# 验证 eBPF 程序
make verify
```

### Mirage-OS

```bash
cd mirage-os

# 开发环境（PostgreSQL + Redis + 全部服务）
docker compose up -d

# 生产 Raft 集群
docker compose -f docker-compose.raft.yml up -d
```

### Phantom Client

```bash
cd phantom-client

# Linux
make build

# Windows (交叉编译)
GOOS=windows GOARCH=amd64 go build -o phantom.exe ./cmd/phantom
```

### CLI 工具

```bash
cd mirage-cli
go build -o mirage ./
```

---

## 部署

### 快速开发环境

```bash
# 启动 OS 全栈
cd mirage-os && docker compose up -d

# 启动 Gateway（需要 root + eBPF 支持）
sudo ./bin/mirage-gateway -iface eth0 -defense 20

# 启动客户端
./phantom --token <bootstrap_token>
```

### 生产部署

```bash
# Ansible 一键部署
cd deploy/ansible
ansible-playbook -i inventory/hosts.yml playbook.yml

# 或使用 Kubernetes
kubectl apply -f mirage-gateway/deployments/production_ready_manifest.yaml
```

### 高安全 Gateway（无磁盘痕迹）

```bash
cd mirage-gateway
docker compose -f docker-compose.tmpfs.yml up -d
# 特性：只读根 + tmpfs 内存挂载 + 断电即消失
```

详细部署指南见 [DEPLOYMENT.md](DEPLOYMENT.md)。

---

## 混沌测试

创世演习（Genesis Drill）— 三幕极端压力测试：

```bash
cd deploy/chaos/genesis

# 启动隔离实验室（Docker 网络命名空间）
./run.sh up

# 执行完整三幕演习
./run.sh drill

# 单独执行
./run.sh act1    # 创世流转：商业闭环验证
./run.sh act2    # 协议绞杀：iptables 切断 UDP/TCP，验证多路径降级
./run.sh act3    # 焦土与复活：杀死 Gateway，验证信令共振绝境复活

# 销毁
./run.sh down
```

其他混沌测试：

```bash
cd deploy/chaos
./chaos_test.sh raft_failover     # Raft Leader 强杀故障转移
./chaos_test.sh domain_block      # G-Switch 域名封锁模拟
./chaos_test.sh heartbeat_death   # 心跳超时自毁验证
./chaos_test.sh quota_cutoff      # 配额熔断精准掐断
```

---

## 项目结构

```
mirage-gateway/          # 融合网关
├── bpf/                 #   C 数据面（7 个 eBPF 程序）
├── cmd/gateway/         #   主程序入口（18 阶段启动）
├── pkg/                 #   Go 控制面（15+ 子包）
├── configs/             #   配置文件
└── deployments/         #   systemd / K8s 清单

mirage-os/               # 控制中心
├── gateway-bridge/      #   Go gRPC 服务（配额/黑名单/Raft）
├── api-server/          #   NestJS REST API
├── web/                 #   React Web Console
├── services/            #   12 个微服务
├── pkg/                 #   共享库（auth/crypto/raft/geo）
└── scripts/             #   测试/部署脚本

phantom-client/          # 终端客户端
├── pkg/gtclient/        #   G-Tunnel QUIC 客户端 + FEC
├── pkg/resonance/       #   信令共振发现器
├── pkg/killswitch/      #   Kill Switch
├── pkg/memsafe/         #   内存安全防护
├── pkg/token/           #   Bootstrap Token 解析
└── pkg/tun/             #   TUN 设备适配

mirage-cli/              # 运维 CLI
├── cmd/                 #   子命令（status/tunnel/threat/quota/sign）
└── main.go

sdk/                     # 多语言 SDK（10 种语言）
deploy/                  # 部署基础设施
├── ansible/             #   Ansible Playbook
├── certs/               #   证书生成脚本
├── chaos/               #   混沌测试（含 Genesis Drill）
└── scripts/             #   运维脚本

benchmarks/              # 性能基准
docs/                    # 项目文档（架构/协议/实施指南）
tests/                   # 集成测试
```

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 内核数据面 | C · eBPF (XDP/TC/Sockops/sk_msg) · clang BPF target |
| 控制面 | Go 1.24 · cilium/ebpf · quic-go · hashicorp/raft |
| 传输层 | QUIC H3 · WebSocket · WebRTC (pion) · ICMP · DNS |
| 加密 | ChaCha20-Poly1305 · X25519 ECDH · Ed25519 · HKDF-SHA256 |
| 后端 | NestJS · Prisma · PostgreSQL · Redis |
| 前端 | React · TypeScript · TailwindCSS · Vite |
| 部署 | Docker · Kubernetes · Ansible · systemd |
| 测试 | Property-Based Testing (rapid) · 混沌工程 · bpftrace |

---

## 验证状态

- ✅ eBPF 数据面：7 个 .o 全部编译通过（OpenCloudOS 9, kernel 6.6.119, clang 17）
- ✅ Go 控制面：零错误编译，bin/mirage-gateway 二进制生成
- ✅ Linux 运行验证：18 阶段全量启动，零降级，零警告
- ✅ eBPF 全量挂载：XDP 1 + TC 8 + Sockops 1 + sk_msg 1，共享 Map 13，总 Map 43
- ✅ Dead Man's Switch 自毁序列验证通过
- ✅ Phantom Client FEC 往返一致性（1-4000 字节，100 次属性测试）
- ✅ 信令共振发现器 6 项测试通过（DoH/Gist/Mastodon + First-Win 竞速）

---

## SDK

支持 10 种语言，6 种文档语言：

| SDK | 文档 |
|-----|------|
| Go · Python · JavaScript · Rust · Java | 中文 · English · 日本語 |
| Kotlin · Swift · C# · PHP · Ruby | Русский · Español · हिन्दी |

---

## 许可证

**严格专有软件** — 禁止任何形式的使用、复制、借鉴或分发。详见 [LICENSE](LICENSE)。
