# 设计文档：Gateway 安全紧急封洞

## 概述

本设计覆盖 Gateway 侧 5 项紧急安全整改。所有改动遵循最小侵入原则：优先修改现有模块，不新建大型子系统。

## 设计原则

1. **改现有代码，不造新轮子**：所有模块已有骨架，只需补齐校验链路和生效路径
2. **C 做数据面，Go 做控制面**：黑名单命中在 eBPF TC 层执行，Go 侧只负责管理和同步
3. **Go → C 通过 eBPF Map**：黑名单通过 LPM Trie Map 下发，不使用直接函数调用
4. **失败时拒绝，不降级**：生产模式下 mTLS 缺失 = 拒绝启动，不静默降级

---

## 模块 1：mTLS 强制（需求 1）

### 改动范围

- `mirage-gateway/cmd/gateway/main.go`：启动校验逻辑
- `mirage-gateway/pkg/api/grpc_client.go`：移除 insecure 回退
- `mirage-gateway/pkg/api/grpc_server.go`：拒绝无 TLS 启动
- `mirage-gateway/configs/gateway.yaml`：默认 `tls.enabled: true`

### 设计细节

#### 启动校验

在 `main.go` 加载配置后、mTLS 初始化前，增加生产模式检查：

```go
// main.go loadConfig 之后
if os.Getenv("MIRAGE_ENV") == "production" && !cfg.MCC.TLS.Enabled {
    log.Fatalf("❌ 生产模式禁止禁用 mTLS，请配置 mcc.tls")
}
```

#### gRPC Client 移除 insecure 回退

当前 `grpc_client.go` Connect 方法中：

```go
if c.tlsConfig != nil {
    opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(c.tlsConfig)))
} else {
    opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
```

改为：

```go
if c.tlsConfig != nil {
    opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(c.tlsConfig)))
} else {
    return fmt.Errorf("mTLS 未配置，拒绝建立不安全连接")
}
```

#### gRPC Server 拒绝无 TLS 启动

`grpc_server.go` Start 方法中，如果 `s.tlsConfig == nil`，直接返回错误：

```go
func (s *GRPCServer) Start() error {
    if s.tlsConfig == nil {
        return fmt.Errorf("gRPC Server 拒绝启动：mTLS 未配置")
    }
    // ... 现有逻辑
}
```

---

## 模块 2：证书钉扎生效（需求 2）

### 改动范围

- `mirage-gateway/cmd/gateway/main.go`：将 certPin 传入 gRPC Client
- `mirage-gateway/pkg/api/grpc_client.go`：增加连接后证书校验
- `mirage-gateway/pkg/security/cert_pinning.go`：增加 TOFU 逻辑

### 设计细节

#### certPin 传入 gRPC Client

当前 `main.go` 中 `_ = certPin` 改为将 certPin 传入 GRPCClient：

```go
grpcClient = api.NewGRPCClient(cfg.MCC.Endpoint, gatewayID, clientTLS)
grpcClient.SetCertPin(certPin) // 新增
```

#### gRPC Client 连接后校验

在 `GRPCClient` 中增加 `certPin` 字段和 `SetCertPin` 方法。在 Connect 成功后，从 gRPC 连接中提取对端证书并调用 `certPin.VerifyPin()`：

```go
func (c *GRPCClient) SetCertPin(pin *security.CertPin) {
    c.certPin = pin
}
```

在 TLS 配置中注入 `VerifyPeerCertificate` 回调：

```go
if c.certPin != nil {
    tlsCfg := c.tlsConfig.Clone()
    tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
        if len(rawCerts) == 0 {
            return fmt.Errorf("no peer certificate")
        }
        cert, err := x509.ParseCertificate(rawCerts[0])
        if err != nil {
            return err
        }
        if !c.certPin.IsPinned() {
            // TOFU: 首次连接自动钉扎
            c.certPin.PinCertificate(cert)
            return nil
        }
        return c.certPin.VerifyPin(cert)
    }
    opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
}
```

#### 钉扎失败处理

VerifyPin 失败时 TLS 握手直接失败，连接不会建立。GRPCClient 的重连逻辑会记录错误并指数退避重试。

---

## 模块 3：入口异常流量可拒绝（需求 3）

### 改动范围

