# eBPF 覆盖图

> **Status**: 草案
> **Source of Truth**: `mirage-gateway/pkg/ebpf/loader.go`
> **Role**: eBPF 覆盖图 — 标定数据面参与边界与用户态处理路径

---

## 1. 文档目的

本文档以 `loader.go` 的 `LoadAndAttach()` 为唯一真相源，完整列出：

1. **运行态实际挂载**的 eBPF 程序（内核数据面）
2. **源码定义但未被 loader 挂载**的 SEC 函数（预留 / 未启用）
3. **纯用户态处理路径**（Go 控制面，不经过 eBPF）

目标是回答一个核心问题：**eBPF 在 Mirage 数据面中参与了多深？哪些路径仍在用户态？**

---

## 2. 三层分类说明

| 层级 | 含义 | 判定依据 |
|------|------|----------|
| 🟢 **运行态挂载** | loader.go `LoadAndAttach()` 中有对应 attach 调用 | 代码可追溯到 `attachXDP` / `attachTCFilter` / `attachSockops` / `attachSkMsg` |
| 🟡 **源码存在未挂载** | `.c` 文件中有 `SEC()` 定义，但 loader 未调用 | 源码 grep 可见 SEC 宏，loader 无对应 attach |
| 🔴 **纯用户态** | 完全在 Go 层实现，无 eBPF 参与 | 实现位于 `pkg/` 目录，无内核态交互 |

---

## 3. 运行态挂载程序清单

以下为 `loader.go` `LoadAndAttach()` 中实际挂载的全部 eBPF 程序。

### 3.1 Map 母体：jitter.o（首个加载）

| 程序 | 文件 | 挂载类型 | 方向 | 协议层 | 用户态协同 | loader 函数 |
|------|------|----------|------|--------|-----------|-------------|
| `jitter_lite_egress` | jitter.c | TC | egress | L3 | 独立完成 | `loadMasterProgram` |
| `vpc_ingress_detect` | jitter.c | TC | ingress | L3-L4 | Ring Buffer 上报 | `loadMasterProgram`（降级） |

### 3.2 后续程序（MapReplacements 共享母体 Map）

| 程序 | 文件 | 挂载类型 | 方向 | 协议层 | 用户态协同 | loader 函数 | critical |
|------|------|----------|------|--------|-----------|-------------|----------|
| `npm_xdp_main` | npm.c | XDP | ingress | L3 | 独立完成 | `attachNPM` | ✅ |
| `bdna_tcp_rewrite` | bdna.c | TC | egress | L4 | 独立完成 | `attachBDNA` | ✅ |
| `bdna_quic_rewrite` | bdna.c | TC | egress | L4 | `skb->mark` 协同 | `attachBDNA`（降级） | — |
| `bdna_ja4_capture` | bdna.c | TC | ingress | L4-L7 | Ring Buffer 上报 | `attachBDNA`（降级） | — |
| `phantom_redirect` | phantom.c | TC | ingress | L3 | — | `attachPhantom` | ❌ |
| `chameleon_tls_rewrite` | chameleon.c | TC | egress | L4-L7 | `skb->mark` 协同 | `attachChameleon` | ❌ |
| `chameleon_quic_rewrite` | chameleon.c | TC | egress | L4 | `skb->mark` 协同 | `attachChameleon`（降级） | — |
| `h3_shaper_egress` | h3_shaper.c | TC | egress | L4-L7 | — | `attachH3Shaper` | ❌ |
| `icmp_tunnel_egress` | icmp_tunnel.c | TC | egress | L3 | — | `attachICMPTunnel` | ❌ |
| `icmp_tunnel_ingress` | icmp_tunnel.c | TC | ingress | L3 | — | `attachICMPTunnel` | ❌ |
| `l1_silent_egress` | l1_silent.c | TC | egress | L3-L4 | 独立完成 | `attachL1Silent` | ❌ |

### 3.3 独立加载：sockmap.o（不共享 common.h Map）

| 程序 | 文件 | 挂载类型 | 方向 | 协议层 | 用户态协同 | loader 函数 |
|------|------|----------|------|--------|-----------|-------------|
| `sockmap_sockops` | sockmap.c | sockops (cgroup) | — | L4 | 独立完成 | `loadSockmap` |
| `sockmap_redirect` | sockmap.c | sk_msg | — | L4 | 独立完成 | `loadSockmap` |


---

## 4. 源码定义但未挂载的 SEC 函数

以下函数在 `.c` 源码中有 `SEC()` 宏定义，但 `loader.go` 中无对应 attach 调用。属于预留能力或尚未启用的路径。

