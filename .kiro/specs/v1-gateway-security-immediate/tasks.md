# 任务清单：Gateway 安全紧急封洞

## 需求 1：生产模式强制 mTLS，禁止明文回退

- [x] 1. mTLS 强制
  - [x] 1.1 修改 `mirage-gateway/cmd/gateway/main.go`：在 `loadConfig` 之后、`NewTLSManager` 之前增加生产模式校验 — 当 `os.Getenv("MIRAGE_ENV") == "production"` 且 `cfg.MCC.TLS.Enabled == false` 时调用 `log.Fatalf` 拒绝启动
  - [x] 1.2 修改 `mirage-gateway/pkg/api/grpc_client.go` Connect 方法：移除 `insecure.NewCredentials()` 回退分支，当 `c.tlsConfig == nil` 时直接返回 `fmt.Errorf("mTLS 未配置，拒绝建立不安全连接")`
  - [x] 1.3 修改 `mirage-gateway/pkg/api/grpc_server.go` Start 方法：在方法开头增加 `if s.tlsConfig == nil { return fmt.Errorf("gRPC Server 拒绝启动：mTLS 未配置") }`
  - [x] 1.4 修改 `mirage-gateway/cmd/gateway/main.go` gRPC Server 启动段：当 `grpcServer.Start()` 返回错误时，从 `log.Printf("⚠️ ...")` 降级运行改为 `log.Fatalf("❌ ...")` 终止启动
  - [x] 1.5 修改 `mirage-gateway/cmd/gateway/main.go` gRPC Client 启动段：当 `clientTLS` 为 nil 时（`tlsMgr.GetClientTLSConfig()` 返回 nil），不再创建 GRPCClient，直接 `log.Fatalf` 终止
  - [x] 1.6 修改 `mirage-gateway/configs/gateway.yaml`：将 `mcc.tls.enabled` 默认值从 `false` 改为 `true`，添加注释说明生产模式下此项必须为 true

## 需求 2：证书钉扎接入连接校验链

- [x] 2. 证书钉扎生效
  - [x] 2.1 修改 `mirage-gateway/pkg/api/grpc_client.go`：为 `GRPCClient` 结构体增加 `certPin *security.CertPin` 字段和 `SetCertPin(pin *security.CertPin)` 方法，增加 `import "mirage-gateway/pkg/security"`
  - [x] 2.2 修改 `mirage-gateway/pkg/api/grpc_client.go` Connect 方法：当 `c.certPin != nil` 时，Clone tlsConfig 并注入 `VerifyPeerCertificate` 回调 — 解析对端证书后调用 `c.certPin.VerifyPin(cert)`，若 certPin 未钉扎（`!c.certPin.IsPinned()`）则执行 TOFU 调用 `c.certPin.PinCertificate(cert)`
  - [x] 2.3 修改 `mirage-gateway/cmd/gateway/main.go`：删除 `_ = certPin` 行，在 `grpcClient = api.NewGRPCClient(...)` 之后增加 `if certPin != nil { grpcClient.SetCertPin(certPin) }`
  - [x] 2.4 修改 `mirage-gateway/pkg/security/cert_pinning.go`：为 `VerifyPin` 方法增加失败时的详细日志（预期指纹前16位 vs 实际指纹前16位），返回包含两个指纹摘要的 error 信息

## 需求 3：入口异常流量可拒绝

- [x] 3. 入口处置能力
  - [x] 3.1 创建 `mirage-gateway/pkg/threat/action.go`：定义 `IngressAction` 枚举（ActionPass=0 / ActionObserve=1 / ActionThrottle=2 / ActionTrap=3 / ActionDrop=4）和 `String()` 方法
  - [x] 3.2 创建 `mirage-gateway/pkg/threat/ingress_log.go`：定义 `IngressLog` 结构体（Timestamp / SourceIP / Action / Reason / ThreatLevel）和 `IngressLogger` — 内部使用 `[]IngressLog` 环形缓冲（maxEntries=10000），提供 `Log(entry IngressLog)` 和 `Recent(n int) []IngressLog` 方法
  - [x] 3.3 修改 `mirage-gateway/pkg/threat/responder.go`：增加 `blacklist *BlacklistManager` 字段，在 `NewResponder` 参数中传入；在威胁等级变化处理逻辑中，当 `level >= ThreatHigh(3)` 且 sourceIP 有效时，调用 `blacklist.Add(sourceIP+"/32", time.Now().Add(ttl), SourceLocal)` — HIGH 级 TTL=1h，CRITICAL 级 TTL=24h
  - [x] 3.4 修改 `mirage-gateway/cmd/gateway/main.go`：将 `blacklist` 传入 `threat.NewResponder(engine, loader, blacklist)` 调用

## 需求 4：黑名单到数据面生效链路修复

