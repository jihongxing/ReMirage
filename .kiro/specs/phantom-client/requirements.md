# 需求文档：Phase 5 — Phantom Client 用户端安全隧道

## 简介

本阶段为 Mirage Project 的第五阶段，目标是构建部署在用户宿主机上的"最后一公里"安全管道——Phantom Client。它在用户终端与 Mirage-Gateway 之间建立一条抗审查的 G-Tunnel 加密隧道，完成 Mirage 架构从服务端到客户端的完整闭环。

设计哲学：**哑管道（Dumb Pipe）**。客户端不承担计费、智能路由选路、威胁分析等业务逻辑，只做三件事：
1. 创建 TUN 虚拟网卡，接管宿主机全部 IP 流量
2. 从 TUN 读取 IP 包，封装成 G-Tunnel 发往 Gateway
3. 从 G-Tunnel 收到响应包，写回 TUN

核心安全原则：
- **配置不落地**：硬盘上永远只有一个通用二进制，零用户特征
- **Fail-Closed**：客户端崩溃 → 宿主机彻底断网，防止流量泄漏
- **行为白名单化**：对 OS 和 EDR 表现为合法的网络应用程序
- **拔盘即失效**：所有敏感数据仅存在于进程内存，断电/取证无法恢复

技术选型：纯 Go 静态编译，`CGO_ENABLED=0`，交叉编译 Windows/macOS/Linux 单文件可执行程序。不涉及 eBPF/XDP/TC 内核态操作（全部在 Gateway 侧）。

## 术语表

- **Phantom_Client**：用户端隧道客户端，本文档的实现目标
- **Gateway**：Mirage-Gateway，服务端融合网关，G-Tunnel 的对端
- **Mirage_OS**：全局控制中心，负责用户认证、节点分配、配置下发
- **G-Tunnel**：Mirage 自研多路径传输协议，基于 QUIC/H3，支持重叠采样、FEC 纠错
- **TUN_Device**：操作系统虚拟网络接口，工作在 L3（IP 层），用于接管宿主机流量
- **Wintun**：Windows 平台 TUN 驱动（微软签名），WireGuard 同款，通过 `//go:embed` 内嵌
- **utun**：macOS 内置 TUN 接口（utun0, utun1...），需要 root 权限
- **Bootstrap_Token**：一次性启动令牌，Base64 编码的加密配置种子，包含 Bootstrap 节点池
- **Bootstrap_Pool**：Token 内嵌的 3 个不同管辖区备用 Gateway 入口 IP:Port
- **Kill_Switch**：防泄漏熔断机制，通过路由表暴力劫持确保 Fail-Closed
- **FEC_Codec**：前向纠错编解码器（Forward Error Correction），Reed-Solomon 编码，客户端必须对称实现
- **Overlap_Sampler**：重叠采样分片器，G-Tunnel 核心算法，客户端必须对称实现
- **Secure_Buffer**：安全内存缓冲区，mlock 锁定 + 使用后零覆盖，防止内存取证
- **Route_Table**：内存动态路由表，通过 G-Tunnel 从 Mirage-OS 拉取，包含上百个可用 Gateway 节点
- **EDR**：端点检测与响应（Endpoint Detection and Response），企业安全软件
- **Version_Info**：Windows PE 文件的版本信息资源（FileDescription、CompanyName、ProductName 等）
- **Code_Signing**：代码签名，使用合法证书对二进制进行数字签名

---

## 维度一：核心基建 (Core Infrastructure)

### 需求 1：单文件编译与跨平台构建

**用户故事：** 作为运维人员，我需要为 Windows/macOS/Linux 三个平台各生成一个无依赖的单文件可执行程序，以便用户下载即用，无需安装运行时环境。

#### 验收标准

