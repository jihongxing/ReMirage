# 需求文档：Phase 4 — 生产加固与交付

## 简介

本阶段为 Mirage Project 四阶段实施的最终阶段，目标是将前三阶段的成果加固为可交付给首批 10-20 个高净值用户的生产级系统。核心工作包括：
1. **安全加固**：内存安全（memzero/mlock）、证书钉扎、反调试检测、tmpfs 全内存部署验证、优雅关闭内存擦除
2. **部署自动化**：Ansible playbook 一键部署 Gateway（编译 eBPF → 构建 Go → 配置 mTLS → 启动 systemd）、docker-compose 部署 OS、mTLS 证书链自动生成与分发
3. **性能验证**：Go benchmark 测试关键路径、bpftrace 脚本测量 eBPF 延迟、负载测试脚本、内存 profiling
4. **透明网关交付**：用户零侵入，仅需将流量路由到 Gateway IP，TPROXY + Sockmap 已实现

砍掉所有 SDK（Nginx 模块、Go SDK、C/Rust FFI、LD_PRELOAD、Python/Node wrapper）。砍掉盲发现协议（IPFS/区块链）、代码膨胀、ZK-SNARKs，这些是 V2 的事。

## 术语表

- **Gateway**：Mirage-Gateway 融合网关，Go 控制面 + C/eBPF 数据面，以透明代理模式运行
- **OS**：Mirage-OS 控制中心，包含 gateway-bridge（Go gRPC）+ api-server（NestJS）+ PostgreSQL + Redis
- **RAM_Shield**：内存保护模块（ram_shield.go 已有基础），负责 mlock 锁定、memzero 清零、swap 检测
- **Anti_Debug**：反调试检测模块，监控 /proc/self/status 中 TracerPid 和常见调试器进程
- **Certificate_Pinning**：证书钉扎机制，Gateway 钉扎 OS 证书指纹，防止中间人攻击
- **Root_CA**：自签名根证书颁发机构，存储于 OS 节点（Shamir 保护）
- **Gateway_Cert**：由 Root_CA 签发的 Gateway 节点证书，部署时自动生成
- **Ansible_Playbook**：Gateway 一键部署脚本，支持 Ubuntu 22.04 / Debian 12（内核 >= 5.15）
- **Benchmark_Suite**：性能基准测试套件，包含 Go benchmark、bpftrace 脚本、负载测试
- **tmpfs_Deploy**：Alpine Linux + 全内存运行部署模式，敏感数据不落盘
- **Emergency_Manager**：紧急自毁管理器（emergency.go 已实现），eBPF Map 原子清空 + 进程自杀
- **Graceful_Shutdown**：优雅关闭流程，逆序关闭模块并擦除所有敏感内存
- **Security_Checklist**：安全检查清单，验证所有安全加固项目通过
- **TPROXY**：透明代理模式，用户流量无需修改即可被 Gateway 拦截处理
- **Sockmap**：eBPF sockmap 程序，实现内核态流量重定向

## 需求

### 需求 1：内存安全加固

**用户故事：** 作为安全工程师，我需要所有敏感变量在使用后被安全擦除且不被交换到磁盘，以便防止内存取证攻击。

#### 验收标准

1. THE RAM_Shield SHALL 对所有加密密钥缓冲区调用 syscall.Mlock，防止操作系统将密钥页交换到磁盘
2. WHEN 加密密钥使用完毕后，THE RAM_Shield SHALL 使用逐字节覆写零值的方式清除密钥缓冲区，并触发 runtime.GC
3. THE RAM_Shield SHALL 提供 SecureAlloc 函数，分配 mlock 锁定的内存缓冲区用于存储敏感数据
4. THE RAM_Shield SHALL 提供 SecureWipe 函数，对指定内存区域执行逐字节清零后调用 Munlock 释放锁定
5. WHEN Gateway 进程启动时，THE RAM_Shield SHALL 调用 DisableCoreDump 禁用 core dump（设置 RLIMIT_CORE=0）
6. THE RAM_Shield SHALL 提供 CheckSwapUsage 函数，通过读取 /proc/self/status 中 VmSwap 字段检测是否有内存被交换到磁盘
7. IF VmSwap 值大于 0，THEN THE RAM_Shield SHALL 记录告警日志并尝试调用 mlockall 锁定所有当前和未来的内存页

### 需求 2：证书钉扎

**用户故事：** 作为安全工程师，我需要 Gateway 钉扎 OS 的证书指纹，以便防止中间人替换证书进行流量劫持。

#### 验收标准

1. THE Certificate_Pinning SHALL 在 Gateway 首次连接 OS 时，提取 OS 证书的 SHA-256 指纹并存储到内存中
2. WHEN Gateway 与 OS 建立后续 mTLS 连接时，THE Certificate_Pinning SHALL 验证 OS 证书的 SHA-256 指纹与钉扎值一致
3. IF OS 证书指纹与钉扎值不匹配，THEN THE Certificate_Pinning SHALL 拒绝连接并记录安全告警日志
4. THE Certificate_Pinning SHALL 支持通过配置文件预设 OS 证书指纹（用于首次部署场景）
5. WHEN OS 证书轮换时，THE Certificate_Pinning SHALL 支持通过安全通道（已认证的 gRPC 连接）更新钉扎指纹

