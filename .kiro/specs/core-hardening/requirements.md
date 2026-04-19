# 需求文档：Phase 3 — 核心链路加固 + 极简控制台

## 简介

本阶段为 Mirage Project 四阶段实施的第三阶段，目标是在 Phase 2 的 Mirage-OS 大脑 MVP 基础上实现三大加固能力：
1. **Raft 高可用集群**：在 gateway-bridge（Go 服务）中集成 hashicorp/raft，实现 3-of-5 节点一致性复制（配额/黑名单/策略状态），Leader 宕机自动选举，服务不中断
2. **Shamir 密钥安全**：实现 GF(256) 有限域上的 3-of-5 秘密分享，主密钥热驻内存（mlock + 自动清零），冷启动时从 Raft 集群收集份额恢复
3. **极简管理控制台**：React + TailwindCSS 纯表格界面，调用 Phase 2 的 NestJS REST API，5 秒轮询刷新，砍掉所有视觉冗余（Three.js/图表/WebSocket）
4. **核心链路集成测试**：验证生死裁决、全局免疫、域名转生、节点自毁、Raft 故障转移五条核心链路

## 术语表

- **Raft_Cluster**：基于 hashicorp/raft 的分布式一致性集群，部署于 gateway-bridge 进程内
- **Raft_FSM**：Raft 有限状态机，负责配额变更、黑名单更新、策略变更的一致性复制
- **Raft_Leader**：Raft 集群中的主节点，负责接受写请求并复制到 Follower
- **Raft_Follower**：Raft 集群中的从节点，接收 Leader 复制的日志并应用到本地状态机
- **Shamir_Engine**：GF(256) 有限域上的 Shamir 秘密分享引擎，将主密钥拆分为 5 个份额，任意 3 个可恢复
- **Hot_Key**：热密钥管理器，主密钥常驻内存（mlock 锁定），提供零延迟加密/解密操作
- **GF_256**：伽罗瓦域 GF(2^8)，Shamir 秘密分享的数学基础，所有运算在 256 个元素的有限域上进行
- **Share**：Shamir 秘密分享的份额，包含 x 坐标和 y 值数组
- **Console_App**：React + TailwindCSS 极简管理控制台，部署于 mirage-os/web/
- **DataTable**：通用数据表格组件，支持排序、分页、状态指示
- **StatusIndicator**：状态灯组件（🟢在线/🟡降级/🔴离线）
- **ControlPanel**：控制面板组件，包含下拉框、开关、按钮
- **Polling_Refresh**：5 秒间隔的 HTTP 轮询刷新机制，替代 WebSocket 实时推送
- **Integration_Test**：核心链路集成测试，验证 Gateway↔OS 完整闭环
- **TC_ACT_STOLEN**：eBPF 内核态流量阻断动作
- **Gateway_Bridge**：Phase 2 已实现的 Go gRPC 服务，本阶段在其中扩展 Raft 和 Shamir 模块
- **API_Server**：Phase 2 已实现的 NestJS REST API 服务，控制台通过其 API 获取数据

## 需求

### 需求 1：Raft 集群管理

**用户故事：** 作为系统运维人员，我需要 gateway-bridge 支持 Raft 集群模式，以便在 Leader 节点宕机时自动选举新 Leader，保证服务不中断。

#### 验收标准

1. THE Raft_Cluster SHALL 使用 hashicorp/raft 库初始化集群，支持 3-of-5 节点配置（3 节点投票，2 节点冷备）
2. WHEN Raft_Cluster 启动时，THE Raft_Cluster SHALL 从配置文件读取集群节点列表（node_id、地址、是否投票节点），并加入或引导集群
3. WHEN Raft_Leader 宕机或网络分区时，THE Raft_Cluster SHALL 在 10 秒内完成自动选举，新 Leader 接管所有写操作
4. THE Raft_Cluster SHALL 提供 IsLeader 方法，返回当前节点是否为 Leader
5. THE Raft_Cluster SHALL 提供 Apply 方法，将状态变更提交到 Raft 日志并复制到多数节点
6. IF Raft_Cluster 无法在 30 秒内达成多数共识，THEN THE Raft_Cluster SHALL 返回超时错误并记录告警日志
7. THE Raft_Cluster SHALL 提供 GetLeaderAddr 方法，返回当前 Leader 的网络地址

