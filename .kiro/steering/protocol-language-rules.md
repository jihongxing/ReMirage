---
inclusion: always
---

# 协议开发语言强制规则

## 核心原则

**我们要做的不是简单的代码堆砌，而是在网络底层构建一套隐形的战争机器。**

**C 做内核数据面，Go 做控制面**

在开发 Mirage Project 的六大协议时，必须严格遵守以下语言分配规则。这是铁律，不可违反。

---

## 语言分配表（强制执行）

| 协议 | 核心实现语言 | 强制理由 |
|------|-------------|---------|
| **NPM（流量伪装）** | **C (eBPF XDP)** | 必须在网卡层直接追加 Padding，实现零拷贝。Go 无法访问 XDP 层。 |
| **B-DNA（指纹识别）** | **C (eBPF TC)** | 必须在 TCP 握手阶段修改 Header 字段和 Window Size。需要内核态操作。 |
| **VPC（噪声注入）** | **C (eBPF TC)** | 必须使用纳秒级精度定时器模拟光缆抖动。Go 时间精度不足。 |
| **Jitter-Lite（时域扰动）** | **C (eBPF TC)** | 必须控制 skb->tstamp（内核时间戳），实现绝对精准 IAT。Go 无法访问。 |
| **G-Tunnel（多路径）** | **Go 控制 + C 数据面** | Go 负责路径调度与 BBR v3，C 负责包拆分与 FEC（AVX-512 优化）。 |
| **G-Switch（域名转生）** | **Go (M.C.C.)** | 涉及 API 调用、数据库更新、Raft 一致性，属于高层逻辑。 |

---

## 禁止行为

### ❌ 禁止使用 Go 实现以下功能

1. **XDP 层数据包处理**
   - Go 无法挂载到 XDP Hook 点
   - 无法实现零拷贝

2. **内核态 skb 操作**
   - Go 无法直接修改 skb->tstamp
   - 无法访问 TC 队列

3. **纳秒级定时控制**
   - Go 的 time.Sleep 精度为毫秒级
   - 无法满足光缆抖动模拟需求

4. **TCP Header 修改**
   - Go 无法在握手阶段修改 Window Size
   - 无法实现 JA4 指纹伪装

### ❌ 禁止使用 C 实现以下功能

1. **API 调用与 HTTP 请求**
   - C 语言 HTTP 库复杂且易出错
   - Go 标准库更安全高效

2. **数据库操作**
   - C 语言 ORM 不成熟
   - Go 的 GORM/sqlx 更易维护

3. **Raft 一致性协议**
   - C 语言实现复杂度极高
   - Go 有成熟的 hashicorp/raft 库

4. **复杂业务逻辑**
   - C 语言错误处理繁琐
   - Go 的 error 接口更清晰

---

## 通信机制（强制规范）

### Go → C（控制指令）

**必须使用 eBPF Map**：

```go
// ✅ 正确：通过 eBPF Map 下发配置
func (ec *EBPFController) SetThreatLevel(level int) error {
    key := uint32(0)
    value := uint32(level)
    return ec.maps["threat_level_map"].Put(key, value)
}

// ❌ 错误：尝试直接调用 C 函数
// func (ec *EBPFController) SetThreatLevel(level int) error {
//     C.set_threat_level(C.int(level)) // 不可行
// }
```

### C → Go（数据上报）

**必须使用 Ring Buffer**：

```c
// ✅ 正确：通过 Ring Buffer 上报事件
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} threat_events SEC(".maps");

SEC("xdp")
int detect_threat(struct xdp_md *ctx) {
    struct threat_event *event = bpf_ringbuf_reserve(&threat_events, sizeof(*event), 0);
    if (!event)
        return XDP_PASS;
    
    event->timestamp = bpf_ktime_get_ns();
    event->threat_type = THREAT_ACTIVE_PROBING;
    
    bpf_ringbuf_submit(event, 0);
    return XDP_PASS;
}
```

```go
// ✅ 正确：Go 从 Ring Buffer 读取
func (ec *EBPFController) MonitorThreats() {
    reader, _ := ringbuf.NewReader(ec.maps["threat_events"])
    for {
        record, _ := reader.Read()
        var event ThreatEvent
        binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &event)
        ec.handleThreat(&event)
    }
}
```

---

## 性能要求（强制指标）

### C 数据面性能要求

| 指标 | 要求 | 验证方式 |
|------|------|---------|
| 延迟增加 | < 1ms | bpftrace 测量 |
| CPU 占用 | < 5% | perf top |
| 内存占用 | < 50MB | /proc/meminfo |
| 零拷贝率 | > 95% | XDP 统计 |

### Go 控制面性能要求

| 指标 | 要求 | 验证方式 |
|------|------|---------|
| 响应时间 | < 100ms | pprof |
| 内存占用 | < 200MB | runtime.MemStats |
| Goroutine 数 | < 1000 | runtime.NumGoroutine |
| GC 暂停 | < 10ms | GODEBUG=gctrace=1 |

---

## 代码组织（强制结构）

