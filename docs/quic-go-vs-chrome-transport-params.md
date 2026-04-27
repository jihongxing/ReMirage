# quic-go 默认 Transport Parameters 与 Chrome 140+ 差异清单

> 生成日期：2025-07  
> 对比基线：quic-go v0.59.0 vs Chromium quiche (Chrome 140+)  
> 目的：记录当前 quic-go 栈无法通过 `*quic.Config` 对齐的 QUIC Transport Parameters 差异，作为技术债务追踪

## 已对齐参数（通过 quic.Config / Transport）

| Transport Parameter | Chrome 140+ 典型值 | quic-go 当前配置 | 状态 |
|---|---|---|---|
| ALPN (NextProtos) | `h3` | `h3` ✅ | 已对齐 |
| max_idle_timeout | 30s | 30s ✅ | 已对齐 |
| initial_max_stream_data_bidi_local | ~6MB | 6MB (InitialStreamReceiveWindow) ✅ | 已对齐 |
| initial_max_stream_data_bidi_remote | ~6MB | 6MB (InitialStreamReceiveWindow) ✅ | 已对齐 |
| max_connection_receive_window | ~15MB | 15MB (InitialConnectionReceiveWindow) ✅ | 已对齐 |
| InitialPacketSize | 1200 | 1200 ✅ | 已对齐 |
| enable_datagrams | true | true ✅ | 已对齐 |
| CID 长度 | 8 bytes | 8 bytes (Transport.ConnectionIDLength) ✅ | 已对齐 |

## 未对齐参数 — 技术债务

### 高优先级差异（DPI 可检测）

| Transport Parameter | Chrome 140+ 典型值 | quic-go 默认值 | 差异影响 | 可控性 |
|---|---|---|---|---|
| initial_max_data | ~15MB (15728640) | 由 InitialConnectionReceiveWindow 间接控制，但 quic-go 内部计算逻辑与 Chrome 不同 | 中 — 精确值差异可被 JA4Q 指纹捕获 | 部分可控 |
| max_udp_payload_size | 1472 (Chrome 典型) | 1452 (quic-go 默认) | 中 — 差异 20 bytes，可被精确指纹匹配 | ❌ 不可通过 Config 设置 |
| initial_max_streams_bidi | 100 (Chrome 典型) | 100 (quic-go 默认) | 低 — 值一致 | ✅ 可通过 MaxIncomingStreams |
| initial_max_streams_uni | 100 (Chrome 典型) | 100 (quic-go 默认) | 低 — 值一致 | ✅ 可通过 MaxIncomingUniStreams |
| initial_max_stream_data_uni | ~6MB | 由 InitialStreamReceiveWindow 控制 | 低 | 部分可控 |

### 中优先级差异（需要深度分析才能检测）

| Transport Parameter | Chrome 140+ 典型值 | quic-go 默认值 | 差异影响 | 可控性 |
|---|---|---|---|---|
| ack_delay_exponent | 3 (RFC 默认) | 3 (RFC 默认) | 无 — 值一致 | ✅ |
| max_ack_delay | 25ms (RFC 默认) | 25ms (RFC 默认) | 无 — 值一致 | ✅ |
| active_connection_id_limit | 2 (RFC 默认) | 2 (quic-go 默认) | 低 — Chrome 可能发送更高值 | ❌ 不可通过 Config 设置 |
| disable_active_migration | false (Chrome 客户端) | false (quic-go 默认) | 无 — 值一致 | ✅ |

### Chrome 特有参数（quic-go 不发送）

| Transport Parameter | Chrome 行为 | quic-go 行为 | 差异影响 |
|---|---|---|---|
| google_connection_options | 发送 QUIC 连接选项 (如 `RENO`, `BBR3`) | 不发送 | 中 — 缺失此参数可区分非 Chrome |
| google_handshake_message | 客户端发送 | 不发送 | 中 — 缺失可区分 |
| initial_round_trip_time | 客户端发送估计 RTT | 不发送 | 低 — 非必需参数 |
| version_information (RFC 9369) | 发送 chosen_version + other_versions | quic-go 发送 | 低 — 两者都发送 |
| GREASE transport parameters | 随机插入 GREASE 参数 (id % 31 == 27) | 不发送 GREASE | 中 — 缺失 GREASE 可被指纹检测 |

### quic-go 特有行为（Chrome 不具备）

| 行为 | quic-go | Chrome | 差异影响 |
|---|---|---|---|
| Transport Parameter 编码顺序 | 固定顺序 | 随机化顺序 | 中 — 固定顺序是指纹特征 |
| GREASE 注入 | 不注入 | 注入随机 GREASE TP | 中 — 缺失 GREASE 可被检测 |
| max_datagram_frame_size | 发送（因 EnableDatagrams=true） | 仅在使用 WebTransport 时发送 | 低 — H3 场景下差异可能暴露 |

## 风险评估

### 当前风险等级：中

通过 `quic.Config` 可用字段已对齐核心参数（idle timeout、flow control windows、packet size、CID length），消除了最明显的差异。

剩余差异主要集中在：
1. `max_udp_payload_size` 精确值差异（1452 vs 1472）
2. Chrome 特有的 Google 扩展参数缺失
3. GREASE transport parameters 缺失
4. Transport Parameter 编码顺序固定

### 缓解方案（需上游支持）

| 方案 | 复杂度 | 效果 |
|---|---|---|
| Fork quic-go 添加 Transport Parameters Hook | 高 | 完全对齐 |
| 使用 uquic (refraction-networking/uquic) | 中 | 提供 Initial Packet 级别拟态 |
| 等待 quic-go 上游支持自定义 TP | 低 | 取决于上游进度 |

## 结论

当前实现通过 quic-go 原生 API 已对齐 Chrome 140+ 的核心 QUIC Transport Parameters。剩余差异需要 fork quic-go 或使用 uquic 等替代方案解决，记录为技术债务。
