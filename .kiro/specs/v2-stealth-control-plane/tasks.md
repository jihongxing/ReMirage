# 任务清单：V2 隐蔽控制面承载

## 1. Protobuf 控制指令定义
- [x] 1.1 创建 `mirage-proto/control_command.proto`，定义 ControlCommandType 枚举（5 种）、ControlCommand 消息、ControlCommandAck 消息
- [x] 1.2 更新 `mirage-proto/generate.sh` 和 `Makefile`，生成 Go 代码到 `mirage-proto/gen/`
- [x] 1.3 PBT: Property 1 — Protobuf ControlCommand 序列化 round-trip（生成随机 ControlCommand，验证 Marshal/Unmarshal 等价）

## 2. 隐写密码学工具
- [x] 2.1 创建 `pkg/gtunnel/stego/crypto.go`，实现 HMACTag、HMACVerify（恒定时间）、Encrypt（ChaCha20-Poly1305）、Decrypt、RandomPadding 函数
- [x] 2.2 PBT: Property 14 — ChaCha20-Poly1305 加解密 round-trip（生成随机密钥和明文，验证 Encrypt/Decrypt 等价；损坏密文返回错误）
- [x] 2.3 创建 `pkg/gtunnel/stego/payload.go`，实现 BuildStegoPayload 和 ParseStegoPayload 函数
- [x] 2.4 PBT: Property 3 — 隐写负载长度不变量（生成随机 ControlCommand 和 targetLen，验证输出长度严格等于 targetLen）
- [x] 2.5 PBT: Property 4 — 隐写编解码 round-trip（生成随机 ControlCommand、密钥、targetLen，验证 Build→Parse 等价）

## 3. StegoEncoder 隐写编码器
- [x] 3.1 创建 `pkg/gtunnel/stego/encoder.go`，实现 StegoEncoder（Enqueue、Encode、GetRate），含隐写率限制逻辑
- [x] 3.2 PBT: Property 6 — 隐写长度不足拒绝（生成大 ControlCommand 和小 dummyLen，验证 Encode 返回 nil）
- [x] 3.3 PBT: Property 7 — 隐写替换率上限不变量（模拟大量编码操作，验证 GetRate() ≤ maxRate）

## 4. StegoDecoder 隐写解码器
- [x] 4.1 创建 `pkg/gtunnel/stego/decoder.go`，实现 StegoDecoder（Decode、IsStego），HMAC 校验使用恒定时间比较
- [x] 4.2 PBT: Property 5 — 非隐写包静默丢弃（生成随机字节数组，验证 Decode 返回 (nil, nil)）

## 5. ControlCommand ↔ ControlEvent 转换
- [x] 5.1 创建 `pkg/gtunnel/stealth/converter.go`，实现 ToControlEvent 和 FromControlEvent 双向转换函数
- [x] 5.2 PBT: Property 2 — ControlCommand 与 ControlEvent 双向转换（生成随机 ControlCommand，验证 ToControlEvent→FromControlEvent round-trip）

## 6. ShadowStreamMux 方案 A
- [x] 6.1 创建 `pkg/gtunnel/stealth/mux.go`，实现 ShadowStreamMux（NewShadowStreamMux、WriteCommand、ReadCommand、IsAvailable、Close）
- [x] 6.2 单元测试：Mock quic.Connection，验证 Stream 0 创建、Protobuf 写入/读取、IsAvailable 状态

## 7. TransitionSmoother 流量潮汐
- [x] 7.1 创建 `pkg/gtunnel/smoother/smoother.go`，实现 MarkovMatrix 类型、PacketSizeDistribution 类型、TransitionSmoother（BeginTransition、CurrentMatrix、CurrentDistribution、Alpha、IsTransitioning）
- [x] 7.2 PBT: Property 8 — 马尔可夫矩阵线性插值正确性（生成随机矩阵对和 α 值，验证插值结果和边界条件）
- [x] 7.3 PBT: Property 9 — 过渡中断连续性（在过渡中途调用 BeginTransition，验证新起始状态等于中断时刻的插值结果）
- [x] 7.4 单元测试：duration=0 使用默认 3000ms、矩阵维度不匹配返回错误

## 8. ConstantTimeLock 恒定时间锁
- [x] 8.1 创建 `pkg/gtunnel/ctlock/ctlock.go`，实现 ConstantTimeLock（ProcessControl、ProcessStego、FakeCryptoWork、BusyWaitUntil），使用 spin-loop
- [x] 8.2 PBT: Property 10 — 恒定时间处理对齐（生成随机耗时 handler，验证总执行时间在 [slotNs, slotNs+tolerance] 范围内，isStego=true/false 耗时差异 < tolerance）
- [x] 8.3 单元测试：handler 耗时超过 slotNs 时立即返回且 AuditCollector 被调用

## 9. CommandTracker 指令可靠性
- [x] 9.1 创建 `pkg/gtunnel/stealth/reliability.go`，实现 CommandTracker（Track、Acknowledge、CheckTimeouts、OnMaxRetryExceeded）
- [x] 9.2 PBT: Property 12 — 方案 B 重试次数上限（模拟超时序列，验证重试次数 ≤ maxRetry）
- [x] 9.3 单元测试：确认超时后重新排队、3 次重试后触发 AuditCollector 和 EventSurvivalModeChange

## 10. StealthControlPlane 通道选择器
- [x] 10.1 创建 `pkg/gtunnel/stealth/control_plane.go`，实现 StealthControlPlane（SendCommand、ReceiveLoop、GetChannelState、Close），含通道选择状态机和本地队列（容量 64）
- [x] 10.2 PBT: Property 11 — 通道选择不变量（Mock 方案 A/B 可用性组合，验证 GetChannelState 返回正确状态和队列容量）
- [x] 10.3 PBT: Property 13 — 指令去重幂等性（发送相同 command_id 两次，验证只处理一次）
- [x] 10.4 单元测试：通道切换时 TimelineCollector 被调用、队列满时丢弃最旧指令

## 11. eBPF 数据面隐写标记
- [x] 11.1 扩展 `bpf/npm.c`，新增 stego_ready_map（RINGBUF）和 stego_command_map（ARRAY）定义，新增 stego_ready_event 和 stego_command 结构体
- [x] 11.2 扩展 `bpf/npm.c` 废包生成逻辑，在生成 Dummy_Packet 时写入 stego_ready_map 通知，发送前检查 stego_command_map 执行替换
- [x] 11.3 创建 `pkg/gtunnel/stego/ebpf_bridge.go`，实现 Go 侧 Ring Buffer 读取和 eBPF Map 写入桥接

## 12. 集成测试
- [x] 12.1 集成测试：ShadowStreamMux 在真实 QUIC 连接上的 Stream 0 读写（loopback）
- [x] 12.2 集成测试：ShadowStreamMux Stream 0 写入不阻塞 Stream 1~N（并发 goroutine）
- [x] 12.3 集成测试：StealthControlPlane 通道切换端到端（方案 A → 方案 B → 方案 A）
- [x] 12.4 集成测试：完整隐写流程端到端（编码 → eBPF Map Mock → 解码 → EventDispatcher）
- [x] 12.5 集成测试：多 goroutine 并发 SendCommand（-race 检测）
