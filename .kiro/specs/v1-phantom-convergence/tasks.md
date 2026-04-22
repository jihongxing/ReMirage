# 任务清单：Phantom 蜜罐收敛（第一阶段）

## 任务

- [x] 1. 数据面统计修复
  - [x] 1.1 修复 `phantom.c` 中 `STAT_PASSED` 计数逻辑：直接用 `STAT_PASSED` 作为 key 查询并递增
  - [x] 1.2 在 Go 侧 PhantomManager 中实现 `GetPhantomStats()` 方法，读取 `phantom_stats` Map 返回四项计数
  - [x] 1.3 将 `STAT_REDIRECTED` 值集成到 Prometheus 指标 `mirage_honeypot_hit_total`（复用 Spec 2-1 指标）
  - [x] 1.4 重新编译 phantom.o：`clang -O2 -target bpf -c phantom.c -o phantom.o`

- [x] 2. 名单结构升级与 TTL 过期
  - [x] 2.1 在 `phantom.c` 中将 `phishing_list_map` value 从 `__u64` 升级为 `struct phantom_entry`（first_seen, last_seen, hit_count, risk_level, ttl_seconds）
  - [x] 2.2 修改数据面命中逻辑：每次命中更新 `last_seen` 和递增 `hit_count`
  - [x] 2.3 在 Go 侧定义对应的 `PhantomEntry` 结构体，确保与 C 结构体内存布局一致
  - [x] 2.4 实现 `AddToPhantom(ip, riskLevel, ttl)` 和 `RemoveFromPhantom(ip)` 方法
  - [x] 2.5 实现 `StartTTLCleaner`：每 30 秒遍历 Map 清理过期条目
  - [x] 2.6 重新编译 phantom.o

- [x] 3. 分层蜜罐目标池
  - [x] 3.1 在 `phantom.c` 中将 `honeypot_config` Map 的 `max_entries` 从 1 扩展为 8
  - [x] 3.2 修改数据面重定向逻辑：按 `entry->risk_level` 索引 `honeypot_config`，未配置时回退 level=0
  - [x] 3.3 在 Go 侧实现 `SetHoneypotPool(level, ip)` 方法
  - [x] 3.4 在 Gateway 启动时从 `gateway.yaml` 读取 `phantom.honeypot_pool` 配置并写入 Map
  - [x] 3.5 重新编译 phantom.o

- [x] 4. 追踪去显式化
  - [x] 4.1 将 `honeypot.go` 中 `_tracking` 字段改为 `ref` 或 `id` 等自然字段名
  - [x] 4.2 将 `/canary/` 回调路径改为 `/static/img/` 和 `/collect` 等自然路径
  - [x] 4.3 移除金丝雀文件中的 `classification: CONFIDENTIAL` 标记
  - [x] 4.4 将追踪像素路径改为常见 Web Analytics 模式

- [x] 5. 调度规则清理
  - [x] 5.1 将 `IsSuspiciousHeaderOrder` 函数标记为 deprecated 并从调度规则中移除
  - [x] 5.2 从 `RequestContext` 中移除 `HeaderOrder` 字段
  - [x] 5.3 从 `extractContext` 方法中移除 Header 顺序提取逻辑

- [x] 6. Persona 业务画像系统
  - [x] 6.1 新建 `pkg/phantom/persona.go`，定义 Persona 结构体和 DefaultPersona
  - [x] 6.2 在 Dispatcher 中增加 `persona` 字段，从 `gateway.yaml` 加载配置
  - [x] 6.3 改造 `serveCorporateWeb` 模板：使用 persona 的公司名称、颜色、标语
  - [x] 6.4 改造 `serveNetworkError` 模板：使用 persona 的错误码前缀和视觉风格
  - [x] 6.5 改造 `serveStandardHTTPS`（404）模板：使用 persona 的页面风格
  - [x] 6.6 改造 `handleDefault` 默认页：使用 persona 的公司名称
  - [x] 6.7 移除 `ShadowOldAdminPortal` 独立模板类型，管理路径探测改为使用迷宫（API 风格）

- [x] 7. 迷宫限深与自然化
  - [x] 7.1 在 LabyrinthEngine 中增加 `MaxDepth` 常量（默认 5），超过深度返回 404
  - [x] 7.2 将 `maxDelay` 从 30s 降为 3s
  - [x] 7.3 移除响应中的 `_links` 和 `_meta` 字段，改为自然的分页风格（`next` 字段）
  - [x] 7.4 迷宫响应使用 persona 的 API 版本号和产品名称
  - [x] 7.5 最终页返回自然的 404 JSON（`{"error": "not_found"}`）

- [x] 8. 配置与集成
  - [x] 8.1 在 `gateway.yaml` 中增加 `phantom` 配置段（persona、honeypot_pool、TTL、迷宫参数）
  - [x] 8.2 在 Gateway 启动流程中加载 phantom 配置并初始化 Persona、目标池、TTL 清理器
  - [x] 8.3 确保 Phantom 与 Spec 2-1 的联动链路正常：蜜罐命中 → Cortex RiskScorer → 自动封禁

- [x] 9. 测试
  - [x] 9.1 编写数据面统计准确性测试：STAT_PASSED 和 STAT_REDIRECTED 计数正确
  - [x] 9.2 编写名单 TTL 过期测试：添加条目 → 等待过期 → 确认已清理
  - [x] 9.3 编写分层目标池测试：不同 risk_level 重定向到不同蜜罐 IP
  - [x] 9.4 编写 persona 一致性测试：所有模板响应包含相同公司名称和风格
  - [x] 9.5 编写迷宫限深测试：深度 > 5 返回 404
