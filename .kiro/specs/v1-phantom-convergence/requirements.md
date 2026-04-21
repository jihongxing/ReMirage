# 需求文档：Phantom 蜜罐收敛（第一阶段）

## 简介

本 Spec 对应 `Phantom-蜜罐策略审计与收敛建议.md` 第一阶段 + 第二阶段，目标是将 Phantom 从"显眼的欺骗系统"收敛为"低可见度、分级触发、有限追踪、可自动退出的异常来源隔离层"。

当前 Phantom 核心问题：
1. **数据面**：`phantom_stats` 中 `STAT_PASSED` 计数逻辑错误，统计不准确
2. **数据面**：`phishing_list_map` 只存首次时间戳，无 TTL/风险等级/命中次数，名单越来越脏
3. **数据面**：全局单一 `honeypot_ip`，所有重定向收敛到同一目标，形成明显模式
4. **调度层**：规则静态写死，Header 顺序判断不可信，不同节点行为高度雷同
5. **模板层**：官网/错误页/API/迷宫缺少统一业务世界观，迷宫无限深度消耗资源
6. **追踪层**：`_tracking` 字段和 `/canary/` 路径过于显眼

本轮整改核心目标：**降低系统自我暴露，提高策略可控性，让 Phantom 服从 Mirage 低暴露路线。**

## 术语表

- **Phantom**：Mirage 的异常来源隔离模块，包含 eBPF 数据面重定向 + Go 用户态蜜罐服务
- **phishing_list_map**：eBPF Hash Map，存储需要重定向的威胁 IP
- **honeypot_config**：eBPF Array Map，存储蜜罐目标 IP（当前只有 1 个）
- **分层目标池**：按 Cell/风险等级分配不同蜜罐目标，替代单一 honeypot_ip
- **业务画像**：每个 Gateway/Cell 绑定的稳定对外身份（如"某企业官网"），所有 Phantom 响应保持一致
- **有限迷宫**：深度有限（最大 5 层）、最终自然死路的 API 响应链

## 需求

### 需求 1：修复数据面统计桶错误

**用户故事：** 作为运维工程师，我需要 Phantom 统计数据准确，以便运营决策不被错误数据误导。

#### 验收标准

1. THE `phantom_redirect` eBPF 程序 SHALL 在正常放行路径中使用 `key = STAT_PASSED` 查询并递增统计，而不是先用 `key=0` 查询再改 key
2. THE `STAT_REDIRECTED` 和 `STAT_PASSED` 计数 SHALL 准确反映实际重定向数和放行数
3. THE Go 侧 SHALL 提供 `GetPhantomStats()` 方法读取 `phantom_stats` Map，返回 redirected/passed/trapped/errors 四项计数
4. THE 统计数据 SHALL 与 Spec 2-1 的 Prometheus 指标集成（`mirage_honeypot_hit_total` 使用 STAT_REDIRECTED 值）

### 需求 2：名单增加 TTL 和过期机制

**用户故事：** 作为安全工程师，我需要 Phantom 名单有自动过期能力，以便误判对象不会被长期困在蜜罐。

#### 验收标准

1. THE `phishing_list_map` 的 value SHALL 从 `__u64`（时间戳）升级为结构体，包含：`first_seen`（uint64）、`last_seen`（uint64）、`hit_count`（uint32）、`risk_level`（uint8）、`ttl_seconds`（uint32）
2. THE Go 侧 PhantomManager SHALL 每 30 秒扫描 `phishing_list_map`，清理 `last_seen + ttl_seconds < now` 的过期条目
3. THE 默认 TTL SHALL 为 3600 秒（1 小时），高风险来源（risk_level >= 3）TTL 为 86400 秒（24 小时）
4. THE 数据面 SHALL 在每次命中时更新 `last_seen` 和递增 `hit_count`
5. THE Go 侧 SHALL 提供 `AddToPhantom(ip, riskLevel, ttl)` 和 `RemoveFromPhantom(ip)` 方法

### 需求 3：单一蜜罐目标升级为分层目标池

**用户故事：** 作为安全工程师，我需要不同风险等级的流量被引导到不同目标，以便避免所有重定向收敛到同一个洞口。

#### 验收标准

