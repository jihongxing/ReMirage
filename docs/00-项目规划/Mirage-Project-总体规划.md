---
Status: input
Target Truth: docs/governance/boundaries/product-scope.md
Migration: 产品边界与商业模型结论已迁移到 product-scope.md，本文保留为历史规划输入材料
---

# Mirage Project 总体实施规划（剃刀版）

## 项目定位

高级流量隐匿与反审计系统。二元架构：Gateway（手脚）+ OS（大脑）。
目标：200 高净值用户，月营收 $300,000。
工程约束：单兵全栈交付，10-12 周打通商业闭环。

---

## 设计原则

1. **极致 MVP**：只做打通"感知-防御-逃逸-计费"闭环的最小功能集
2. **残酷裁剪**：砍掉所有不直接产生商业价值的视觉冗余和技术炫技
3. **栈位分层**：内核态/gRPC 用 Go+C 保性能，上层业务 CRUD 用 NestJS+Prisma 抢速度
4. **透明网关优先**：用户侧零侵入（eBPF TPROXY/Sockmap），砍掉所有跨语言 SDK
5. **启发式优先**：红蓝对抗用统计阈值+硬规则，砍掉 ML 模型

---

## 当前状态

### ✅ 已完成（Gateway 核心战斗力）

| 层级 | 模块 | 状态 |
|------|------|------|
| C 数据面 | jitter / npm / bdna / chameleon / phantom / sockmap / h3_shaper（全部有 .c + .o） | ✅ |
| eBPF 管理 | loader / manager / monitor / ringbuffer / billing / emergency / tactical_sync / burn_engine | ✅ |
| 策略引擎 | engine / chameleon / dna_loader / dna_templates / cost / cell_lifecycle / heartbeat / ram_shield | ✅ |
| G-Tunnel | tunnel / multipath / fec / h3_logic / h3_framer / cid_rotation / reincarnation / zero_rtt | ✅ |
| G-Switch | manager / autonomous | ✅ |
| 健康检测 | monitor / app_profiles / app_simulator / echo_escape / feedback_loop / universal_sensor | ✅ |
| Phantom | 13 文件（honeypot / labyrinth / llm_ghost / self_destruct 等） | ✅ |
| Cortex | analyzer / syncer / threat_bus / quarantine / context_persistence / cortex_bridge | ✅ |
| M.C.C. | kill_switch / quorum_sync / signal / sync_broadcast | ✅ |
| 评估器 | scanner / feedback | ✅ |
| TPROXY | bridge / splice（含 Linux 特化） | ✅ |
| 基础设施 | ephemeral logger / storage schema / Dockerfile / systemd / production manifest | ✅ |
| CLI | Ed25519 签名 / 密钥生成 | ✅ |
| 文档 | 架构 / 协议 / 实施指南 / 定价 | ✅ |

### ❌ 缺口

| 缺口 | 影响 |
|------|------|
| Gateway: pkg/api（空） | Gateway 无法与 OS 通信，生死裁决不存在 |
| Gateway: pkg/threat（空） | 威胁事件散落各处，无统一编排 |
| Gateway: pkg/security（空） | 无 mTLS，通信裸奔 |
| Mirage-OS（不存在） | 无大脑，无计费，无蜂窝管理，商业模型不成立 |

---

## 四阶段实施

---

### Phase 1：Gateway 闭环（2 周）

**目标**：Gateway 成为可独立运行、可对外通信的完整战斗单元。

#### 1.1 pkg/security — 安全基础

| 文件 | 职责 |
|------|------|
| tls_manager.go | mTLS 证书加载/验证（读 gateway.yaml 中 mcc.tls 配置） |
| shadow_auth.go | Ed25519 挑战-响应验证（对接 mirage-cli） |

不做 token_store。影子认证是一次性签名验证，不需要会话管理。

#### 1.2 pkg/threat — 威胁编排

| 文件 | 职责 |
|------|------|
| aggregator.go | 统一事件聚合（从 cortex + ebpf/monitor + evaluator 汇总） |
| responder.go | 威胁等级 → 协议参数联动（调用 strategy engine + ebpf loader） |
| blacklist.go | 本地黑名单（LPM Trie 同步到 eBPF Map，接收 OS 下发的全局黑名单） |

#### 1.3 pkg/api — gRPC 通信

| 文件 | 职责 |
|------|------|
| proto/mirage.proto | Protobuf 定义（SyncHeartbeat / ReportTraffic / ReportThreat / PushBlacklist / PushStrategy） |
| grpc_client.go | 上行通道：心跳 + 流量上报 + 威胁上报 |
| grpc_server.go | 下行通道：接收 OS 下发的策略/黑名单/配额/转生指令 |
| handlers.go | 下行指令处理（写入 eBPF Map / 触发 G-Switch / 更新黑名单） |

