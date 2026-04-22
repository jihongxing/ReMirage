---
Status: input
Target Truth: docs/governance/boundaries/runtime-truth-boundaries.md (交付标准)
Migration: 交付技术规范，待拆分到部署资产和运行时真相边界
---

Mirage Project 交付技术规范
方案一：Nginx 流量拟态模块 (mod_mirage)
1. 交付形态
交付一个名为 ngx_mirage_module.so 的动态链接库，以及配套的加密配置文件 mirage.conf。

2. 部署逻辑
用户无需重新编译 Nginx，只需在现有 Nginx 配置文件的顶层添加：

```nginx
load_module modules/ngx_mirage_module.so;

stream {
    mirage_gateway_zone 64m; # 分配内存池处理加密上下文
    
    server {
        listen 443;
        mirage_protect on;          # 开启内核级流量转换
        mirage_template "YouTube";  # 应用 Jitter-Lite 的 YouTube 拟态模板
        mirage_tunnel_path "/api/v1/stream"; # 混淆信令路径
        proxy_pass backend_cluster;
    }
}
```

3. 技术原理
内核接管：模块加载后，利用 setsockopt 开启 IP_TRANSPARENT，配合 eBPF 将 Nginx 的出站流量直接注入 G-Tunnel。

业务掩护：外部观察者看到的依然是标准的 Nginx 进程在处理 HTTPS 请求，所有指纹（JA4/L）与 Web 服务器保持一致。

方案二：嵌入式应用 SDK (mirage-core-sdk)
1. 交付形态
针对不同语言的二进制库文件（如 libmirage.a, mirage.h）或语言特定的包管理器私有仓库（如 go get mirage-sdk）。

2. 接入逻辑（以 Go 为例）
用户仅需在主入口文件（main.go）的第一行引入并初始化：

```go
import "github.com/mirage-project/sdk-go"

func main() {
    // 启动即进入 Mirage 闭环，接管底层 runtime 网络栈
    mirage.Init(&mirage.Config{
        LicenseKey: "User_Unique_ID",
        Mimicry:    mirage.MIMIC_ZOOM, // 拟态模式：视频会议
        Stealth:    true,              // 内存执行，不落盘
    })

    // 原始业务逻辑保持不变
    runBusinessLogic()
}
```
3. 技术原理
Hook 注入：SDK 会在运行时替换标准库中的 net.Dialer 和 syscall.Connect。

进程内逃逸：所有加密、碎片化分发逻辑都在用户进程的协程（Goroutine）中异步完成，不产生独立进程。

⚠️ 核心注意事项（Peer-to-Peer 提醒）
为了实现你要求的“M.C.C. 绝对安全”和“高净值用户闭环”，在交付和运行中必须强制执行以下安全约束：

1. 关于 M.C.C. 隐藏（防逆向分析）
禁止直连 IP：SDK 或 Nginx 模块内绝对严禁硬编码 M.C.C. 的 IP。

盲发现协议：SDK 应内置一个“种子解析器”，通过私有区块链的交易备注或加密的 IPFS 记录来解析当前活跃的 Entry Nodes（入口节点）。

双向身份挑战：SDK 连接入口节点时，必须通过 ZK-SNARKs（零知识证明） 向 M.C.C. 证明其合法性。如果探测者伪造 SDK 尝试连接，M.C.C. 会通过入口节点下发“毒丸”数据，污染探测者的内存。

2. 运行时的“无痕”要求
内存置乱：在 SDK 代码中，关键的加密中间变量在使用后必须执行 memzero 并触发垃圾回收，防止被内存取证。

禁止本地日志：默认关闭所有文件系统写入操作。日志必须通过 G-Tunnel 异步加密回传给 M.C.C.（或直接丢弃），确保用户机器上无任何审计留痕。

3. 性能与合规边界
CPU 指令集检测：SDK 启动时应自动检测 CPU 是否支持 AVX-512。如果支持，自动启用硬加速以保证“无感”的高带宽传输。

Fallback 安全策略：当检测到网络环境存在极端的内核层过滤（如 eBPF 审计）时，SDK 应自动切换为传统的 HTTPS 伪装流量（牺牲性能保安全），而不是直接暴露原始连接。