- `mirage-gateway/pkg/threat/responder.go`：增加自动封禁逻辑
- `mirage-gateway/pkg/threat/action.go`（新建）：入口处置动作枚举
- `mirage-gateway/pkg/threat/blacklist.go`：syncToEBPF 错误处理

### 设计细节

#### 入口处置动作枚举

```go
// pkg/threat/action.go
package threat

type IngressAction int

const (
    ActionPass     IngressAction = iota // 放行
    ActionObserve                       // 观察（记录但不阻断）
    ActionThrottle                      // 限速
    ActionTrap                          // 引流蜜罐
    ActionDrop                          // 静默丢弃
)
```

#### ThreatResponder 自动封禁

在 `responder.go` 的威胁等级变化回调中，当等级 >= HIGH(3) 时，将触发源 IP 加入黑名单：

```go
func (r *Responder) handleThreatEscalation(level ThreatLevel, sourceIP string) {
    if level >= ThreatHigh && sourceIP != "" && sourceIP != "0.0.0.0" {
        ttl := time.Hour
        if level >= ThreatCritical {
            ttl = 24 * time.Hour
        }
        if err := r.blacklist.Add(sourceIP+"/32", time.Now().Add(ttl), SourceLocal); err != nil {
            log.Printf("[Responder] 自动封禁失败: %s: %v", sourceIP, err)
        } else {
            log.Printf("[Responder] 自动封禁: %s (TTL=%v, level=%d)", sourceIP, ttl, level)
        }
    }
}
```

#### 安全日志

所有入口处置动作写入结构化安全日志：

```go
type IngressLog struct {
    Timestamp  time.Time     `json:"ts"`
    SourceIP   string        `json:"src"`
    Action     IngressAction `json:"action"`
    Reason     string        `json:"reason"`
    ThreatLevel int          `json:"level"`
}
```

---

## 模块 4：黑名单数据面生效（需求 4）

### 改动范围

- `mirage-gateway/bpf/common.h`：确认 `blacklist_lpm` Map 定义
- `mirage-gateway/bpf/jitter.c`（或新建 `ingress_filter.c`）：入口黑名单查询
- `mirage-gateway/pkg/threat/blacklist.go`：错误处理 + SyncStats
- `mirage-gateway/pkg/ebpf/loader.go`：确认 Map 加载

### 设计细节

#### eBPF 数据面黑名单查询（C）

在 TC ingress 路径（jitter.c 入口或独立程序）增加黑名单查询：

```c
// bpf/common.h 中确认定义
struct {
    __uint(type, BPF_MAP_TYPE_LPM_TRIE);
    __uint(max_entries, 65536);
    __uint(map_flags, BPF_F_NO_PREALLOC);
    __type(key, struct lpm_key);
    __type(value, __u32);
} blacklist_lpm SEC(".maps");

struct lpm_key {
    __u32 prefixlen;
    __u32 addr;
};
```

在 TC ingress 入口：

```c
SEC("tc")
int ingress_filter(struct __sk_buff *skb) {
    struct iphdr *ip = /* 解析 IP 头 */;
    struct lpm_key key = {
        .prefixlen = 32,
        .addr = ip->saddr,
    };
    __u32 *blocked = bpf_map_lookup_elem(&blacklist_lpm, &key);
    if (blocked) {
        return TC_ACT_SHOT; // 静默丢弃
    }
    return TC_ACT_OK;
}
```

#### Go 侧错误处理

`BlacklistManager.syncToEBPF` 当前写入失败只打日志，改为返回错误并向上传播：

```go
func (bm *BlacklistManager) syncToEBPF(cidr string, add bool) error {
    // ... 现有逻辑
    if add {
        if err := lpmMap.Put(key, &value); err != nil {
            log.Printf("[Blacklist] ❌ eBPF LPM 写入失败: %s: %v", cidr, err)
            return err
        }
    }
    return nil
}
```

#### SyncStats 一致性校验

```go
func (bm *BlacklistManager) SyncStats() (goCount int, ebpfCount int) {
    bm.mu.RLock()
    goCount = len(bm.entries)
    bm.mu.RUnlock()
    // 遍历 eBPF Map 计数
    ebpfCount = bm.countEBPFEntries()
    return
}
```

---

## 模块 5：高危命令源验证（需求 5）