- [x] 4. 黑名单数据面生效
  - [x] 4.1 检查 `mirage-gateway/bpf/common.h`：确认 `blacklist_lpm` Map 定义存在（类型 BPF_MAP_TYPE_LPM_TRIE，max_entries 65536，key 为 struct lpm_key { __u32 prefixlen; __u32 addr; }，value 为 __u32），如不存在则添加
  - [x] 4.2 修改 `mirage-gateway/bpf/jitter.c` TC ingress 入口函数：在现有逻辑最前面增加 blacklist_lpm 查询 — 解析 IP 头获取 saddr，构造 lpm_key（prefixlen=32, addr=saddr），调用 `bpf_map_lookup_elem(&blacklist_lpm, &key)`，命中时返回 `TC_ACT_SHOT`
  - [x] 4.3 修改 `mirage-gateway/pkg/threat/blacklist.go` `syncToEBPF` 方法：将返回类型从无返回值改为 `error`，Put 失败时返回错误；同步修改 `Add` 和 `MergeGlobal` 中调用 syncToEBPF 的位置，记录但不阻断业务（syncToEBPF 失败时 log.Printf 但 Add 仍返回 nil）
  - [x] 4.4 修改 `mirage-gateway/pkg/threat/blacklist.go`：在 `NewBlacklistManager` 中增加启动校验 — 调用 `loader.GetMap("blacklist_lpm")`，如果返回 nil 则 `log.Printf("⚠️ blacklist_lpm Map 不存在，黑名单数据面降级")` 并设置 `bm.degraded = true` 字段
  - [x] 4.5 创建 `mirage-gateway/pkg/threat/blacklist.go` `SyncStats` 方法：返回 `(goCount int, ebpfCount int)`，goCount 为 `len(bm.entries)`，ebpfCount 通过遍历 eBPF Map 的 `NextKey` 循环计数

## 需求 5：高危命令收敛到唯一可信来源

- [x] 5. 高危命令源验证
  - [x] 5.1 创建 `mirage-gateway/pkg/api/command_auth.go`：实现 `CommandAuthenticator` 结构体 — `NewCommandAuthenticator(secret string)` 构造函数；`Verify(ctx context.Context, commandType string) error` 方法从 gRPC metadata 提取 `x-mirage-sig`（HMAC-SHA256 hex）和 `x-mirage-ts`（Unix timestamp string），校验时间窗口 ±60 秒，计算 `HMAC-SHA256(secret, commandType+timestamp)` 并用 `hmac.Equal` 比较
  - [x] 5.2 创建 `mirage-gateway/pkg/api/command_audit.go`：实现 `CommandAuditor` 结构体 — 内部 `[]CommandAuditEntry` 环形缓冲（maxEntries=5000）；`Log(commandType, sourceAddr, params string, success bool, message string)` 方法写入缓冲并 `log.Printf` 输出；`Recent(n int) []CommandAuditEntry` 查询方法
  - [x] 5.3 创建 `mirage-gateway/pkg/api/command_ratelimit.go`：实现 `CommandRateLimiter` 结构体 — 内部 `map[string]*rateBucket`（sourceAddr → bucket），每个 bucket 记录 `count int` 和 `windowStart time.Time`；`Check(sourceAddr string) error` 方法：窗口 1 分钟，上限 10 次，超出返回 error；`cleanup()` 方法定期清理过期 bucket
  - [x] 5.4 修改 `mirage-gateway/pkg/api/handlers.go`：为 `CommandHandler` 增加 `auth *CommandAuthenticator`、`audit *CommandAuditor`、`rateLimiter *CommandRateLimiter` 三个字段，增加 `SetAuth`、`SetAudit`、`SetRateLimiter` 方法
  - [x] 5.5 修改 `mirage-gateway/pkg/api/handlers.go` `PushReincarnation` 方法：在业务逻辑前增加三步校验 — ① `h.rateLimiter.Check(peerAddr(ctx))` 失败返回 `codes.ResourceExhausted` ② `h.auth.Verify(ctx, "PushReincarnation")` 失败返回 `codes.PermissionDenied` ③ defer 审计日志
  - [x] 5.6 修改 `mirage-gateway/pkg/api/handlers.go` `PushStrategy` 方法：当 `req.DefenseLevel >= 4` 时执行签名校验和速率限制（同 5.5 模式）；所有调用都写审计日志
  - [x] 5.7 修改 `mirage-gateway/pkg/api/handlers.go` `PushQuota` 方法：当 `req.RemainingBytes == 0` 时执行签名校验和速率限制（同 5.5 模式）；所有调用都写审计日志
  - [x] 5.8 修改 `mirage-gateway/pkg/api/handlers.go` `PushBlacklist` 方法：所有调用都写审计日志（黑名单下发不做签名校验，因为已通过 mTLS 认证）
  - [x] 5.9 创建 `mirage-gateway/pkg/api/handlers.go` `peerAddr` 辅助函数：从 `context.Context` 中通过 `peer.FromContext(ctx)` 提取对端地址字符串
  - [x] 5.10 修改 `mirage-gateway/configs/gateway.yaml`：在 `security` 段增加 `command_secret: "${MIRAGE_COMMAND_SECRET}"` 配置项
  - [x] 5.11 修改 `mirage-gateway/cmd/gateway/main.go`：在创建 `CommandHandler` 后，初始化 `CommandAuthenticator`（从 `cfg.Security.CommandSecret` 读取）、`CommandAuditor`、`CommandRateLimiter`，通过 Set 方法注入到 handler
  - [x] 5.12 修改 `mirage-gateway/cmd/gateway/main.go` `GatewayConfig.SecurityConfig`：增加 `CommandSecret string \`yaml:"command_secret"\`` 字段
