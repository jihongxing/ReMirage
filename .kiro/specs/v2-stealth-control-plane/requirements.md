# 需求文档：V2 隐蔽控制面承载

## 简介

本文档定义 Mirage V2 隐蔽控制面承载层（Stealth Control Plane）的需求。该层负责将编排内核的控制语义（Persona Flip、Budget Sync、Survival Mode Change 等）通过两种互补方案安全传输：方案 A 利用 QUIC 原生多路复用在 AEAD 视界内部隐藏控制流；方案 B 利用 NPM 废包隐写术实现零特征畸变的侧信道控制。同时引入防御状态切换流量潮汐（马尔可夫链概率矩阵平滑过渡）和恒定时间锁（Spin-loop RTT 侧信道消除）两项抗 DPI 对抗措施。

前置依赖：Spec 5-3（控制语义层 ControlEvent / EventDispatcher）、Spec 6-1（观测与审计 AuditCollector / TimelineCollector）、Spec 4-2（PersonaEngine / PersonaMapUpdater）。

涉及模块：mirage-gateway（`pkg/gtunnel/` + `bpf/npm.c`）+ phantom-client。

## 术语表

- **Stealth_Control_Plane**：隐蔽控制面承载层，负责在 AEAD 视界内部传输控制语义
- **Shadow_Stream_Mux**：方案 A 的 QUIC 隐蔽流多路复用器，管理 Stream 0 控制流与 Stream 1~N 数据流
- **Stego_Encoder**：方案 B 的隐写编码器，将控制指令嵌入 NPM 废包
- **Stego_Decoder**：方案 B 的隐写解码器，从废包中提取控制指令
- **Transition_Smoother**：防御状态切换流量潮汐的平滑过渡控制器
- **Constant_Time_Lock**：恒定时间锁处理器，消除 RTT 侧信道
- **Control_Command**：Protobuf 序列化的控制指令（PersonaFlip / BudgetSync / SurvivalModeChange 等）
- **AEAD_Horizon**：AEAD 认证加密视界，指 QUIC/TLS 加密保护的内部空间
- **Dummy_Packet**：NPM 马尔可夫链生成的废包（用于流量形态伪装）
- **Stego_Payload**：隐写负载，格式为 `[HMAC_Tag + Ciphertext + Random_Padding]`
- **Markov_Matrix**：马尔可夫链概率转移矩阵，控制流量形态的统计特征
- **ControlEvent**：Spec 5-3 定义的控制事件对象
- **EventDispatcher**：Spec 5-3 定义的事件分发器
- **AuditCollector**：Spec 6-1 定义的审计采集器
- **TimelineCollector**：Spec 6-1 定义的时间线采集器
- **Stream_0**：QUIC 连接中约定的特权控制流（单向）
- **NPM_eBPF**：`bpf/npm.c` 中的 eBPF 程序，负责数据面废包生成与标记

## 需求

### 需求 1：QUIC 隐蔽流多路复用（方案 A）

**用户故事：** 作为网关运维者，我希望控制指令通过 QUIC 原生多路复用隐藏在加密流量中，使 DPI 无法区分控制流和数据流。

#### 验收标准

1. WHEN 一条 QUIC 连接建立完成，THE Shadow_Stream_Mux SHALL 在该连接上打开 Stream_0 作为特权控制流，Stream 1~N 作为用户代理数据流
2. WHEN 编排内核产生 Control_Command（PersonaFlip / BudgetSync / SurvivalModeChange），THE Shadow_Stream_Mux SHALL 将 Control_Command 序列化为 Protobuf 并写入 Stream_0
3. WHEN Stream_0 接收到数据，THE Shadow_Stream_Mux SHALL 反序列化 Protobuf 并将 Control_Command 转换为 ControlEvent 投递给 EventDispatcher
4. THE Shadow_Stream_Mux SHALL 确保 Stream_0 的写入操作不阻塞 Stream 1~N 的数据传输
5. IF Stream_0 写入失败，THEN THE Shadow_Stream_Mux SHALL 记录错误并通过 AuditCollector 上报，同时保持 Stream 1~N 数据传输不中断
6. THE Shadow_Stream_Mux SHALL 禁止在 QUIC 外层 UDP 报文头部或 TLS 握手阶段附加任何自定义字节

### 需求 2：Protobuf 控制指令序列化

**用户故事：** 作为开发者，我希望控制指令使用 Protobuf 序列化，使指令格式紧凑且可扩展。

#### 验收标准

