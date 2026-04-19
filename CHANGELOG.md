# Changelog

## [0.9.0] - 2026-04-20

### 核心架构
- 完成二元架构（C 数据面 + Go 控制面）全链路实现
- eBPF Loader 支持 XDP/TC/Sockmap 三种挂载模式
- Go ↔ C 通信通过 eBPF Map + Ring Buffer 实现

### 协议实现
- NPM 流量伪装协议（XDP 层零拷贝 Padding）
- B-DNA 行为识别协议（TCP/QUIC/TLS 指纹重写）
- Jitter-Lite 时域扰动协议（纳秒级 IAT 控制）
- VPC 噪声注入协议（物理噪音模拟）
- G-Tunnel 多路径传输（QUIC H3 + FEC + CID 轮换）
- G-Switch 域名转生（M.C.C. 信令 + Raft 一致性）

### Mirage-Gateway
- 18 阶段启动流程
- Cortex 威胁感知中枢
- Phantom 影子欺骗引擎
- 策略引擎（B-DNA 模板、成本计算、心跳监控）
- 安全加固（mTLS、反调试、RAM 防护、证书钉扎）
- TPROXY 透明代理 + Splice 零拷贝
- 计费引擎（流量统计 + 配额熔断）

### Mirage-OS
- Raft 3 节点集群
- 地理围栏 + 热密钥管理
- NestJS API Server
- React Web Console
- 12 个微服务（计费、蜂窝、域名、智能、影子等）
- Monero 支付集成
- WebSocket 实时推送

### Phantom Client
- Windows TUN 适配（Wintun）
- G-Tunnel QUIC 客户端
- Kill Switch
- 内存安全防护
- Token 引导连接

### 部署
- Ansible Playbook（Gateway + OS + Certs）
- Kubernetes 生产清单（DaemonSet + HPA + PDB）
- Docker Compose 开发环境
- 证书生成脚本（Root CA + Gateway + OS）
- 混沌测试脚本

### SDK
- 10 种语言 SDK（Go/Python/JS/Rust/Java/Kotlin/Swift/C#/PHP/Ruby）
- 多语言文档（中/英/日/俄/西/印）

### 测试
- 集成测试（生命周期、转生、自毁、Raft 故障转移）
- 性能基准（FEC、G-Switch、资源占用）
- eBPF 延迟测量（bpftrace）
- 高压测试脚本

---

## [0.9.1] - 2026-04-20

### 验证
- Linux 编译验证通过（OpenCloudOS 9, kernel 6.6.119, clang 17, Go 1.24.2）
- eBPF 数据面：7 个 .o 全部编译成功（bdna/chameleon/h3_shaper/jitter/npm/phantom/sockmap）
- Go 控制面：bin/mirage-gateway 二进制生成，零错误
- Linux 运行验证通过：18 阶段全量启动，零降级，零警告
- eBPF 全量挂载：XDP 1 个 + TC 8 个 + Sockops 1 个 + sk_msg 1 个，共享 Map 13 个，总 Map 43 个
- Sockmap 零拷贝路径已激活
- Dead Man's Switch 自毁序列验证通过（心跳超时 → 0xDEADBEEF → Map 清空 → 内存擦除 → 进程退出）

### eBPF Verifier 修复
- jitter.c: 内联 emergency_wipe 消除跨程序 BPF 调用
- npm.c: 合并双 XDP 为 npm_xdp_main 单入口（Single-Tenancy Rule）
- bdna.c: Bulletproof Scanner — 逐字节 skb 游走，零栈变量偏移访问
- bdna.c: Pointer Refresh Pattern — store_bytes 后刷新所有包指针
- bdna.c: bpf_l4_csum_replace 替代手动校验和计算
- bdna.c/chameleon.c: 位掩码黄金法则 (doff_len &= 0x3C) 强制定界
- phantom.c/npm.c: ip_hlen &= 0x3C 位掩码定界
- loader.go: sk_msg 改用 RawAttachProgram 挂载到 Map FD

---

## [Unreleased]

### 待完成
- FEC AVX-512 C 加速对接
- Phantom Client Wintun 实际读写
- Mirage-OS 威胁检测实现（DDoS、异常流量）
- Monero 实时汇率 API 对接
- 单元测试覆盖率提升至 80%+
