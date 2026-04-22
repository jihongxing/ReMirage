# 需求文档：V2 Persona Manifest 与原子切换

## 简介

Mirage V2 编排内核的统一画像引擎。将 B-DNA（握手指纹）、NPM（包长伪装）、Jitter-Lite（时域扰动）、VPC（背景噪声）四类协议参数收敛为不可拆分的 Persona Manifest 快照，通过 Shadow/Active 双区模型实现原子切换，禁止对单一协议参数的独立修改。Persona Engine 是 State Commit Engine（Spec 4-3）和 Survival Orchestrator（Spec 5-2）的前置依赖。

## 术语表

- **Persona_Engine**：画像引擎，mirage-gateway 中 `pkg/orchestrator/` 模块的子组件，负责 Persona 的创建、校验、选择、切换和回滚
- **Persona_Manifest**：画像清单，一个不可拆分的完整快照，包含握手画像、包长画像、时域画像、背景画像等全部协议参数的引用
- **Persona_ID**：画像唯一标识符，格式为 UUID v4 字符串
- **Persona_Version**：画像版本号，uint64 类型，同一 Persona_ID 下严格递增
- **Persona_Epoch**：画像纪元，与 Control_State 的 Epoch 对齐，标识该 Persona 生效时的全局逻辑时钟
- **Persona_Checksum**：画像校验和，基于 Persona_Manifest 全部 profile_id 字段计算的 SHA-256 哈希，用于完整性校验
- **Handshake_Profile**：握手画像，对应 B-DNA 协议参数（TLS 指纹、TCP Window Size、JA4 特征等）
- **Packet_Shape_Profile**：包长画像，对应 NPM 协议参数（Padding 策略、目标 MTU、突发模式等）
- **Timing_Profile**：时域画像，对应 Jitter-Lite 协议参数（IAT 均值、IAT 标准差、模板 ID 等）
- **Background_Profile**：背景画像，对应 VPC 协议参数（光缆抖动、路由器延迟、噪声强度、丢包率等）
- **Shadow_Slot**：影子区，eBPF Map 中用于预写入待切换 Persona 参数的存储区域，尚未生效
- **Active_Slot**：活跃区，eBPF Map 中当前正在被数据面使用的 Persona 参数存储区域
- **Atomic_Flip**：原子翻转，通过单次 eBPF Map 写入将 Active 指针从旧 Slot 切换到新 Slot 的操作
- **Persona_Lifecycle**：画像生命周期，包含 Prepared / ShadowLoaded / Active / Cooling / Retired 五个阶段
- **Service_Class**：服务等级（Standard / Platinum / Diamond），由 Spec 4-1 定义
- **Survival_Mode**：生存姿态（Normal / LowNoise / Hardened / Degraded / Escape / LastResort），由 Spec 4-1 定义枚举
- **Link_State**：链路状态，由 Spec 4-1 定义
- **Session_State**：会话状态，由 Spec 4-1 定义，其 current_persona_id 字段引用 Persona_Manifest
- **Control_State**：控制状态，由 Spec 4-1 定义，其 persona_version 字段跟踪当前画像版本
- **eBPF_Map**：内核态键值存储，Go 控制面通过 cilium/ebpf 库读写，C 数据面直接访问
- **Persona_Map_Updater**：画像 Map 更新器，Go 控制面组件，负责将 Persona 参数批量写入 eBPF Map

## 需求

### 需求 1：Persona Manifest 结构定义

**用户故事：** 作为编排器开发者，我需要一个不可拆分的统一画像结构，以便将 B-DNA / NPM / Jitter-Lite / VPC 四类协议参数收敛为完整快照，禁止对单一协议参数的独立修改。

#### 验收标准