#### 1.4 main.go 集成

启动顺序调整为：
```
mTLS 初始化 → eBPF 加载 → 策略引擎 → 威胁编排 → gRPC 客户端（上行） → gRPC 服务端（下行） → 健康检查
```

**Phase 1 交付物**：
- Gateway 可通过 mTLS + gRPC 与 OS 双向通信
- 威胁事件有统一的聚合→评估→响应链路
- 影子认证可验证用户合法性

---

### Phase 2：Mirage-OS 大脑（3-4 周）

**目标**：最小可用控制中心，打通生死裁决 + 计费闭环。

#### 架构决策：栈位分层

| 层级 | 技术栈 | 理由 |
|------|--------|------|
| gRPC 服务（与 Gateway 通信） | Go | 性能敏感，必须低延迟 |
| 核心调度（配额熔断/黑名单分发） | Go | 与 gRPC 同进程，避免跨进程开销 |
| 业务 API（用户/蜂窝/计费 CRUD） | NestJS + Prisma | 快速落地复杂业务关系，压缩开发周期 |
| 数据库 | PostgreSQL（numeric(20,8) 精度） | 文档已定义，不改 |
| 缓存/消息 | Redis（Pub/Sub + 缓存） | 轻量，够用 |

#### 目录结构

```
mirage-os/
├── gateway-bridge/                  # Go 服务（gRPC + 核心调度）
│   ├── cmd/bridge/main.go
│   ├── pkg/
│   │   ├── grpc/server.go          # SyncHeartbeat / ReportTraffic / ReportThreat
│   │   ├── quota/enforcer.go       # 配额熔断决策（remaining_quota → 0 时下发阻断）
│   │   ├── intel/distributor.go    # 全局黑名单聚合 + 分发到所有 Gateway
│   │   └── dispatch/strategy.go    # 策略下发（防御等级/拟态模板）
│   ├── proto/mirage.proto          # 与 Gateway 共享
│   └── go.mod
│
├── api-server/                      # NestJS 服务（业务 CRUD）
│   ├── src/
│   │   ├── modules/
│   │   │   ├── auth/               # 用户认证（邀请制 + TOTP）
│   │   │   ├── users/              # 用户管理
│   │   │   ├── cells/              # 蜂窝管理（创建/分配/隔离）
│   │   │   ├── billing/            # 计费（流水/配额/充值）
│   │   │   ├── domains/            # 域名管理（温储备池/流水线状态）
│   │   │   ├── threats/            # 威胁情报查询
│   │   │   └── gateways/           # 节点状态查询
│   │   ├── prisma/
│   │   │   └── schema.prisma       # 数据模型
│   │   └── main.ts
│   ├── package.json
│   └── Dockerfile
│
├── configs/
│   └── mirage-os.yaml
└── docker-compose.yaml              # Go bridge + NestJS + PostgreSQL + Redis
```

#### 核心优先级

| 优先级 | 模块 | 交付标准 |
|--------|------|---------|
| P0 | gateway-bridge: gRPC server | Gateway 心跳能收到，流量能结算 |
| P0 | gateway-bridge: quota enforcer | 配额归零 → 下发 remaining_quota=0 → Gateway 内核态 TC_ACT_STOLEN |
| P0 | api-server: billing module | 流量流水可查，配额可充值 |
| P0 | api-server: auth + users | 邀请制注册，影子认证绑定 |
| P1 | gateway-bridge: intel distributor | 威胁 IP 聚合 → 全局黑名单 → 分发到所有 Gateway |
| P1 | api-server: cells module | 蜂窝创建，用户分配到蜂窝 |
| P1 | api-server: gateways module | 节点在线状态，健康度查询 |
| P2 | api-server: domains module | 域名温储备池状态查看（自动采购放 Phase 4） |
| P2 | api-server: threats module | 威胁情报列表查询 |

**Phase 2 交付物**：
- Gateway → OS 心跳/流量上报 → 结算扣费 → 配额熔断，完整闭环
- 威胁情报聚合 → 全局黑名单秒级分发
- 用户注册/认证/蜂窝分配/配额充值 API 就绪
- 商业模型可运转（能收钱、能断流）

---

### Phase 3：核心链路加固 + 极简控制台（3 周）

**目标**：Raft 高可用 + Shamir 密钥安全 + 可操作的管理界面。