1. THE Phantom_Client SHALL 使用 `CGO_ENABLED=0` 纯 Go 静态编译，生成无外部依赖的单文件可执行程序
2. THE 构建脚本 SHALL 支持三平台交叉编译：`GOOS=windows/darwin/linux` × `GOARCH=amd64,arm64`
3. THE Windows 构建产物 SHALL 为单一 phantom.exe，通过 `//go:embed` 将微软签名的 wintun.dll 打包在内
4. THE 构建脚本 SHALL 使用 `-ldflags="-s -w"` 剥离调试符号，并通过 `-ldflags` 注入版本号、构建时间、Git Commit
5. THE 构建产物大小 SHALL 控制在 15MB 以内（不含 Wintun DLL 的平台）、25MB 以内（含 Wintun DLL 的 Windows 平台）

### 需求 2：跨平台 TUN 虚拟网卡管理

**用户故事：** 作为 Phantom_Client，我需要在 Windows/macOS/Linux 上创建和管理 TUN 虚拟网卡，以便透明接管宿主机的全部 IP 流量。

#### 验收标准

1. WHEN Phantom_Client 在 Windows 上启动时，SHALL 从 `//go:embed` 资源中释放 wintun.dll 到 `os.TempDir()` 下的随机子目录，通过 `windows.LoadDLL()` 加载，并创建 TUN 适配器
2. WHEN Phantom_Client 在 macOS 上启动时，SHALL 通过 PF_SYSTEM socket + SYSPROTO_CONTROL 系统调用创建 utun 接口，需要 root 权限（sudo）
3. WHEN Phantom_Client 在 Linux 上启动时，SHALL 通过 `/dev/net/tun` + ioctl TUNSETIFF 创建 TUN 接口，需要 `CAP_NET_ADMIN` 权限
4. WHEN TUN 接口创建成功后，SHALL 为 TUN 接口分配内部 IP 地址（10.7.0.2/24），并设置 MTU 为 G-Tunnel 协商的最优值（默认 1400）
5. WHEN Phantom_Client 正常退出时，SHALL 销毁 TUN 接口、卸载 Wintun DLL（Windows）、删除释放的临时文件和目录
6. WHEN Phantom_Client 非正常退出后再次启动时，SHALL 扫描并清理上次残留的临时文件（Windows Wintun DLL 残留目录）
7. IF TUN 接口创建失败（权限不足或驱动缺失），THEN SHALL 输出明确的错误信息（包含所需权限说明）并以非零退出码终止

### 需求 3：主程序生命周期管理

**用户故事：** 作为用户，我希望通过简单的命令行启动客户端，客户端自动完成所有初始化并进入隧道模式，退出时安全清理所有状态。

#### 验收标准

1. WHEN 用户执行 `phantom -token <base64>` 或启动后通过 stdin 输入 Token 时，SHALL 按以下顺序初始化：Token 解析 → 内存安全初始化 → 残留清理 → TUN 创建 → Bootstrap 节点探测 → G-Tunnel 建立 → Kill_Switch 激活 → 动态路由表拉取 → 进入稳态双向转发
2. IF Token 解析、TUN 创建或 Kill_Switch 激活失败（关键步骤），THEN SHALL 立即擦除已加载的敏感内存并以非零退出码终止
3. IF Bootstrap 节点全部不可达（非关键步骤），THEN SHALL 进入指数退避重试循环（初始 2 秒，最大 120 秒），而非立即终止
4. WHEN 收到 SIGINT/SIGTERM（Linux/macOS）或 Ctrl+C（Windows）时，SHALL 按逆序关闭：Kill_Switch 解除（恢复路由） → G-Tunnel 断开 → TUN 销毁 → 临时文件清理 → 内存擦除 → 退出码 0
5. THE 优雅关闭 SHALL 在 30 秒超时内完成，超时后强制退出
6. WHILE 处于稳态运行时，SHALL 在终端输出最小化状态信息（连接状态、当前节点 Region、运行时长），不输出敏感信息（IP、密钥、Token）
7. THE 整个客户端 SHALL 编译为单一可执行文件，Windows 上为 phantom.exe（内嵌 wintun.dll），macOS/Linux 上为 phantom

---

## 维度二：路由暴力劫持 (Routing & Kill Switch)