1. THE Stealth_Control_Plane SHALL 定义 Protobuf 消息类型覆盖以下控制指令：PersonaFlip、BudgetSync、SurvivalModeChange、Rollback、SessionMigrate
2. WHEN 一个 Control_Command 被序列化为 Protobuf，THE Stealth_Control_Plane SHALL 在消息中包含 command_id（UUID）、command_type、epoch、timestamp 和 payload 字段
3. FOR ALL 合法的 Control_Command 对象，Protobuf 序列化后再反序列化 SHALL 产生等价对象（round-trip 属性）
4. WHEN 接收到未知 command_type 的 Protobuf 消息，THE Stealth_Control_Plane SHALL 丢弃该消息并记录警告日志

### 需求 3：废包隐写术（方案 B）

**用户故事：** 作为网关运维者，我希望控制指令能嵌入 NPM 废包中传输，实现零额外带宽特征的侧信道控制。

#### 验收标准

1. WHEN 控制层有紧急指令需要发送，THE Stego_Encoder SHALL 拦截 NPM 即将发出的下一个 Dummy_Packet
2. THE Stego_Encoder SHALL 将 Dummy_Packet 内容替换为 Stego_Payload，格式为 `[HMAC_Tag(32 bytes) + Ciphertext + Random_Padding]`
3. THE Stego_Encoder SHALL 确保 Stego_Payload 的总长度严格等于被替换的 Dummy_Packet 的原始长度
4. THE Stego_Encoder SHALL 使用 HMAC-SHA256 计算 HMAC_Tag，密钥为会话级共享密钥
5. THE Stego_Encoder SHALL 使用 ChaCha20-Poly1305 加密 Control_Command 生成 Ciphertext
6. THE Stego_Encoder SHALL 使用密码学安全随机数填充 Random_Padding 至目标长度
7. WHEN 接收端收到一个 Dummy_Packet，THE Stego_Decoder SHALL 先对前 32 字节执行 HMAC 校验
8. WHEN HMAC 校验命中，THE Stego_Decoder SHALL 解密 Ciphertext 提取 Control_Command 并投递给 EventDispatcher
9. WHEN HMAC 校验未命中，THE Stego_Decoder SHALL 将该包作为正常废包丢弃，不产生任何额外处理
10. IF Ciphertext 解密失败，THEN THE Stego_Decoder SHALL 静默丢弃该包并通过 AuditCollector 记录解密失败事件

### 需求 4：隐写长度对齐与统计不变量

**用户故事：** 作为安全工程师，我希望隐写操作不改变流量的统计特征，使 ML 分类器无法检测隐写存在。

#### 验收标准

1. FOR ALL 隐写替换操作，THE Stego_Encoder SHALL 保证替换后的包长度与原始 Dummy_Packet 长度的差值为 0 字节
2. IF Control_Command 加密后的 Ciphertext 长度加上 HMAC_Tag 长度超过 Dummy_Packet 原始长度，THEN THE Stego_Encoder SHALL 拒绝该次隐写并将指令排队等待下一个足够长的 Dummy_Packet
3. THE Stego_Encoder SHALL 维护隐写替换率统计，隐写包占总废包数量的比例不超过配置的上限（默认 5%）
4. THE Stego_Encoder SHALL 确保隐写操作不改变 NPM 废包的发送时序（IAT 分布不变）

### 需求 5：eBPF 数据面隐写标记

**用户故事：** 作为开发者，我希望 eBPF 数据面能标记可用于隐写的废包，使 Go 控制面能高效拦截。

#### 验收标准

1. THE NPM_eBPF SHALL 在 `bpf/npm.c` 中新增 `stego_ready_map`（BPF_MAP_TYPE_RINGBUF），用于通知 Go 控制面有废包可供隐写
2. WHEN NPM_eBPF 生成一个 Dummy_Packet，THE NPM_eBPF SHALL 将该包的长度和序列号写入 `stego_ready_map`
3. THE Go 控制面 SHALL 通过 Ring Buffer 读取 `stego_ready_map` 事件，决定是否对该废包执行隐写替换
4. WHEN Go 控制面决定执行隐写，THE Go 控制面 SHALL 通过 `stego_command_map`（BPF_MAP_TYPE_ARRAY）将 Stego_Payload 写入 eBPF Map
5. THE NPM_eBPF SHALL 在发送废包前检查 `stego_command_map`，如果有待发送的 Stego_Payload 则替换废包内容

### 需求 6：防御状态切换流量潮汐

**用户故事：** 作为安全工程师，我希望 Persona 切换时流量形态平滑过渡，防止 ML 分类器捕捉到状态突变。

#### 验收标准