### 需求 2：Raft 状态机

**用户故事：** 作为系统架构师，我需要 Raft 状态机复制配额变更、黑名单更新和策略变更，以便所有节点保持数据一致性。

#### 验收标准

1. THE Raft_FSM SHALL 实现 hashicorp/raft.FSM 接口（Apply、Snapshot、Restore 三个方法）
2. WHEN Raft_FSM 收到配额变更命令时，THE Raft_FSM SHALL 更新本地配额缓存，命令格式包含 user_id 和 new_remaining_quota
3. WHEN Raft_FSM 收到黑名单更新命令时，THE Raft_FSM SHALL 更新本地黑名单缓存，命令格式包含 source_ip、is_banned 和 expire_at
4. WHEN Raft_FSM 收到策略变更命令时，THE Raft_FSM SHALL 更新本地策略缓存，命令格式包含 cell_id 和策略参数（defense_level、jitter_mean_us、noise_intensity、padding_rate、template_id）
5. THE Raft_FSM SHALL 支持快照（Snapshot），将当前配额、黑名单、策略状态序列化为 JSON 格式
6. THE Raft_FSM SHALL 支持恢复（Restore），从 JSON 快照反序列化恢复配额、黑名单、策略状态
7. FOR ALL 有效的状态变更命令，Apply 后再 Snapshot 再 Restore SHALL 产生与 Apply 后等价的状态（快照往返一致性）

### 需求 3：Shamir 秘密分享

**用户故事：** 作为安全工程师，我需要将主密钥拆分为多个份额分布存储，以便任意单节点被查封时无法恢复完整密钥。

#### 验收标准

1. THE Shamir_Engine SHALL 在 GF(256) 有限域上实现 Shamir 秘密分享算法，支持将任意长度的字节序列拆分为 N 个份额
2. THE Shamir_Engine SHALL 支持 3-of-5 阈值配置（threshold=3，total=5），任意 3 个份额可恢复原始密钥
3. FOR ALL 有效的密钥字节序列和任意 3 个份额的组合，Shamir_Engine.Split 后选取任意 3 个份额调用 Shamir_Engine.Combine SHALL 恢复出与原始密钥相同的字节序列（往返一致性）
4. IF 提供的份额数量少于 threshold，THEN THE Shamir_Engine.Combine SHALL 返回错误
5. IF 提供的份额中包含重复的 x 坐标，THEN THE Shamir_Engine.Combine SHALL 返回错误
6. THE Shamir_Engine SHALL 使用 crypto/rand 生成随机多项式系数，确保每次 Split 产生不同的份额

### 需求 4：热密钥管理

**用户故事：** 作为安全工程师，我需要主密钥在正常运行时常驻内存且受保护，以便 G-Switch 转生等操作实现零延迟加密。

#### 验收标准

1. THE Hot_Key SHALL 使用 syscall.Mlock 锁定密钥内存页，防止操作系统将密钥交换到磁盘
2. WHEN Hot_Key 被停用或进程退出时，THE Hot_Key SHALL 使用逐字节清零方式擦除内存中的密钥数据
3. THE Hot_Key SHALL 提供 Encrypt 方法（AES-256-GCM 加密）和 Decrypt 方法，使用常驻内存的主密钥进行零延迟操作
4. WHEN 节点冷启动时（热密钥未激活），THE Hot_Key SHALL 从 Raft 集群收集 3 个 Shamir 份额，调用 Shamir_Engine.Combine 恢复主密钥并激活热密钥
5. THE Hot_Key SHALL 提供 IsActive 方法，返回热密钥是否已激活
6. IF Mlock 调用失败（权限不足），THEN THE Hot_Key SHALL 记录告警日志并继续运行（降级模式，不锁定内存）
7. FOR ALL 有效的明文数据，Hot_Key.Encrypt 后调用 Hot_Key.Decrypt SHALL 恢复出与原始明文相同的数据（加密往返一致性）