### 需求 3：反调试检测

**用户故事：** 作为安全工程师，我需要 Gateway 检测是否被调试器附加，以便在被逆向分析时进入静默模式保护核心逻辑。

#### 验收标准

1. THE Anti_Debug SHALL 每 30 秒读取 /proc/self/status 文件，检查 TracerPid 字段是否不等于 0
2. THE Anti_Debug SHALL 检查 /proc 目录中是否存在 gdb、strace、ltrace、perf 等常见调试器进程
3. WHEN Anti_Debug 检测到调试器附加时，THE Gateway SHALL 进入静默模式：停止所有真实流量处理，仅发送伪造的背景噪声流量
4. THE Anti_Debug SHALL 提供 IsDebuggerPresent 函数，返回当前是否检测到调试器
5. WHEN Gateway 从静默模式恢复（调试器脱离）时，THE Gateway SHALL 在 10 秒内恢复正常流量处理

### 需求 4：tmpfs 部署验证

**用户故事：** 作为运维工程师，我需要验证 tmpfs 部署模式下所有敏感数据仅存在于内存中，以便确保物理查封时无数据残留。

#### 验收标准

1. THE tmpfs_Deploy SHALL 使用 Alpine Linux 基础镜像（Dockerfile.alpine 已存在），容器根文件系统设置为只读
2. THE tmpfs_Deploy SHALL 将 /var/mirage 和 /tmp 挂载为 tmpfs（内存文件系统），所有运行时数据写入 tmpfs
3. THE tmpfs_Deploy SHALL 设置 mem_swappiness=0，禁止容器内存被交换到磁盘
4. THE tmpfs_Deploy SHALL 验证 eBPF 程序对象文件（.o）从 tmpfs 加载，不从持久化存储读取
5. THE tmpfs_Deploy SHALL 验证 gateway.yaml 配置文件以只读方式挂载，运行时不产生任何磁盘写入

### 需求 5：优雅关闭与内存擦除

**用户故事：** 作为安全工程师，我需要 Gateway 在关闭时擦除所有敏感内存，以便进程退出后不留下可被取证的数据。

#### 验收标准

1. WHEN Gateway 收到 SIGINT 或 SIGTERM 信号时，THE Graceful_Shutdown SHALL 按逆序关闭所有模块（gRPC → 威胁编排 → 策略引擎 → eBPF → mTLS）
2. THE Graceful_Shutdown SHALL 在关闭过程中调用 RAM_Shield.SecureWipe 擦除所有已注册的敏感内存区域
3. THE Graceful_Shutdown SHALL 调用 Emergency_Manager.wipeSensitiveMaps 清空所有 eBPF Map 中的敏感数据
4. THE Graceful_Shutdown SHALL 在 30 秒超时内完成所有清理操作，超时后强制退出
5. WHEN Graceful_Shutdown 完成所有清理后，THE Gateway SHALL 以退出码 0 终止进程

### 需求 6：mTLS 证书链自动生成

**用户故事：** 作为运维工程师，我需要自动生成 mTLS 证书链，以便部署时无需手动管理证书。

#### 验收标准

1. THE Ansible_Playbook SHALL 包含证书生成任务：使用 openssl 生成自签名 Root_CA（RSA 4096 位，有效期 10 年）
2. THE Ansible_Playbook SHALL 为每个 Gateway 节点生成由 Root_CA 签发的证书（RSA 2048 位，有效期 1 年），证书 CN 包含节点 ID
3. THE Ansible_Playbook SHALL 将 Root_CA 证书分发到所有 Gateway 和 OS 节点
4. THE Ansible_Playbook SHALL 将 Gateway 证书和私钥部署到 /etc/mirage/certs/ 目录，权限设置为 600
5. IF Root_CA 已存在，THEN THE Ansible_Playbook SHALL 跳过 Root_CA 生成，仅生成 Gateway 节点证书
6. THE Ansible_Playbook SHALL 生成 OS 节点证书（由 Root_CA 签发），用于 gRPC 服务端 mTLS 认证

### 需求 7：Gateway Ansible 一键部署

**用户故事：** 作为运维工程师，我需要通过 Ansible playbook 一键部署 Gateway，以便快速扩展节点数量。

#### 验收标准