1. THE Persona_Engine SHALL 定义 Persona_Manifest 结构体，包含以下字段：persona_id（string，UUID v4 唯一标识）、version（uint64，版本号）、epoch（uint64，对齐 Control_State 的 Epoch）、checksum（string，SHA-256 校验和）、handshake_profile_id（string，握手画像引用）、packet_shape_profile_id（string，包长画像引用）、timing_profile_id（string，时域画像引用）、background_profile_id（string，背景画像引用）、mtu_profile_id（string，MTU 画像引用）、fec_profile_id（string，FEC 画像引用）、lifecycle_policy_id（string，生命周期策略引用）、lifecycle（Persona_Lifecycle 枚举）、created_at（时间戳）
2. THE Persona_Engine SHALL 确保每个 Persona_Manifest 的 handshake_profile_id、packet_shape_profile_id、timing_profile_id、background_profile_id 四个字段全部为非空字符串，缺少任一字段的 Persona_Manifest 视为无效
3. THE Persona_Engine SHALL 基于 Persona_Manifest 的 handshake_profile_id、packet_shape_profile_id、timing_profile_id、background_profile_id、mtu_profile_id、fec_profile_id 六个字段的拼接值计算 SHA-256 哈希作为 checksum
4. IF 创建 Persona_Manifest 时任一必填 profile_id 为空字符串，THEN THE Persona_Engine SHALL 拒绝创建并返回包含缺失字段名称的错误信息

### 需求 2：版本与 Epoch 规则

**用户故事：** 作为编排器开发者，我需要 Persona 版本号与全局 Epoch 对齐，以便在崩溃恢复时能判定哪个 Persona 版本是已提交的稳定版本。

#### 验收标准

1. WHEN 创建新的 Persona_Manifest 时，THE Persona_Engine SHALL 确保 version 严格大于同一 persona_id 下已存在的最大 version
2. WHEN 创建新的 Persona_Manifest 时，THE Persona_Engine SHALL 将 epoch 设置为当前 Control_State 的 Epoch 值
3. THE Persona_Engine SHALL 禁止修改已创建的 Persona_Manifest 的 version、epoch、checksum 字段，这三个字段在创建后不可变
4. IF 创建 Persona_Manifest 时提供的 version 小于或等于同一 persona_id 下已存在的最大 version，THEN THE Persona_Engine SHALL 拒绝创建并返回版本冲突错误

### 需求 3：Persona 生命周期

**用户故事：** 作为编排器开发者，我需要 Persona 有明确的生命周期状态机，以便追踪每个画像从准备到退役的完整过程。

#### 验收标准

1. THE Persona_Engine SHALL 定义 Persona_Lifecycle 枚举，包含五个阶段：Prepared、ShadowLoaded、Active、Cooling、Retired
2. WHEN Persona_Lifecycle 从一个阶段转换到另一个阶段时，THE Persona_Engine SHALL 校验转换合法性，仅允许以下转换路径：Prepared→ShadowLoaded、ShadowLoaded→Active、ShadowLoaded→Retired（校验失败回退）、Active→Cooling、Cooling→Retired
3. IF Persona_Lifecycle 转换请求不在合法路径中，THEN THE Persona_Engine SHALL 返回包含当前阶段和目标阶段的错误信息，拒绝该转换
4. WHEN Persona_Lifecycle 变为 Active 时，THE Persona_Engine SHALL 确保同一 Session 下只有一个 Persona_Manifest 处于 Active 阶段，将前一个 Active 的 Persona_Manifest 转换为 Cooling
5. WHEN Persona_Lifecycle 变为 Retired 时，THE Persona_Engine SHALL 保留该 Persona_Manifest 的记录用于审计，标记为不可再激活

### 需求 4：Shadow / Active 双区模型

**用户故事：** 作为编排器开发者，我需要 eBPF Map 中存在 Shadow 和 Active 两个存储区域，以便实现先写后切的原子切换语义，避免数据面在切换过程中读到不一致的参数。

#### 验收标准