### 需求 5：控制台应用框架

**用户故事：** 作为系统管理员，我需要一个极简的 Web 管理控制台，以便通过浏览器查看系统状态和执行管理操作。

#### 验收标准

1. THE Console_App SHALL 使用 React 18 + TypeScript + TailwindCSS + Vite 构建，部署于 mirage-os/web/ 目录
2. THE Console_App SHALL 包含以下页面路由：Dashboard（/）、Gateways（/gateways）、Cells（/cells）、Billing（/billing）、Threats（/threats）、Strategy（/strategy）
3. THE Console_App SHALL 包含侧边栏导航，显示所有页面链接和当前活跃页面高亮
4. THE Console_App SHALL 通过 useApi hook 调用 Phase 2 的 NestJS REST API（基础 URL 可配置）
5. THE Console_App SHALL 使用 5 秒间隔的 HTTP 轮询刷新数据，替代 WebSocket 实时推送

### 需求 6：Dashboard 页面

**用户故事：** 作为系统管理员，我需要在 Dashboard 页面看到系统核心指标的数字概览，以便快速了解系统运行状态。

#### 验收标准

1. THE Dashboard SHALL 显示以下数字指标卡片：在线 Gateway 数量、总蜂窝数量、活跃用户数量、已封禁威胁 IP 数量
2. THE Dashboard SHALL 为每个指标卡片显示 StatusIndicator 状态灯（全部在线=🟢、部分降级=🟡、全部离线=🔴）
3. THE Dashboard SHALL 每 5 秒轮询 API 刷新指标数据

### 需求 7：Gateways 页面

**用户故事：** 作为系统管理员，我需要查看所有 Gateway 节点的状态列表，以便监控节点运行情况。

#### 验收标准

1. THE Gateways 页面 SHALL 使用 DataTable 组件显示 Gateway 列表，列包含：IP 地址、状态（StatusIndicator）、最后心跳时间、活跃连接数、内存使用量、威胁等级
2. THE Gateways 页面 SHALL 支持按状态（ONLINE/DEGRADED/OFFLINE）过滤
3. THE Gateways 页面 SHALL 每 5 秒轮询 GET /api/gateways 刷新数据

### 需求 8：Cells 页面

**用户故事：** 作为系统管理员，我需要查看所有蜂窝的状态列表，以便监控蜂窝资源使用情况。

#### 验收标准

1. THE Cells 页面 SHALL 使用 DataTable 组件显示蜂窝列表，列包含：名称、区域、级别、用户数/最大用户数、Gateway 数、健康状态（StatusIndicator）
2. THE Cells 页面 SHALL 每 5 秒轮询 GET /api/cells 刷新数据

### 需求 9：Billing 页面

**用户故事：** 作为系统管理员，我需要查看计费流水和配额信息，以便管理用户的流量消费。

#### 验收标准

1. THE Billing 页面 SHALL 使用 DataTable 组件显示计费流水列表，列包含：时间、用户 ID、业务流量、防御流量、业务费用、防御费用、总费用
2. THE Billing 页面 SHALL 显示当前用户的配额余额、总充值、总消费
3. THE Billing 页面 SHALL 包含充值按钮，点击后弹出充值表单（金额输入 + 确认按钮），调用 POST /api/billing/recharge
4. THE Billing 页面 SHALL 每 5 秒轮询刷新流水和配额数据

### 需求 10：Threats 页面

**用户故事：** 作为系统管理员，我需要查看威胁情报列表，以便了解全网威胁态势和封禁状态。

#### 验收标准

1. THE Threats 页面 SHALL 使用 DataTable 组件显示威胁情报列表，列包含：源 IP、威胁类型、严重程度、命中次数、封禁状态（StatusIndicator）、最后发现时间
2. THE Threats 页面 SHALL 支持按威胁类型和封禁状态过滤
3. THE Threats 页面 SHALL 每 5 秒轮询 GET /api/threats 刷新数据