| 函数 | 文件 | SEC 类型 | 状态说明 |
|------|------|----------|----------|
| `bdna_tls_rewrite` | bdna.c | TC | TLS ClientHello 重写逻辑存在，但 loader 未挂载；当前 TLS 重写由用户态 uTLS 完成，eBPF 仅做 `skb->mark` 标记 |
| `jitter_lite_adaptive` | jitter.c | TC | 自适应时域扰动，源码存在，loader 未挂载 |
| `jitter_lite_physical` | jitter.c | TC | VPC 综合物理噪声模拟，源码存在，loader 未挂载 |
| `jitter_lite_social` | jitter.c | TC | 社交行为模拟扰动，源码存在，loader 未挂载 |
| `emergency_wipe` | jitter.c | TC | 紧急擦除，源码存在，loader 未挂载 |
| `heartbeat_check` | jitter.c | TC | 心跳检测，源码存在，loader 未挂载 |
| `vpc_content_injection` | chameleon.c | TC | VPC 内容注入，源码存在，loader 未挂载 |
| `npm_decoy_marker` | npm.c | TC | 诱饵包标记，源码存在，loader 未挂载 |
| `npm_mtu_probe` | npm.c | XDP | MTU 探测，源码存在，loader 未挂载 |

> **注意**: 这些函数的存在不代表 eBPF 覆盖了对应能力。覆盖图仅以 loader 运行态行为为准。

---

## 5. 用户态处理路径

以下路径完全在 Go 用户态实现，不经过 eBPF 内核数据面。

| 路径 | 实现位置 | 语言 | 说明 |
|------|----------|------|------|
| G-Tunnel 分片与重组 | `mirage-gateway/pkg/gtunnel/` | Go | 多路径传输的分片、重组、路径调度 |
| FEC 编解码 | `mirage-gateway/pkg/gtunnel/` | Go | 前向纠错编解码 |
| QUIC 握手与传输参数协商 | quic-go 库 | Go | QUIC 协议栈完整握手 |
| TLS ClientHello 完整重写 | uTLS 库 | Go | eBPF 仅做 `skb->mark=0x544C5348` 标记，实际重写在用户态 |
| HTTP/2 SETTINGS 与请求序列 | net/http2 | Go | 应用层协议协商与请求编排 |
| G-Switch 域名转生 | `mirage-gateway/pkg/gswitch/` | Go | 域名轮换、API 调用、Raft 一致性 |
| B-DNA 画像 registry 管理 | `mirage-gateway/pkg/ebpf/bdna_profile_updater.go` | Go | 指纹模板管理与下发（通过 eBPF Map） |

---

## 6. eBPF 参与 vs 用户态处理路径对照表

| 功能维度 | eBPF 内核态 | Go 用户态 | 协同方式 |
|----------|------------|-----------|----------|
| **包长填充 (NPM)** | `npm_xdp_main` XDP 层 `bpf_xdp_adjust_tail` | — | eBPF 独立完成 |
| **TCP 指纹拟态 (B-DNA)** | `bdna_tcp_rewrite` 修改 SYN Window/MSS/WScale | — | eBPF 独立完成 |
| **QUIC 指纹标记** | `bdna_quic_rewrite` 设置 `skb->mark=0x51554943` | quic-go 读取 mark 协同 | eBPF 标记 + 用户态执行 |
| **TLS 指纹标记** | `chameleon_tls_rewrite` 设置 `skb->mark=0x544C5348` | uTLS 完整重写 ClientHello | eBPF 标记 + 用户态执行 |
| **JA4 指纹捕获** | `bdna_ja4_capture` Ring Buffer 上报 | Go 读取 Ring Buffer 分析 | eBPF 采集 + 用户态消费 |
| **时域扰动 (Jitter)** | `jitter_lite_egress` 修改 `skb->tstamp` | — | eBPF 独立完成 |
| **入口威胁检测 (VPC)** | `vpc_ingress_detect` Ring Buffer 上报 | Go 读取事件处理 | eBPF 检测 + 用户态响应 |
| **协议变色龙** | `chameleon_tls_rewrite` / `chameleon_quic_rewrite` | — | eBPF 标记 + 用户态协同 |
| **H3 流量整形** | `h3_shaper_egress` TC egress | — | eBPF 独立完成 |
| **ICMP 隧道** | `icmp_tunnel_egress` / `icmp_tunnel_ingress` | — | eBPF 独立完成 |
| **L1 静默响应** | `l1_silent_egress` TC egress | — | eBPF 独立完成 |
| **Sockmap 零拷贝** | `sockmap_sockops` + `sockmap_redirect` | — | eBPF 独立完成 |
| **多路径传输 (G-Tunnel)** | — | `pkg/gtunnel/` 分片/重组/FEC | 纯用户态 |
| **域名转生 (G-Switch)** | — | `pkg/gswitch/` API/Raft | 纯用户态 |
| **QUIC 握手** | — | quic-go 库 | 纯用户态 |
| **TLS ClientHello 重写** | — | uTLS 库 | 纯用户态（eBPF 仅标记） |