### 需求 4：Fail-Closed Kill Switch

**用户故事：** 作为用户，我希望在客户端运行期间，宿主机的所有流量都必须经过加密隧道，即使客户端崩溃也不会泄漏明文流量。

#### 验收标准

1. WHEN G-Tunnel 成功建立后，SHALL 执行以下路由表操作序列：备份当前默认网关路由（IP + 接口） → 删除系统原有默认网关路由 → 添加 TUN 接口为新的默认路由（0.0.0.0/0） → 添加一条 /32 明细路由直达当前 Gateway IP（通过原始物理网关出站）
2. WHEN 路由表劫持生效后，宿主机上任何非 G-Tunnel 的出站流量 SHALL 要么进入 TUN 被加密，要么因无路由被操作系统丢弃
3. WHEN Phantom_Client 正常退出时，SHALL 恢复备份的原始默认网关路由，宿主机网络恢复正常
4. WHEN Phantom_Client 异常崩溃时，由于原始默认路由已被删除，宿主机 SHALL 彻底断网（Fail-Closed），用户重启电脑即可恢复原始路由
5. IF 路由表操作失败（权限不足），THEN SHALL 输出告警并拒绝启动隧道，防止在无 Kill_Switch 保护下运行

### 需求 5：Gateway IP 路由原子更新

**用户故事：** 作为 Phantom_Client，我需要在 Gateway IP 发生变更（G-Switch 域名转生）时原子性地更新路由，以便不产生路由空窗期导致流量泄漏。

#### 验收标准

1. WHEN Gateway IP 发生变更时，SHALL 先添加新 Gateway IP 的 /32 明细路由，再删除旧 Gateway IP 的 /32 明细路由，确保任意时刻至少存在一条有效的 Gateway 直连路由
2. THE 路由更新操作 SHALL 在 1 秒内完成
3. THE Kill_Switch SHALL 在 Windows（route add/delete）、macOS（route 命令）、Linux（ip route）三个平台上实现等效的路由操作行为

---

## 维度三：内存态鉴权与配置 (In-Memory Config)

### 需求 6：一次性 Bootstrap Token 认证

**用户故事：** 作为用户，我希望通过一次性 Token 启动客户端，无需在磁盘上存储任何配置文件，以便实现配置不落地的防取证能力。

#### 验收标准

1. WHEN 用户通过命令行参数（`-token <base64>`）或标准输入传递 Bootstrap_Token 时，SHALL 解析并解密 Token 获取 Bootstrap 配置
2. THE Bootstrap_Token SHALL 包含以下加密字段：Bootstrap_Pool（3 个不同管辖区的 Gateway IP:Port）、用户认证凭证（Ed25519 密钥对或会话密钥）、G-Tunnel 加密参数（预共享密钥或证书指纹）、UserID（匿名标识）、ExpiresAt（过期时间）
3. WHEN Token 解密成功后，SHALL 将配置结构体保存在 Go 进程内存中，使用 Secure_Buffer 锁定包含敏感数据的内存页（mlock）
4. WHEN Phantom_Client 退出时（正常或崩溃），SHALL 通过 Secure_Buffer 将敏感内存页用零覆盖后释放
5. IF Token 格式无效或解密失败，THEN SHALL 输出 "Invalid token" 错误并以非零退出码终止，不泄露具体失败原因（解密算法、字段缺失等）
6. THE Phantom_Client 的二进制文件本身 SHALL 不包含任何用户特定的配置信息、硬编码 IP 或密钥
7. WHEN Token 被客户端成功消费并建立 G-Tunnel 连接后，SHALL 通知 Mirage_OS 作废该 Token（防止重放攻击）
8. SHALL NOT 在操作系统注册表（Windows）、LaunchServices（macOS）或任何持久化存储中注册 URI Scheme 或协议处理器

### 需求 7：安全内存管理

**用户故事：** 作为安全工程师，我需要所有敏感数据（密钥、Token、节点列表）在内存中受保护且使用后被安全擦除，以便防止内存转储取证。