### 改动范围

- `mirage-gateway/pkg/api/handlers.go`：签名校验 + 审计日志 + 速率限制
- `mirage-gateway/pkg/api/command_auth.go`（新建）：HMAC 签名校验器
- `mirage-gateway/pkg/api/command_audit.go`（新建）：命令审计日志
- `mirage-gateway/pkg/api/command_ratelimit.go`（新建）：命令速率限制
- `mirage-gateway/configs/gateway.yaml`：增加 `security.command_secret`

### 设计细节

#### HMAC 签名校验

```go
// pkg/api/command_auth.go
package api

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "google.golang.org/grpc/metadata"
)

type CommandAuthenticator struct {
    secret []byte
}

func NewCommandAuthenticator(secret string) *CommandAuthenticator {
    return &CommandAuthenticator{secret: []byte(secret)}
}

// Verify 从 gRPC metadata 中提取签名并校验
// metadata key: "x-mirage-sig" = HMAC-SHA256(secret, commandType + timestamp)
// metadata key: "x-mirage-ts" = Unix timestamp string
func (ca *CommandAuthenticator) Verify(ctx context.Context, commandType string) error {
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return fmt.Errorf("missing metadata")
    }
    sig := md.Get("x-mirage-sig")
    ts := md.Get("x-mirage-ts")
    if len(sig) == 0 || len(ts) == 0 {
        return fmt.Errorf("missing signature")
    }
    // 校验时间窗口（±60秒）
    // 计算 HMAC 并比较
    mac := hmac.New(sha256.New, ca.secret)
    mac.Write([]byte(commandType + ts[0]))
    expected := hex.EncodeToString(mac.Sum(nil))
    if !hmac.Equal([]byte(sig[0]), []byte(expected)) {
        return fmt.Errorf("signature mismatch")
    }
    return nil
}
```

#### 命令审计日志

```go
// pkg/api/command_audit.go
package api

type CommandAuditEntry struct {
    Timestamp   time.Time `json:"ts"`
    CommandType string    `json:"cmd"`
    SourceAddr  string    `json:"src"`
    Params      string    `json:"params"`
    Result      string    `json:"result"`
    Success     bool      `json:"ok"`
}
```

所有 CommandHandler 方法在执行前后写审计日志。

#### 速率限制

```go
// pkg/api/command_ratelimit.go
package api

type CommandRateLimiter struct {
    mu       sync.Mutex
    counters map[string]*rateBucket // sourceAddr → bucket
    limit    int                     // 每分钟上限
    window   time.Duration
}
```

高危命令（PushReincarnation、defense_level >= 4、remaining_bytes = 0）每分钟每来源最多 10 次。

#### CommandHandler 集成

在每个高危命令处理方法开头增加三步校验：

```go
func (h *CommandHandler) PushReincarnation(ctx context.Context, req *pb.ReincarnationPush) (*pb.PushResponse, error) {
    // 1. 速率限制
    if err := h.rateLimiter.Check(peerAddr(ctx)); err != nil {
        h.audit.Log("PushReincarnation", peerAddr(ctx), req, false, err.Error())
        return nil, status.Errorf(codes.ResourceExhausted, "rate limited")
    }
    // 2. 签名校验
    if err := h.auth.Verify(ctx, "PushReincarnation"); err != nil {
        h.audit.Log("PushReincarnation", peerAddr(ctx), req, false, err.Error())
        return nil, status.Errorf(codes.PermissionDenied, "auth failed")
    }
    // 3. 审计日志
    defer func() { h.audit.Log("PushReincarnation", peerAddr(ctx), req, true, "ok") }()
    // ... 现有业务逻辑
}
```

---

## 配置变更

`gateway.yaml` 新增：

```yaml
security:
  command_secret: "${MIRAGE_COMMAND_SECRET}"  # HMAC 签名密钥
```

`mcc.tls.enabled` 默认值改为 `true`。

---

## 不在本次范围内

- OS 侧的对应签名生成逻辑（由 Spec 1-1 覆盖）
- 版本级安全改造（Gateway 安全状态机、跨节点威胁情报）
- Phantom 蜜罐收敛（由 Spec 2-4 覆盖）
- 零信任三层纵深防御（由 Spec 3-3 覆盖）