1. THE Ansible_Playbook SHALL 支持 Ubuntu 22.04 和 Debian 12 目标系统，内核版本 >= 5.15
2. THE Ansible_Playbook SHALL 自动安装编译依赖：clang、llvm、golang（>= 1.21）、libelf-dev、libbpf-dev
3. THE Ansible_Playbook SHALL 编译 eBPF 程序：使用 clang -O2 -target bpf 编译 bpf/ 目录下所有 .c 文件为 .o 文件
4. THE Ansible_Playbook SHALL 编译 Go 二进制：使用 CGO_ENABLED=1 go build 编译 mirage-gateway 可执行文件
5. THE Ansible_Playbook SHALL 生成 gateway.yaml 配置文件，包含节点 ID、网络接口、OS 端点地址等参数
6. THE Ansible_Playbook SHALL 部署 systemd 服务文件（mirage-gateway.service），配置自动重启和 eBPF 所需的 capabilities
7. THE Ansible_Playbook SHALL 启动 mirage-gateway 服务并验证服务状态为 active
8. THE Ansible_Playbook SHALL 支持升级场景：停止服务 → 备份旧二进制 → 部署新二进制 → 重启服务
9. IF 目标系统内核版本 < 5.15，THEN THE Ansible_Playbook SHALL 终止部署并输出错误信息

### 需求 8：OS docker-compose 部署

**用户故事：** 作为运维工程师，我需要通过 docker-compose 一键部署 Mirage-OS 控制中心，以便快速搭建管理平面。

#### 验收标准

1. THE OS docker-compose SHALL 包含四个服务：gateway-bridge（Go gRPC）、api-server（NestJS）、PostgreSQL 15、Redis 7
2. THE OS docker-compose SHALL 通过环境变量配置所有敏感参数（数据库密码、JWT 密钥、mTLS 证书路径）
3. THE OS docker-compose SHALL 配置 PostgreSQL 数据持久化卷和 Redis 数据持久化卷
4. THE OS docker-compose SHALL 配置服务依赖关系：gateway-bridge 和 api-server 依赖 PostgreSQL 和 Redis
5. THE OS docker-compose SHALL 包含健康检查配置，确保 PostgreSQL 和 Redis 就绪后再启动应用服务

### 需求 9：性能基准测试 — eBPF 延迟

**用户故事：** 作为性能工程师，我需要验证 eBPF 数据面延迟低于 1ms，以便确认用户流量处理不会引入可感知的延迟。

#### 验收标准

1. THE Benchmark_Suite SHALL 包含 bpftrace 脚本，测量 XDP 和 TC 程序的单次执行延迟（从进入到返回的纳秒数）
2. THE Benchmark_Suite SHALL 在 1000 次采样后输出 P50、P95、P99 延迟统计
3. THE Benchmark_Suite SHALL 验证 P99 延迟低于 1ms（1,000,000 纳秒）

### 需求 10：性能基准测试 — FEC 编码

**用户故事：** 作为性能工程师，我需要验证 FEC 编码延迟低于 1ms，以便确认前向纠错不会成为传输瓶颈。

#### 验收标准

1. THE Benchmark_Suite SHALL 包含 Go benchmark 测试，测量 FEC 编码和解码的单次执行延迟
2. THE Benchmark_Suite SHALL 使用不同数据包大小（64B、512B、1500B、9000B）进行测试
3. THE Benchmark_Suite SHALL 验证所有数据包大小的 FEC 编码 P99 延迟低于 1ms

### 需求 11：性能基准测试 — G-Switch 转生

**用户故事：** 作为性能工程师，我需要验证 G-Switch 域名转生在 5 秒内完成，以便确认域名切换对用户透明。

#### 验收标准

1. THE Benchmark_Suite SHALL 包含 Go benchmark 测试，测量从收到转生指令到新域名生效的端到端延迟
2. THE Benchmark_Suite SHALL 验证转生延迟低于 5 秒

### 需求 12：性能基准测试 — 资源占用

**用户故事：** 作为性能工程师，我需要验证 Gateway 的 CPU 和内存占用在目标范围内，以便确认系统不会过度消耗宿主机资源。

#### 验收标准

1. THE Benchmark_Suite SHALL 包含负载测试脚本，模拟 N 个并发 Gateway 连接（N=10、50、100）
2. THE Benchmark_Suite SHALL 使用 runtime.MemStats 采集内存使用数据，验证稳态内存占用低于 200MB
3. THE Benchmark_Suite SHALL 使用 CPU profiling 采集 CPU 使用数据，验证稳态 CPU 占用低于 20%
4. THE Benchmark_Suite SHALL 输出资源占用报告，包含内存分配量、GC 暂停时间、Goroutine 数量

### 需求 13：安全检查清单验证

**用户故事：** 作为安全工程师，我需要一个自动化的安全检查脚本，以便验证所有安全加固项目均已正确实施。

#### 验收标准

1. THE Security_Checklist SHALL 验证所有加密密钥缓冲区已注册到 RAM_Shield 并调用了 Mlock
2. THE Security_Checklist SHALL 验证 mTLS 在生产配置中为强制启用状态
3. THE Security_Checklist SHALL 验证证书钉扎功能已激活
4. THE Security_Checklist SHALL 验证反调试检测循环已启动
5. THE Security_Checklist SHALL 验证 tmpfs 部署模式下无磁盘写入（检查 /proc/self/io 中 write_bytes）
6. THE Security_Checklist SHALL 验证紧急自毁功能可正常触发（调用 Emergency_Manager.TriggerWipe 后所有敏感 Map 被清空）
7. THE Security_Checklist SHALL 验证优雅关闭后所有敏感内存区域已被清零