#### 验收标准

1. THE Secure_Buffer SHALL 使用 syscall.Mlock 锁定敏感数据内存页，防止操作系统将其交换到磁盘
2. WHEN Secure_Buffer.Wipe 被调用时，SHALL 使用逐字节覆写零值的方式清除缓冲区内容
3. THE Phantom_Client SHALL 提供 WipeAll 全局函数，在退出时擦除所有已注册的 Secure_Buffer 实例
4. IF Mlock 调用失败（权限不足或平台不支持），THEN SHALL 记录告警日志并继续运行（降级模式，不锁定内存但仍执行零覆盖擦除）

### 需求 8：Bootstrap 节点池与故障转移

**用户故事：** 作为 Phantom_Client，我需要从 Bootstrap_Pool 中自动发现可用的 Gateway 节点，并在连接失败时自动切换，以便在 IP 封锁场景下保持连通性。

#### 验收标准

1. WHEN Phantom_Client 启动并解析 Token 后，SHALL 并发探测 Bootstrap_Pool 中的 3 个 Gateway IP，选择首个成功响应的节点建立 G-Tunnel
2. IF 所有 3 个 Bootstrap 节点均不可达，THEN SHALL 使用指数退避策略（初始 2 秒，最大 120 秒）循环重试
3. WHEN G-Tunnel 成功建立后，SHALL 通过加密隧道从 Mirage_OS 拉取完整的动态 Route_Table（包含上百个可用节点），存储在进程内存中
4. WHEN 当前 Gateway 连接断开或检测到封锁时，SHALL 在 5 秒内从内存中的 Route_Table 选择下一个可用节点自动重连
5. THE Route_Table SHALL 仅存在于进程内存中，永远不写入磁盘、注册表或任何持久化存储
6. WHEN Mirage_OS 通过 G-Tunnel 推送路由表更新时，SHALL 在内存中热更新节点列表，无需重启客户端

---

## 维度四：G-Tunnel 客户端栈 (Data Plane)

### 需求 9：G-Tunnel 对称传输层

**用户故事：** 作为 Phantom_Client，我需要实现 G-Tunnel 协议的客户端侧传输层，包括 FEC 编解码和重叠采样，以便与 Gateway 建立完整的加密隧道。

#### 验收标准

1. THE Phantom_Client SHALL 实现 G-Tunnel 协议的对称传输层，包括：数据分片与重组、重叠采样编解码（Overlap_Sampler）、FEC（Reed-Solomon）编码与解码（FEC_Codec）
2. WHEN 从 TUN 接口读取到 IP 包时，SHALL 按 G-Tunnel 协议进行重叠采样分片 → FEC 编码 → ChaCha20-Poly1305 加密 → 通过 QUIC/UDP 发送到 Gateway
3. WHEN 从 Gateway 收到 G-Tunnel 数据包时，SHALL 解密 → FEC 解码（如有丢失分片则纠错恢复） → 重叠采样重组为完整 IP 包 → 写回 TUN 接口
4. THE FEC_Codec SHALL 采用纯 Go 软件实现（基于 klauspost/reedsolomon 库，不依赖 AVX-512 指令集），默认配置 8 数据分片 + 4 校验分片
5. THE FEC_Codec SHALL 在最多 4 个分片丢失（parityShards 个）的场景下实现零数据丢失恢复
6. THE Overlap_Sampler SHALL 使用默认参数：ChunkSize=400 字节、OverlapSize=100 字节（25% 重叠率）

### 需求 10：QUIC 传输与连接管理

**用户故事：** 作为 Phantom_Client，我需要通过 QUIC 协议与 Gateway 通信，以便利用 0-RTT 连接恢复和 UDP 穿透能力。

#### 验收标准