#### 3.1 Raft 集群（Go，在 gateway-bridge 中扩展）

| 文件 | 职责 |
|------|------|
| pkg/raft/cluster.go | hashicorp/raft 集群管理（3-of-5） |
| pkg/raft/fsm.go | 状态机（配额/黑名单/策略的一致性复制） |

不做 geo_fence。地理围栏是运维层面的事，用部署脚本控制节点位置即可。

#### 3.2 Shamir 密钥

| 文件 | 职责 |
|------|------|
| pkg/crypto/shamir.go | 3-of-5 秘密分享（GF(256) 有限域） |
| pkg/crypto/hot_key.go | 热密钥常驻内存（mlock + 自动清零） |

#### 3.3 极简控制台（砍掉所有视觉冗余）

**砍掉**：
- ❌ Three.js 3D 地球
- ❌ 拟态相似度曲线图
- ❌ 拦截热图
- ❌ 所有复杂图表组件

**只保留**：

```
mirage-os/web/
├── src/
│   ├── pages/
│   │   ├── Dashboard.tsx       # 状态总览（数字 + 状态灯，无图表）
│   │   ├── Gateways.tsx        # 节点列表（表格：IP/状态/延迟/流量/威胁等级）
│   │   ├── Cells.tsx           # 蜂窝列表（表格：名称/用户数/域名数/健康度）
│   │   ├── Billing.tsx         # 计费（表格：流水/配额/充值按钮）
│   │   ├── Threats.tsx         # 威胁情报（表格：IP/类型/命中次数/封禁状态）
│   │   └── Strategy.tsx        # 策略控制（下拉框：防御等级/拟态模板 + 应用按钮）
│   ├── components/
│   │   ├── StatusIndicator.tsx # 状态灯（🟢🟡🔴）
│   │   ├── DataTable.tsx       # 通用数据表格
│   │   └── ControlPanel.tsx    # 控制面板（开关/下拉/按钮）
│   ├── hooks/
│   │   └── useApi.ts           # REST API 调用
│   └── App.tsx
├── package.json
└── vite.config.ts
```

技术栈：React + TailwindCSS + 纯表格。无 WebSocket（轮询 5 秒刷新，够用）。

#### 3.4 集成测试（核心链路）

| 测试场景 | 验证点 |
|---------|--------|
| 生死裁决 | 配额归零 → OS 下发 0 → Gateway TC_ACT_STOLEN → 用户断流 |
| 全局免疫 | 威胁 IP 命中 100 次 → OS 聚合 → 黑名单分发 → 所有 Gateway 内核态拦截 |
| 域名转生 | G-Switch 触发 → OS 分配新域名 → 5 秒切换 → 用户无感 |
| 节点自毁 | 心跳超时 300s → eBPF Map 清空 → 内存擦除 → 进程退出 |
| Raft 故障转移 | Leader 宕机 → 自动选举 → 服务不中断 |

**Phase 3 交付物**：
- Raft 3 节点高可用（冰岛/瑞士/新加坡）
- Shamir 密钥分片 + 热密钥零延迟
- 极简控制台可操作（纯表格 + 控制开关）
- 核心链路集成测试全部通过

---

### Phase 4：生产加固与交付（2-3 周）

**目标**：可交付给首批高净值用户。

#### 4.1 交付形态（只做一种）

**透明网关部署**（零侵入）：
- 用户无需修改任何代码
- Gateway 以透明代理模式运行（TPROXY + Sockmap 已实现）
- 用户只需将流量路由到 Gateway IP
- 提供一键部署脚本（Ansible playbook）

砍掉所有 SDK：Nginx 模块、Go SDK、C/Rust FFI、LD_PRELOAD、Python/Node wrapper 全部砍掉。
透明拦截已经足够，用户侧零侵入。

#### 4.2 安全加固

| 项目 | 内容 | 优先级 |
|------|------|--------|
| 内存安全 | memzero 关键变量 / mlock 防 swap（ram_shield.go 已有基础） | P0 |
| 通信安全 | mTLS 双向认证（Phase 1 已实现） + 证书钉扎 | P0 |
| 反调试 | ptrace 检测 / /proc/self/status 监控 | P1 |
| tmpfs 部署 | Alpine Linux + 全内存运行（Dockerfile.alpine 已有） | P1 |
| 紧急自毁 | eBPF Map 原子清空（emergency.go 已实现）+ 进程自杀 | P0（已完成） |

砍掉：盲发现协议（IPFS/区块链）、代码膨胀、ZK-SNARKs。这些是 V2 的事。

#### 4.3 性能验证