---

## 7. 参与度定性结论

> **eBPF 深度参与关键路径，但非全链路零拷贝。**

### 7.1 eBPF 深度参与的路径

- **L3 包长伪装**: NPM XDP 层零拷贝填充，完全内核态
- **L4 TCP 指纹拟态**: B-DNA TC 层直接改写 SYN 包头字段，完全内核态
- **L3 时域扰动**: Jitter TC 层精确控制 `skb->tstamp`，纳秒级精度
- **L4 零拷贝转发**: Sockmap `sockops` + `sk_msg` 实现内核态 socket 重定向
- **L3-L4 入口检测**: VPC ingress 检测 + L1 静默响应，内核态闭环
- **L3 ICMP 隧道**: ICMP 封装/解封装完全内核态

### 7.2 eBPF 标记 + 用户态执行的路径

- **TLS ClientHello 重写**: eBPF 仅设置 `skb->mark`，实际重写由 uTLS 在用户态完成
- **QUIC 指纹协同**: eBPF 标记后由 quic-go 在用户态配合
- **JA4 指纹分析**: eBPF 采集原始数据到 Ring Buffer，Go 消费分析

### 7.3 纯用户态路径（无 eBPF 参与）

- **G-Tunnel 多路径传输**: 分片、重组、FEC、路径调度全部 Go 实现
- **G-Switch 域名转生**: API 调用、DNS 管理、Raft 一致性全部 Go 实现
- **QUIC/TLS 完整握手**: 依赖 quic-go / uTLS 库，应用层协议协商

### 7.4 覆盖率统计

| 分类 | 程序数 | 占比 |
|------|--------|------|
| 🟢 运行态挂载 eBPF 程序 | 15 | — |
| 🟡 源码存在未挂载 SEC 函数 | 9 | — |
| 🔴 纯用户态关键路径 | 7 | — |

> loader.go 加载策略：jitter.o 作为 Map 母体首个加载创建所有共享 Map，后续 .o 通过 `MapReplacements` 纯内存动态链接共享 Map 实例。sockmap.o 完全独立加载。进程退出时所有 Map 随 FD 关闭自动销毁，无 bpffs 残留。

---

## 8. 性能证据（待采集）

性能数据需在 Linux + 对应工具环境下实际采集，当前为占位。

### 8.1 采集方法

| 指标 | 工具 | 命令 | 对照基准 |
|------|------|------|----------|
| XDP/TC 延迟 (P50/P95/P99) | bpftrace | `sudo bpftrace benchmarks/ebpf_latency.bt` | C 数据面延迟 < 1ms |
| eBPF 程序 CPU 占用 | perf | `sudo perf top -e cycles:k` | C 数据面 CPU < 5% |
| eBPF Map 内存占用 | bpftool | `sudo bpftool map show` | C 数据面内存 < 50MB |
| 零拷贝率 | XDP 统计 | `ethtool -S <iface> \| grep xdp` | 零拷贝率 > 95% |

### 8.2 产出路径

| 报告 | 路径 | 状态 |
|------|------|------|
| 延迟报告 | `artifacts/ebpf-perf/latency-report.txt` | 待采集 |
| CPU 报告 | `artifacts/ebpf-perf/cpu-report.txt` | 待采集 |
| 内存报告 | `artifacts/ebpf-perf/memory-report.txt` | 待采集 |

### 8.3 采集前置条件

- Linux 内核 ≥ 5.15
- clang（eBPF 编译）
- bpftrace（延迟采集）
- perf / mpstat（CPU 采集）
- bpftool（Map 内存采集）
- root 或 CAP_BPF + CAP_NET_ADMIN 权限

---

## 9. 原始数据路径

| 数据类型 | 路径 | 说明 |
|----------|------|------|
| eBPF 延迟采集脚本 | `benchmarks/ebpf_latency.bt` | bpftrace 脚本，测量 XDP/TC 处理延迟 |
| 延迟报告 | `artifacts/ebpf-perf/latency-report.txt` | 待采集 |
| CPU 报告 | `artifacts/ebpf-perf/cpu-report.txt` | 待采集 |
| 内存报告 | `artifacts/ebpf-perf/memory-report.txt` | 待采集 |
| loader 源码 | `mirage-gateway/pkg/ebpf/loader.go` | 本文档唯一真相源 |
| 设计文档 | `.kiro/specs/phase2-stealth-evidence/design.md` | M7 eBPF 覆盖图设计 |