```
mirage-gateway/
├── bpf/                    # C 数据面（eBPF 程序）
│   ├── npm.c              # NPM 协议（XDP）
│   ├── bdna.c             # B-DNA 协议（TC）
│   ├── jitter.c           # Jitter-Lite 协议（TC）
│   ├── vpc.c              # VPC 协议（TC）
│   ├── gtunnel.c          # G-Tunnel 数据面（FEC）
│   └── common.h           # 公共头文件
│
├── pkg/                    # Go 控制面
│   ├── ebpf/              # eBPF 管理器
│   ├── gtunnel/           # G-Tunnel 控制面
│   ├── gswitch/           # G-Switch 协议
│   ├── strategy/          # 策略引擎
│   └── threat/            # 威胁检测
│
└── cmd/
    └── gateway/           # 主程序入口（Go）
```

---

## 编译要求（强制流程）

### C 数据面编译

```bash
# ✅ 必须使用 clang + BPF target
clang -O2 -target bpf -c npm.c -o npm.o

# ❌ 禁止使用 gcc
# gcc -c npm.c -o npm.o  # 不支持 BPF target
```

### Go 控制面编译

```bash
# ✅ 必须启用 CGO（用于加载 eBPF）
CGO_ENABLED=1 go build -o mirage-gateway cmd/gateway/main.go

# ❌ 禁止禁用 CGO
# CGO_ENABLED=0 go build  # 无法加载 eBPF 程序
```

---

## 开发检查清单

### 开始开发前

- [ ] 确认协议属于哪个维度（空间/形态/指纹/存活/时间/背景）
- [ ] 查表确认使用 C 还是 Go
- [ ] 确认是否需要内核态操作
- [ ] 确认性能要求（延迟/CPU/内存）

### 开发过程中

- [ ] C 代码：使用 eBPF Map 接收配置
- [ ] C 代码：使用 Ring Buffer 上报事件
- [ ] C 代码：避免动态内存分配
- [ ] Go 代码：使用 cilium/ebpf 库加载程序
- [ ] Go 代码：实现降级方案（eBPF 加载失败时）

### 开发完成后

- [ ] 性能测试：验证延迟 < 1ms（C）或 < 100ms（Go）
- [ ] 压力测试：验证 CPU < 5%（C）或 < 15%（Go）
- [ ] 内存测试：验证内存 < 50MB（C）或 < 200MB（Go）
- [ ] 兼容性测试：验证内核版本 ≥ 5.15 或 ≥ 4.19（Fallback）

---

## 常见错误与纠正

### 错误 1：尝试用 Go 实现 XDP

```go
// ❌ 错误：Go 无法实现 XDP
func (npm *NPM) ApplyPadding(packet []byte) []byte {
    padding := make([]byte, 100)
    return append(packet, padding...)
}
```

**纠正**：必须使用 C + eBPF XDP

```c
// ✅ 正确：C eBPF XDP
SEC("xdp")
int apply_padding(struct xdp_md *ctx) {
    if (bpf_xdp_adjust_tail(ctx, 100) < 0)
        return XDP_PASS;
    return XDP_PASS;
}
```

### 错误 2：尝试用 C 实现 HTTP API

```c
// ❌ 错误：C 语言 HTTP 库复杂
int call_api(const char *url) {
    CURL *curl = curl_easy_init();
    // 100+ 行代码...
}
```

**纠正**：必须使用 Go

```go
// ✅ 正确：Go 标准库
func (gs *GSwitch) CallAPI(url string) error {
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil
}
```

### 错误 3：C 和 Go 直接函数调用

```go
// ❌ 错误：尝试直接调用 C 函数
// #cgo LDFLAGS: -L. -lebpf
// #include "npm.h"
// import "C"
// func ApplyPadding() {
//     C.apply_padding() // eBPF 程序无法这样调用
// }
```

**纠正**：必须通过 eBPF Map 通信

```go
// ✅ 正确：通过 Map 通信
func (npm *NPM) EnablePadding() error {
    key := uint32(0)
    value := uint32(1) // 启用
    return npm.maps["padding_enabled"].Put(key, value)
}
```

---

## 审查要点

在代码审查时，必须检查：

1. **语言选择正确性**
   - NPM/B-DNA/VPC/Jitter-Lite 必须是 C
   - G-Switch 必须是 Go
   - G-Tunnel 必须是 Go + C 混合

2. **通信机制正确性**
   - Go → C 必须使用 eBPF Map
   - C → Go 必须使用 Ring Buffer
   - 禁止直接函数调用

3. **性能指标达标**
   - C 延迟 < 1ms
   - Go 响应 < 100ms
   - 总 CPU < 20%

4. **错误处理完整性**
   - C 失败时返回默认行为（XDP_PASS）
   - Go 失败时启用降级方案
   - 禁止 panic

---

## 总结

**记住三个原则**：

1. **数据包处理 = C**（XDP/TC 层）
2. **业务逻辑 = Go**（API/数据库/Raft）
3. **通信 = eBPF Map + Ring Buffer**（禁止直接调用）

违反这些规则将导致：
- 性能下降 10 倍以上
- 无法实现核心功能
- 代码无法编译或运行