### 需求 11：Strategy 页面

**用户故事：** 作为系统管理员，我需要查看和修改蜂窝的防御策略，以便根据威胁态势调整防御强度。

#### 验收标准

1. THE Strategy 页面 SHALL 显示蜂窝选择下拉框，选择蜂窝后显示当前防御策略参数
2. THE Strategy 页面 SHALL 包含防御等级下拉框（Level 0-4）和拟态模板下拉框（Zoom/Chrome/Teams）
3. THE Strategy 页面 SHALL 包含"应用策略"按钮，点击后调用 API 将策略变更提交到选定蜂窝
4. WHEN 策略应用成功时，THE Strategy 页面 SHALL 显示成功提示消息

### 需求 12：通用组件

**用户故事：** 作为前端开发者，我需要可复用的通用组件，以便所有页面保持一致的视觉风格和交互模式。

#### 验收标准

1. THE StatusIndicator 组件 SHALL 接受 status 属性（online/degraded/offline），分别渲染 🟢/🟡/🔴 状态灯和对应文字标签
2. THE DataTable 组件 SHALL 接受 columns 和 data 属性，渲染表头和数据行，支持空数据状态提示
3. THE ControlPanel 组件 SHALL 支持渲染下拉框（select）、开关（toggle）和按钮（button）三种控件类型

### 需求 13：集成测试 — 生死裁决

**用户故事：** 作为质量工程师，我需要验证配额归零到用户断流的完整链路，以便确认生死裁决机制正确运作。

#### 验收标准

1. WHEN 用户 remaining_quota 通过流量结算降至 0 时，THE Integration_Test SHALL 验证 gateway-bridge 的 SyncHeartbeat 响应中 remaining_quota 为 0
2. THE Integration_Test SHALL 验证 Gateway 收到 remaining_quota=0 后写入 eBPF quota_map 值为 0，触发 TC_ACT_STOLEN

### 需求 14：集成测试 — 全局免疫

**用户故事：** 作为质量工程师，我需要验证威胁 IP 从命中到全网封禁的完整链路，以便确认全局免疫机制正确运作。

#### 验收标准

1. WHEN 同一源 IP 被上报 100 次后，THE Integration_Test SHALL 验证 Intel_Distributor 将该 IP 标记为 is_banned=true
2. THE Integration_Test SHALL 验证封禁事件通过 Redis Pub/Sub 发布，并通过 PushBlacklist 下发到所有在线 Gateway

### 需求 15：集成测试 — 域名转生

**用户故事：** 作为质量工程师，我需要验证 G-Switch 域名转生的完整链路，以便确认域名切换对用户透明。

#### 验收标准

1. WHEN G-Switch 转生指令下发时，THE Integration_Test SHALL 验证 Gateway 收到 PushReincarnation 指令并包含新域名和新 IP
2. THE Integration_Test SHALL 验证转生指令的 deadline_seconds 大于 0

### 需求 16：集成测试 — 节点自毁

**用户故事：** 作为质量工程师，我需要验证心跳超时后节点自毁的完整链路，以便确认敏感数据不会残留。

#### 验收标准

1. WHEN Gateway 心跳超时 300 秒后，THE Integration_Test SHALL 验证 Gateway 触发自毁流程
2. THE Integration_Test SHALL 验证自毁流程包含 eBPF Map 清空和内存擦除步骤

### 需求 17：集成测试 — Raft 故障转移

**用户故事：** 作为质量工程师，我需要验证 Raft Leader 宕机后自动选举的完整链路，以便确认高可用机制正确运作。

#### 验收标准

1. WHEN Raft_Leader 节点停止后，THE Integration_Test SHALL 验证剩余节点在 10 秒内选举出新 Leader
2. THE Integration_Test SHALL 验证新 Leader 选举后，SyncHeartbeat 和 ReportTraffic 请求可正常处理
