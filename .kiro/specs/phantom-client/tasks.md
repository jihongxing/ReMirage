# 任务清单：Phase 5 — Phantom Client 用户端安全隧道

- [x] 1. 项目脚手架与依赖
  - [x] 1.1 创建 `phantom-client/` 目录结构（cmd/phantom、pkg/token、pkg/memsafe、pkg/tun、pkg/gtclient、pkg/killswitch、assets、embed）、`go.mod`（module phantom-client, go 1.21+）、`Makefile`
  - [x] 1.2 添加核心依赖：`golang.org/x/crypto`（ChaCha20-Poly1305、X25519、Ed25519）、`github.com/klauspost/reedsolomon`（纯 Go FEC）、`github.com/quic-go/quic-go`（QUIC 传输）、`golang.org/x/sys`（syscall.Mlock、ioctl）、`pgregory.net/rapid`（属性测试）
  - [x] 1.3 创建 `build.sh`：三平台交叉编译脚本（CGO_ENABLED=0、-ldflags 注入版本/时间/commit、可选代码签名步骤）
  - [x] 1.4 准备 `embed/wintun.dll` 占位文件 + `assets/app.ico` 占位图标 + `assets/winres.json` Version Info 配置

- [x] 2. 维度三：内存态鉴权与配置
  - [x] 2.1 创建 `pkg/memsafe/buffer.go`：SecureBuffer 结构体、NewSecureBuffer（mlock 锁定，失败降级告警）、Write、Read、Wipe（逐字节零覆盖 + munlock）、全局 registry + WipeAll
  - [x] 2.2 创建 `pkg/memsafe/buffer_test.go`：Property 3（Wipe 后全零验证）属性测试 + mlock 降级单元测试
  - [x] 2.3 创建 `pkg/token/token.go`：BootstrapConfig 结构体、GatewayEndpoint 结构体、ParseToken（Base64 解码 → ChaCha20-Poly1305 解密 → JSON 反序列化 → SecureBuffer 锁定）、TokenToBase64（序列化 → 加密 → Base64 编码）、统一错误 "Invalid token"
  - [x] 2.4 创建 `pkg/token/token_test.go`：Property 1（往返一致性）、Property 2（无效 Token 统一拒绝）属性测试 + 过期 Token 单元测试

- [x] 3. 维度一：核心基建
  - [x] 3.1 创建 `pkg/tun/tun.go`：TUNDevice 接口定义（Read/Write/Name/MTU/Close）、CreateTUN 工厂函数（runtime.GOOS 分发）、CleanupStale
  - [x] 3.2 创建 `pkg/tun/tun_linux.go`：LinuxTUNDevice 实现（open /dev/net/tun → ioctl TUNSETIFF IFF_TUN|IFF_NO_PI → ip link set up → ip addr add）
  - [x] 3.3 创建 `pkg/tun/tun_darwin.go`：UtunDevice 实现（PF_SYSTEM socket → SYSPROTO_CONTROL connect → getsockopt 获取 utun 名称 → ifconfig 配置 IP/MTU）
  - [x] 3.4 创建 `pkg/tun/tun_windows.go`：WintunDevice 实现（//go:embed wintun.dll → os.MkdirTemp 释放 → windows.LoadDLL 加载 → WintunCreateAdapter → WintunStartSession → defer 清理链：CloseAdapter → FreeLibrary → 删文件/目录）
  - [x] 3.5 创建 `pkg/tun/tun_test.go`：各平台 mock 测试 + CleanupStale 单元测试