| 指标 | 目标 | 已有基础 |
|------|------|---------|
| eBPF 延迟 | < 1ms | 文档标称 0.5ms |
| FEC 编码 | < 1ms | AVX-512 加速已实现 |
| G-Switch 转生 | < 5s | reincarnation.go 已实现 |
| CPU 总占用 | < 20% | 文档标称 15% |
| 内存占用 | < 200MB | 文档标称 180MB |

#### 4.4 部署自动化

| 项目 | 内容 |
|------|------|
| Gateway 部署 | Ansible playbook：安装 → 编译 BPF → 启动服务 → 注册到 OS |
| OS 部署 | docker-compose：Go bridge + NestJS + PostgreSQL + Redis |
| 证书分发 | 自动生成 mTLS 证书链 + 分发到各节点 |

**Phase 4 交付物**：
- 透明网关一键部署
- 安全加固通过基础红队测试
- 性能指标验证通过
- 可接入首批 10-20 个高净值用户

---

## 时间线

| 阶段 | 周期 | 核心产出 |
|------|------|---------|
| Phase 1 | 第 1-2 周 | Gateway 闭环（security + threat + api） |
| Phase 2 | 第 3-6 周 | OS 大脑 MVP（Go gRPC bridge + NestJS 业务层） |
| Phase 3 | 第 7-9 周 | Raft + Shamir + 极简控制台 + 集成测试 |
| Phase 4 | 第 10-12 周 | 安全加固 + 部署自动化 + 首批用户接入 |

**总计：10-12 周**

---

## 被砍掉的（V2 再做）

| 砍掉的功能 | 理由 | V2 时机 |
|-----------|------|---------|
| Three.js 3D 地球 | 视觉冗余，高净值用户要的是隐蔽性不是动画 | 用户量 > 50 后 |
| 拟态相似度曲线/热图 | 同上 | 同上 |
| WebSocket 实时推送 | 5 秒轮询够用，省掉整个 ws 层 | 用户反馈需要时 |
| ML 红蓝对抗模型 | CPU 开销大，开发周期长，启发式阈值够用 | 有专职 ML 工程师时 |
| 跨语言 SDK 矩阵 | 透明网关零侵入，不需要 SDK | 有移动端需求时 |
| Nginx 模块 | 同上 | 有高并发服务端用户时 |
| 盲发现协议 | IPFS/区块链解析入口节点，V1 用硬编码 + 配置文件 | 节点 > 50 时 |
| 域名自动采购 | Monero 混币 + Tor 注册，V1 手动采购 | 域名消耗速度要求自动化时 |
| 死因分析 ML 建模 | V1 用简单统计（存活天数/注册商/后缀） | 数据积累足够时 |
| 地理围栏 | 部署脚本控制节点位置即可 | 节点动态迁移需求出现时 |
| 代码膨胀/ZK-SNARKs | 过度防御，V1 mTLS + Ed25519 够用 | 面临国家级逆向时 |

---

## 红蓝对抗策略（V1 启发式方案）

砍掉 ML 模型后，评估器（evaluator/scanner.go + feedback.go）的闭环反馈改为：

| 检测维度 | 阈值规则 | 触发动作 |
|---------|---------|---------|
| IAT 偏离度 | 实际 IAT 与模板 IAT 的 KL 散度 > 0.3 | 自动切换 B-DNA 模板 |
| 包长度熵 | Shannon 熵偏离目标 > 15% | 调整 NPM padding 参数 |
| TLS 指纹匹配度 | JA4 哈希不匹配率 > 5% | 切换 Chameleon 配置 |
| 异常分数 | 综合分 > 20（满分 100） | 提升防御等级 |

这些规则已经可以用现有的 evaluator 代码实现，不需要引入任何 ML 依赖。

---

## 关键风险

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| NestJS 与 Go bridge 通信延迟 | 低 | 计费不准 | 共享 PostgreSQL，Go 直接写流水，NestJS 只读 |
| Raft 集群跨洲延迟 | 中 | 选举超时 | 调大 heartbeat timeout，Leader 优先亚洲节点 |
| 首批用户发现功能缺失 | 中 | 口碑风险 | 明确 V1 功能边界，承诺 V2 路线图 |
| eBPF 内核兼容性 | 低 | 部分节点无法部署 | Fallback 到用户态（已有降级逻辑） |

---

## 下一步

**立即开始 Phase 1**：pkg/security → pkg/threat → pkg/api → main.go 集成。

从 proto/mirage.proto 开始，定义 Gateway ↔ OS 的通信契约，这是整个系统的脊柱。