1. THE Phantom_Client SHALL 使用 quic-go 库建立到 Gateway 的 QUIC 连接，目标端口 UDP 443
2. THE QUIC 连接 SHALL 支持 0-RTT 连接恢复，减少重连延迟
3. THE 传输层加密 SHALL 使用 ChaCha20-Poly1305（客户端可能无 AES-NI 硬件加速），密钥通过 X25519 密钥交换协商
4. WHEN Gateway 协商启用多路径传输时，SHALL 支持通过多条 UDP 路径并发发送分片
5. THE QUIC 连接 SHALL 配置 MaxIdleTimeout=30s，超时后自动触发重连

### 需求 11：双向转发引擎

**用户故事：** 作为 Phantom_Client，我需要在 TUN 接口和 G-Tunnel 之间高效地双向转发 IP 包。

#### 验收标准

1. THE 转发引擎 SHALL 启动两个独立的 goroutine：TUN→Tunnel（出站）和 Tunnel→TUN（入站）
2. WHEN TUN→Tunnel goroutine 从 TUN 读取到 IP 包时，SHALL 调用 GTunnelClient.Send 发送，单包处理延迟 SHALL 低于 5ms
3. WHEN Tunnel→TUN goroutine 从 GTunnelClient.Receive 收到 IP 包时，SHALL 写回 TUN 接口
4. IF 任一方向的转发出现错误（TUN 读写失败或 G-Tunnel 断连），SHALL 触发重连流程而非直接终止进程

---

## 维度五：反取证与伪装 (Anti-Forensics)

### 需求 12：二进制元数据伪装

**用户故事：** 作为安全工程师，我需要 Phantom_Client 的二进制文件在文件属性和进程列表中表现为合法的商业软件，以便不引起安全审计人员的注意。

#### 验收标准

1. THE Windows 构建产物 SHALL 嵌入 Version_Info 资源（通过 go-winres 或 rsrc 工具），包含伪装的 FileDescription、CompanyName、ProductName、LegalCopyright 字段，模拟企业 OA 辅助工具
2. THE Windows 构建产物 SHALL 嵌入自定义图标（.ico），外观为通用的企业办公软件图标
3. THE macOS 构建产物 SHALL 在 Info.plist（如打包为 .app）中设置伪装的 CFBundleName 和 CFBundleIdentifier
4. THE 进程名 SHALL 在所有平台上显示为伪装名称（如 "enterprise-sync" 或类似的无害名称），而非 "phantom"

### 需求 13：代码签名集成

**用户故事：** 作为安全工程师，我需要构建流程自动化地对二进制进行代码签名，以便在 Windows SmartScreen 和 macOS Gatekeeper 中获得信任。

#### 验收标准

1. THE 构建脚本 SHALL 集成 Windows 代码签名步骤（signtool），支持通过环境变量传入证书路径和密码
2. THE 构建脚本 SHALL 集成 macOS 代码签名步骤（codesign），支持通过环境变量传入 Developer ID
3. THE 构建脚本 SHALL 在签名步骤失败时输出告警但不终止构建（签名为可选步骤，开发环境可跳过）
4. THE 签名后的二进制 SHALL 通过 `signtool verify`（Windows）或 `codesign --verify`（macOS）验证通过

### 需求 14：运行时行为白名单化

**用户故事：** 作为用户，我希望 Phantom_Client 在宿主机上的运行时行为与合法商业软件一致，不触发 EDR 行为检测告警。

#### 验收标准

1. THE 网络行为 SHALL 表现为通过 UDP 443 端口与远端服务器通信（标准 QUIC 流量特征），不产生高频端口扫描
2. SHALL NOT 使用 UPX 压缩、代码混淆器、进程隐藏、Reflective DLL Injection 等会触发 EDR 行为检测的技术
3. WHEN 在 Windows 上释放 Wintun DLL 时，SHALL 释放到标准临时目录（os.TempDir），且 DLL 本身带有微软数字签名，加载方式为标准的 windows.LoadDLL
4. SHALL NOT 产生异常 DNS 查询、注册表修改（除 TUN 网卡驱动安装所必需的）、系统服务注册等可疑行为
5. SHALL NOT 修改系统 hosts 文件、安装根证书、或注入其他进程