- [x] 4. 维度四：G-Tunnel 客户端栈
  - [x] 4.1 创建 `pkg/gtclient/fec.go`：FECCodec 结构体（基于 klauspost/reedsolomon，8 数据 + 4 校验）、NewFECCodec、Encode（数据→分片+校验）、Decode（分片→原始数据，支持最多 4 个丢失恢复）
  - [x] 4.2 创建 `pkg/gtclient/sampler.go`：OverlapSampler 结构体（ChunkSize=400、OverlapSize=100）、Fragment 结构体、Split（重叠采样分片）、Reassemble（按 SeqNum 排序 + XOR 校验重组）
  - [x] 4.3 创建 `pkg/gtclient/client.go`：GTunnelClient 结构体、RouteTable 结构体、NewGTunnelClient、ProbeAndConnect（并发探测 3 节点，context 超时控制）、Send（分片→FEC→ChaCha20 加密→QUIC 发送）、Receive（QUIC 接收→解密→FEC 解码→重组）、PullRouteTable（通过隧道拉取节点列表到内存）、Reconnect（< 5s 从 RouteTable 选下一节点）、OnGatewaySwitch 回调、Close
  - [x] 4.4 创建 `pkg/gtclient/gtclient_test.go`：Property 4（FEC 纠错能力）、Property 5（FEC 往返一致性）、Property 6（分片重组一致性）属性测试 + 边界条件单元测试（空数据、单字节、64KB 最大包）

- [x] 5. 维度二：路由暴力劫持
  - [x] 5.1 创建 `pkg/killswitch/killswitch.go`：KillSwitch 结构体、Platform 接口（GetDefaultGateway/DeleteDefaultRoute/AddDefaultRoute/AddHostRoute/DeleteHostRoute/RestoreDefaultRoute）、NewKillSwitch、Activate（4 步序列：备份→删默认→TUN 默认→/32 明细）、UpdateGatewayRoute（先加新再删旧，原子无空窗）、Deactivate（恢复原始路由）、IsActivated
  - [x] 5.2 创建 `pkg/killswitch/route_linux.go`：Linux Platform 实现（ip route del default / ip route add default dev / ip route add host via）
  - [x] 5.3 创建 `pkg/killswitch/route_darwin.go`：macOS Platform 实现（route delete default / route add default -interface / route add -host）
  - [x] 5.4 创建 `pkg/killswitch/route_windows.go`：Windows Platform 实现（route delete 0.0.0.0 / route add 0.0.0.0 mask / route add host）
  - [x] 5.5 创建 `pkg/killswitch/killswitch_test.go`：Property 8（路由原子性）属性测试 + 激活/解除序列 mock 单元测试

- [x] 6. 主程序集成
  - [x] 6.1 创建 `cmd/phantom/main.go`：命令行解析（-token 参数或 stdin 输入）、启动序列（Token→MemSafe→CleanupStale→TUN→ProbeAndConnect→KillSwitch.Activate→PullRouteTable→双向转发 goroutine）、信号处理（SIGINT/SIGTERM/Ctrl+C）、优雅关闭（逆序：KillSwitch.Deactivate→Client.Close→TUN.Close→WipeAll，30s 超时）
  - [x] 6.2 实现双向转发循环：forwardTUNToTunnel（TUN.Read → GTunnelClient.Send）、forwardTunnelToTUN（GTunnelClient.Receive → TUN.Write）、错误时触发 Reconnect 而非终止
  - [x] 6.3 实现 Gateway 切换监听：OnGatewaySwitch 回调 → KillSwitch.UpdateGatewayRoute 原子更新 /32 路由
  - [x] 6.4 实现最小化状态输出：连接状态（Connected/Reconnecting）、当前节点 Region（不输出 IP）、运行时长、上下行流量字节数

- [x] 7. 维度五：反取证与伪装
  - [x] 7.1 配置 `assets/winres.json`：Version Info 资源（伪装 CompanyName/FileDescription/ProductName/LegalCopyright）+ 图标引用
  - [x] 7.2 集成 go-winres 到 build.sh：Windows 构建前执行 `go-winres make` 生成 rsrc_windows_amd64.syso，嵌入 Version Info 和图标
  - [x] 7.3 完善 build.sh 签名流程：Windows signtool 签名（环境变量 SIGN_CERT/SIGN_PASS）、macOS codesign 签名（环境变量 APPLE_IDENTITY）、签名失败告警但不终止构建
  - [x] 7.4 配置进程名伪装：构建产物重命名为伪装名称（如 enterprise-sync.exe / enterprise-sync），-ldflags 注入伪装版本信息