1. WHEN 编排内核下发 Persona 切换指令，THE Transition_Smoother SHALL 从指令中读取 Transition_Duration（过渡时间，单位毫秒）
2. THE Transition_Smoother SHALL 禁止原子性生硬切换包长/时序模型，所有切换必须经过平滑过渡
3. WHILE 过渡期间，THE Transition_Smoother SHALL 通过线性插值计算当前时刻的马尔可夫链概率矩阵：`M(t) = (1 - α(t)) * M_old + α(t) * M_new`，其中 `α(t) = elapsed / Transition_Duration`
4. WHILE 过渡期间，THE Transition_Smoother SHALL 同步插值包大小分布参数（均值和标准差）：`mean(t) = (1 - α(t)) * mean_old + α(t) * mean_new`
5. WHEN Transition_Duration 到期，THE Transition_Smoother SHALL 完全切换到新模型并通过 TimelineCollector 记录过渡完成事件
6. IF 过渡期间收到新的切换指令，THEN THE Transition_Smoother SHALL 以当前插值状态作为新的起点开始新的过渡
7. IF Transition_Duration 为 0 或未指定，THEN THE Transition_Smoother SHALL 使用默认过渡时间 3000 毫秒

### 需求 7：恒定时间锁与 RTT 侧信道消除

**用户故事：** 作为安全工程师，我希望处理控制流和数据流的耗时完全相同，消除基于 RTT 差异的侧信道。

#### 验收标准

1. THE Constant_Time_Lock SHALL 将 Stream_0 控制指令的处理耗时对齐到与普通数据包相同的恒定时间槽（ConstantTimeSlotNs）
2. THE Constant_Time_Lock SHALL 将隐写废包的 HMAC 校验 + 解密处理耗时对齐到与普通废包丢弃相同的恒定时间槽
3. THE Constant_Time_Lock SHALL 使用 Spin-loop（busy-wait）实现纳秒级精度的时间对齐，禁止使用 time.Sleep
4. THE Constant_Time_Lock SHALL 对 HMAC 校验未命中的废包执行等价计算量的伪处理（fake crypto work），使 CPU 耗时与校验命中时一致
5. IF 实际处理耗时超过恒定时间槽，THEN THE Constant_Time_Lock SHALL 立即返回结果并通过 AuditCollector 记录超时事件

### 需求 8：控制面承载通道选择

**用户故事：** 作为网关运维者，我希望系统能根据网络条件自动选择最优的控制面承载通道。

#### 验收标准

1. WHILE QUIC 连接正常且 Stream_0 可用，THE Stealth_Control_Plane SHALL 优先使用方案 A（QUIC 隐蔽流）传输控制指令
2. WHEN Stream_0 不可用或 QUIC 连接断开，THE Stealth_Control_Plane SHALL 自动降级到方案 B（废包隐写术）传输控制指令
3. WHEN 方案 A 恢复可用，THE Stealth_Control_Plane SHALL 自动切回方案 A
4. THE Stealth_Control_Plane SHALL 通过 TimelineCollector 记录每次通道切换事件（包含切换原因和时间戳）
5. IF 方案 A 和方案 B 均不可用，THEN THE Stealth_Control_Plane SHALL 将控制指令排入本地队列，队列容量上限为 64 条指令

### 需求 9：控制指令可靠性

**用户故事：** 作为开发者，我希望控制指令传输具备可靠性保证，确保关键指令不丢失。

#### 验收标准

1. WHEN 一条 Control_Command 通过方案 A 发送，THE Stealth_Control_Plane SHALL 依赖 QUIC 的可靠传输保证（重传机制）
2. WHEN 一条 Control_Command 通过方案 B 发送，THE Stealth_Control_Plane SHALL 在发送后启动确认超时计时器（默认 5000 毫秒）
3. IF 方案 B 的确认超时，THEN THE Stealth_Control_Plane SHALL 重新排队该指令并在下一个可用废包中重发，最多重试 3 次
4. IF 重试 3 次仍未确认，THEN THE Stealth_Control_Plane SHALL 通过 AuditCollector 记录指令丢失事件并触发 EventSurvivalModeChange 事件
5. THE Stealth_Control_Plane SHALL 通过 command_id 实现指令去重，接收端对重复 command_id 的指令执行幂等处理

### 需求 10：安全约束

**用户故事：** 作为安全工程师，我希望隐蔽控制面的所有操作严格遵守密码学安全规范。

#### 验收标准

1. THE Stealth_Control_Plane SHALL 确保所有控制语义发生在 AEAD_Horizon 内部
2. THE Stealth_Control_Plane SHALL 禁止在外层 TCP/UDP 报文头部或 QUIC/TLS 握手阶段附加任何自定义字节
3. THE Stego_Encoder SHALL 使用恒定时间比较（constant-time compare）执行所有密码学校验操作
4. THE Stego_Decoder SHALL 使用恒定时间比较执行 HMAC 校验
5. THE Stealth_Control_Plane SHALL 为每条 QUIC 连接维护独立的会话密钥，密钥通过 QUIC 握手的 TLS 1.3 密钥导出
6. IF 会话密钥泄露或过期，THEN THE Stealth_Control_Plane SHALL 触发密钥轮换并通过 AuditCollector 记录密钥轮换事件