1. THE Persona_Map_Updater SHALL 在 eBPF Map 中维护两个 Slot（Slot 0 和 Slot 1），通过一个 active_slot_map（key=0, value=0 或 1）标识当前活跃 Slot
2. WHEN 准备切换 Persona 时，THE Persona_Map_Updater SHALL 将新 Persona 的全部参数写入当前非活跃的 Shadow_Slot，写入过程中 Active_Slot 的参数保持不变
3. THE Persona_Map_Updater SHALL 在写入 Shadow_Slot 完成后，从 Shadow_Slot 回读全部参数并与预期值逐字段比对，确认写入完整性
4. IF Shadow_Slot 回读校验发现任一字段与预期值不一致，THEN THE Persona_Map_Updater SHALL 放弃本次切换，保持 Active_Slot 不变，返回校验失败错误
5. WHEN Shadow_Slot 校验通过后，THE Persona_Map_Updater SHALL 通过单次 eBPF Map Put 操作将 active_slot_map 的值从旧 Slot 编号切换为新 Slot 编号，完成 Atomic_Flip

### 需求 5：原子切换语义

**用户故事：** 作为编排器开发者，我需要 Persona 切换是原子的且可回滚的，以便在切换失败时能恢复到上一个稳定版本，保证数据面参数一致性。

#### 验收标准

1. THE Persona_Engine SHALL 执行以下切换流程：校验新 Persona_Manifest 完整性 → 将参数写入 Shadow_Slot → 回读校验 Shadow_Slot → 执行 Atomic_Flip → 更新 Session_State 的 current_persona_id → 更新 Control_State 的 persona_version
2. WHEN Atomic_Flip 执行成功后，THE Persona_Engine SHALL 将前一个 Active 的 Persona_Manifest 保留在旧 Slot 中，lifecycle 转换为 Cooling，作为回滚目标
3. IF 切换流程中任一步骤失败，THEN THE Persona_Engine SHALL 保持 active_slot_map 指向旧 Slot 不变，当前 Active 的 Persona_Manifest 继续生效
4. THE Persona_Engine SHALL 提供 Rollback 操作，将 active_slot_map 切换回上一个 Cooling 状态的 Persona 所在的 Slot，恢复到上一个稳定版本
5. THE Persona_Engine SHALL 确保同一时刻只允许一个 Persona 切换操作在执行，通过互斥锁防止并发切换导致 Shadow_Slot 数据被覆盖
6. WHEN 执行 Rollback 时，THE Persona_Engine SHALL 将回滚目标 Persona 的 lifecycle 从 Cooling 转换为 Active，将当前失败的 Persona 的 lifecycle 转换为 Retired

### 需求 6：Persona 参数收敛到 eBPF Map

**用户故事：** 作为编排器开发者，我需要将 Persona 的四类画像参数批量写入对应的 eBPF Map，以便 C 数据面能读取到完整一致的协议参数。

#### 验收标准

1. THE Persona_Map_Updater SHALL 将 Handshake_Profile 参数写入 dna_template_map，写入格式与现有 DNATemplateEntry 结构体兼容（TargetIATMu、TargetIATSigma、PaddingStrategy、TargetMTU、BurstSize、BurstInterval）
2. THE Persona_Map_Updater SHALL 将 Timing_Profile 参数写入 jitter_config_map，写入格式与现有 JitterConfig 结构体兼容（Enabled、MeanIATUs、StddevIATUs、TemplateID）
3. THE Persona_Map_Updater SHALL 将 Background_Profile 参数写入 vpc_config_map，写入格式与现有 VPCConfig 结构体兼容（Enabled、FiberJitterUs、RouterDelayUs、NoiseIntensity）
4. THE Persona_Map_Updater SHALL 将 Packet_Shape_Profile 参数写入 npm_config_map，写入格式与现有 ConfigMap 的 PaddingRate 字段兼容
5. WHEN 写入 Shadow_Slot 时，THE Persona_Map_Updater SHALL 使用 Slot 编号作为 eBPF Map 的 key 偏移量（Slot 0 使用 key=0，Slot 1 使用 key=1），实现双区隔离
6. IF 任一 eBPF Map 写入失败，THEN THE Persona_Map_Updater SHALL 停止后续写入，返回包含失败 Map 名称的错误信息，Shadow_Slot 中已写入的部分数据视为无效

### 需求 7：Persona 选择约束

**用户故事：** 作为编排器开发者，我需要 Persona 选择受到 Session 服务等级、Link 健康状态和 Survival Mode 三重约束，以便选出的 Persona 与当前系统状态匹配。

#### 验收标准