1. THE `honeypot_config` eBPF Map SHALL 从 `max_entries=1` 升级为 `max_entries=8`，支持按 risk_level 索引不同蜜罐 IP
2. THE 数据面 SHALL 在重定向时根据名单条目的 `risk_level` 查找对应蜜罐 IP
3. THE Go 侧 SHALL 提供 `SetHoneypotPool(level int, ip string)` 方法配置分层目标池
4. IF 对应 risk_level 的蜜罐 IP 未配置，THEN 回退到 level=0 的默认蜜罐
5. THE 分层目标池 SHALL 可通过 `gateway.yaml` 配置

### 需求 4：去除显眼追踪标记和回调路径

**用户故事：** 作为安全工程师，我需要 Phantom 的追踪手段不留下明显系统指纹，以便不暴露 Mirage 的存在。

#### 验收标准

1. THE 蜜罐响应 SHALL 不再包含 `_tracking` 字段名，追踪 ID 改为嵌入到自然字段中（如 `id`、`ref`、`session` 等常见字段名）
2. THE `/canary/` 回调路径 SHALL 改为更自然的路径（如 `/assets/`、`/static/`、`/img/`）
3. THE 金丝雀文件下载 SHALL 不使用 `classification: CONFIDENTIAL` 等显眼标记
4. THE 追踪像素 SHALL 使用常见的 Web Analytics 路径模式（如 `/collect`、`/pixel`）

### 需求 5：停止依赖不可信的 Header 顺序判断

**用户故事：** 作为开发工程师，我需要调度规则基于可信信号，以便降低误判率。

#### 验收标准

1. THE `IsSuspiciousHeaderOrder` 函数 SHALL 被标记为 deprecated 并从调度规则中移除
2. THE 调度规则 SHALL 不再依赖 Go `map` 迭代顺序作为 Header 顺序信号
3. THE `RequestContext.HeaderOrder` 字段 SHALL 被移除或标记为不可信

### 需求 6：减少模板种类，绑定稳定业务画像

**用户故事：** 作为安全工程师，我需要每个 Gateway/Cell 对外呈现一致的业务画像，以便 Phantom 响应不像"模板集合"。

#### 验收标准

1. THE Phantom SHALL 支持为每个 Gateway 配置一个稳定的业务画像（`phantom.persona`），包含：公司名称、域名风格、错误码风格
2. THE 所有 Phantom 响应（官网/错误页/API/默认页）SHALL 使用同一个 persona 的世界观，保持一致
3. THE 模板种类 SHALL 从 4 种收敛为 3 种：`corporate`（企业站）、`error`（错误页）、`api`（API 服务），移除 `old_admin_portal` 独立模板
4. THE persona 配置 SHALL 可通过 `gateway.yaml` 设置，也可由 OS 下发

### 需求 7：无限迷宫改为有限深度自然死路

**用户故事：** 作为安全工程师，我需要迷宫有限深度且自然结束，以便不消耗过多资源且不暴露"刻意诱导"的意图。

#### 验收标准

1. THE LabyrinthEngine SHALL 设置最大深度为 5 层，超过 5 层返回 404 或空结果
2. THE 迷宫响应 SHALL 不再包含 `_links` 和 `_meta` 等明显的 HATEOAS 风格字段
3. THE 迷宫延迟 SHALL 上限为 3 秒（当前 maxDelay=30s 过高）
4. THE 迷宫最终页 SHALL 返回自然的 404/403/空 JSON，而不是继续生成更深链接
5. THE 迷宫 SHALL 使用 persona 中的 API 风格，保持与其他 Phantom 响应一致

### 需求 8：统一官网、错误页、API、默认页的世界观

**用户故事：** 作为安全工程师，我需要所有 Phantom 响应属于同一个"世界"，以便不因模板割裂暴露系统。

#### 验收标准

1. THE 企业官网模板 SHALL 使用 persona 中的公司名称和风格
2. THE 错误页模板 SHALL 使用与官网一致的视觉风格和错误码命名空间
3. THE API 响应 SHALL 使用与官网一致的版本号、产品名称
4. THE 默认页（404）SHALL 使用与官网一致的页面风格
5. THE 所有模板 SHALL 共享同一个 CSS 风格基础，不出现风格跳变