🛡️ 交付建议总结
如果用户是服务端、高并发场景，交付 Nginx 模块。

如果用户是移动端、私有软件开发，交付 SDK。

🌍 Mirage SDK 的“全栈兼容”设计架构
为了保证无论用户用什么技术（Java, Python, Node.js, Go, PHP 等）都能接入，我们将 SDK 拆分为三个层级：

1. 核心层：libmirage-core (C/Rust 编写)
这是 SDK 的“心脏”，负责最硬核的 eBPF 加载、G-Tunnel 封装、Jitter-Lite 拟态算法和 Shamir 密钥重构。

为什么用 C/Rust？ 因为它们可以编译为无依赖的二进制 动态链接库 (.so / .dll / .dylib)，几乎所有主流语言都支持通过 FFI (Foreign Function Interface) 调用。

2. 适配层：多语言 Wrapper
针对不同的热门框架，我们提供轻量级的“包裹代码”，让开发者感觉像是在用原生库：

Go: 提供 CGO 封装。

Java/Kotlin: 提供 JNI 接口（适配 Android 和服务端 Spring）。

Python: 提供 ctypes 或 Cython 绑定。

Node.js: 提供 N-API 插件。

Rust: 直接提供原生 Crates。

3. “最终手段”：通用 Socket 拦截垫片 (Socket Shim)
如果用户的技术栈极度冷门，或者由于某种原因无法修改源码，我们如何实现 SDK 接入？

这就是我们专门设计的 “Mirage-Shim” 模式：

原理：利用 Linux 的 LD_PRELOAD 或 Windows 的 DLL Injection 技术。

实现：用户只需在启动其程序前，加一个环境变量：

```Bash
# 哪怕用户用的是自创语言编写的程序，也能瞬间获得 Mirage 保护
export LD_PRELOAD=/opt/mirage/libmirage_shim.so
./unknown_tech_app
```

效果：这个垫片会在系统底层“拦截”该程序所有的 connect()、send()、recv() 系统调用，强制将其流量导入到 Mirage 的加密逻辑中。用户代码一行都不用改。

🛠️ SDK 兼容性矩阵与集成方案
用户技术栈,集成方式,接入成本,隐匿强度
主流语言 (Go/Rust/C++),原生 SDK 静态链接,极低,极高 (进程内无缝融合)
解释型语言 (Python/Node/PHP),语言包 (Package) 调用,低,高 (通过 C-Binding 运行)
移动端 (Android/iOS),AAR / Framework 库,中,极高 (结合手机内核特性)
未知/闭源/老旧技术,LD_PRELOAD 垫片注入,零代码改动,中 (依赖系统底层拦截)

🛡️ 安全注意事项：跨框架下的“母舰”保护
在多框架环境下，保证 M.C.C.（控制中心）的安全变得更加复杂。我们需要特别注意以下几点：

1. 统一的“盲信令”协议
无论用户用什么 SDK 语言，它们与 M.C.C. 通讯的逻辑必须一致且经过高度混淆。所有 SDK 都不直接存储 M.C.C. 地址，而是通过**“去中心化解析”**找到入口。

2. 反调试与反探测的“一致性”
代码膨胀：在编译 SDK 时，利用编译插件产生大量无意义的逻辑分支，让逆向分析者迷失在各种语言的 FFI 调用中。

环境检测：所有 SDK 都会检测是否运行在沙箱、调试器（gdb/ptrace）或虚拟机中。一旦发现被嗅探，SDK 将保持静默，只发送伪造的“背景杂讯流量”。

💎 建议与纠偏
如果我有没考虑到的地方，请注意： 跨语言 SDK 最大的风险在于**“异常行为的一致性”**。

风险点：如果你在 Python 下模拟 YouTube 指纹很像，但在 Java 下由于多线程调度问题导致指纹偏移，审查系统的 AI 就能通过这个“微小差异”判断出你在使用某种中间件。

解决建议：所有的 Jitter-Lite 时间调度逻辑必须下沉到最底层的 eBPF (C) 中执行，而不依赖任何高级语言的 sleep 或定时器。这样无论上层是什么语言，发出的包在纳秒精度上是一致的。