1. THE Persona_Engine SHALL 定义 Persona 选择接口，接受 Session_State、Link_State、Survival_Mode 三个输入参数，返回一个匹配的 Persona_Manifest 或选择失败错误
2. WHEN Service_Class 为 Standard 时，THE Persona_Engine SHALL 仅允许选择标记为 Standard 兼容的 Persona_Manifest
3. WHEN Survival_Mode 为 Hardened 或更高级别（Escape / LastResort）时，THE Persona_Engine SHALL 优先选择防御强度更高的 Persona_Manifest
4. WHEN Link_State 的 health_score 低于 50 时，THE Persona_Engine SHALL 优先选择资源消耗更低的 Persona_Manifest，避免在退化链路上使用高开销画像
5. IF 没有任何 Persona_Manifest 满足当前三重约束条件，THEN THE Persona_Engine SHALL 返回选择失败错误，包含不满足的约束条件描述

### 需求 8：回滚能力

**用户故事：** 作为编排器开发者，我需要系统始终保留上一个稳定版本的 Persona，以便在切换失败或异常检测时能快速回滚到已知良好状态。

#### 验收标准

1. THE Persona_Engine SHALL 在每次成功切换后，将前一个 Active 的 Persona_Manifest 标记为 Cooling 状态，保留在 eBPF Map 的旧 Slot 中
2. THE Persona_Engine SHALL 确保任意时刻最多存在一个 Cooling 状态的 Persona_Manifest（每个 Session 维度）
3. WHEN 执行回滚时，THE Persona_Engine SHALL 通过单次 active_slot_map 写入将数据面切换回 Cooling 状态的 Persona 所在 Slot
4. WHEN 执行回滚时，THE Persona_Engine SHALL 更新 Session_State 的 current_persona_id 为回滚目标的 persona_id
5. IF 不存在 Cooling 状态的 Persona_Manifest（首次启动或 Cooling 已 Retired），THEN THE Persona_Engine SHALL 拒绝回滚操作并返回无可用回滚目标的错误

### 需求 9：Persona 持久化

**用户故事：** 作为运维人员，我需要 Persona Manifest 持久化到数据库，以便系统重启后能恢复画像状态，支持画像版本历史查询和审计。

#### 验收标准

1. THE Persona_Engine SHALL 在 mirage-os 数据库中创建 `persona_manifests` 表，包含 Persona_Manifest 的所有字段，以 persona_id + version 为联合主键
2. THE Persona_Engine SHALL 为 `persona_manifests` 表的 lifecycle 字段添加 CHECK 约束，仅允许 Persona_Lifecycle 枚举中定义的值
3. THE Persona_Engine SHALL 为 `persona_manifests` 表的 epoch 字段建立索引，支持按 Epoch 范围查询
4. WHEN Persona_Lifecycle 发生转换时，THE Persona_Engine SHALL 在同一数据库事务中更新 lifecycle 字段
5. THE Persona_Engine SHALL 使用 GORM AutoMigrate 机制，将 `persona_manifests` 表纳入现有的 AutoMigrate 函数

### 需求 10：Persona 查询 API

**用户故事：** 作为编排器和运维工具的调用方，我需要通过 HTTP API 查询 Persona 状态，以便了解当前画像配置和历史版本。

#### 验收标准

1. THE Persona_Engine SHALL 提供 `GET /api/v2/personas/{persona_id}` 端点，返回指定 persona_id 的最新版本 Persona_Manifest
2. THE Persona_Engine SHALL 提供 `GET /api/v2/personas/{persona_id}/versions` 端点，返回指定 persona_id 的全部版本列表，按 version 降序排列
3. THE Persona_Engine SHALL 提供 `GET /api/v2/sessions/{session_id}/persona` 端点，返回指定 Session 当前关联的 Active 状态 Persona_Manifest
4. IF 查询的资源不存在，THEN THE Persona_Engine SHALL 返回 HTTP 404 状态码和包含资源类型与标识的错误消息
5. THE Persona_Engine SHALL 以 JSON 格式返回所有响应，时间戳字段使用 RFC 3339 格式
