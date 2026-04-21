# 任务清单：OS-Gateway 安全闭环

## 任务

- [x] 1. 统一 RBAC 与资源归属模型（OS 侧）
  - [x] 1.1 新建 `rbac-matrix.ts`，定义 Role 枚举、Permission 枚举、RBAC_MATRIX 角色权限矩阵
  - [x] 1.2 升级 `roles.guard.ts`，从 RBAC_MATRIX 查询权限替代硬编码角色检查
  - [x] 1.3 新建 `@Permissions()` 装饰器，替代原 `@Roles()` 装饰器用于细粒度权限控制
  - [x] 1.4 新建 `owner.guard.ts`，实现资源归属校验（user 角色只能访问自己的资源）
  - [x] 1.5 为 UsersController、GatewaysController、CellsController、ThreatsController、DomainsController 应用 @Permissions + OwnerGuard
  - [x] 1.6 更新 `JwtStrategy.validate` 确保 role 缺失时默认为 `user`
  - [x] 1.7 编写 RBAC 单元测试：验证四种角色的权限边界

- [x] 2. 内部服务统一鉴权标准（OS 侧）
  - [x] 2.1 新建 `gateway-bridge/pkg/grpc/interceptor.go`，实现 gRPC CN 白名单拦截器
  - [x] 2.2 在 gateway-bridge gRPC Server 启动时注入 CN 白名单拦截器
  - [x] 2.3 为 ws-gateway 增加 WebSocket 连接 JWT 校验中间件
  - [x] 2.4 为 gateway-bridge REST middleware 增加访问日志记录（来源 IP、路径、鉴权结果）
  - [x] 2.5 确保 API Server 到 gateway-bridge 的内部调用统一携带 `X-Internal-Secret`

- [x] 3. 控制面审计日志（OS 侧）
  - [x] 3.1 在 Prisma schema 中增加 `audit_logs` 表定义
  - [x] 3.2 运行 Prisma migrate 生成数据库迁移
  - [x] 3.3 新建 `audit.module.ts`、`audit.service.ts`，实现审计日志写入服务
  - [x] 3.4 新建 `audit-interceptor.ts`，自动拦截敏感操作并写入审计日志
  - [x] 3.5 新建 `audit.controller.ts`，实现审计日志查询 API（仅 admin/auditor 可访问）
  - [x] 3.6 在 gateway-bridge 新建 `pkg/audit/audit.go`，为下行指令记录审计日志
  - [x] 3.7 将 AuditInterceptor 注册到 AppModule 全局拦截器

- [x] 4. 威胁情报回路（OS → Gateway 封禁同步）
  - [x] 4.1 在 Prisma schema 中增加 `threat_intel` 表定义并运行迁移
  - [x] 4.2 新建 `threat-intel.service.ts`，实现威胁情报 CRUD + TTL 过期清理 + 自动全局封禁评估
  - [x] 4.3 新建 `gateway-bridge/pkg/dispatch/blacklist_sync.go`，实现 OS → Gateway 黑名单同步（通过 PushBlacklist）
  - [x] 4.4 在 gateway-bridge gRPC handler 中增加 Gateway 威胁事件上报处理，转发到 ThreatIntelService
  - [x] 4.5 扩展 Gateway 心跳上报，增加黑名单摘要字段（条目数 + 最新更新时间戳）
  - [x] 4.6 在 OS 侧心跳处理中增加黑名单一致性校验，不一致时触发全量同步

- [x] 5. 安全基线（OS 侧）
  - [x] 5.1 在 `main.ts` 中启用 ValidationPipe（whitelist + forbidNonWhitelisted + transform）
  - [x] 5.2 安装并配置 helmet 中间件，设置安全 HTTP Header
  - [x] 5.3 收敛 CORS 配置：生产模式下从环境变量读取 allowed_origins
  - [x] 5.4 新建 `http-exception.filter.ts`，实现错误响应脱敏（生产模式不返回堆栈）
  - [x] 5.5 为所有 GET 列表接口增加分页 DTO（默认 20，最大 100）
  - [x] 5.6 在 gateway-bridge 生产模式下禁用 gRPC reflection

- [x] 6. 标准入口处置策略（Gateway 侧）
  - [x] 6.1 新建 `pkg/threat/policy.go`，实现 IngressPolicy 策略引擎（条件匹配 + 优先级排序 + 动作执行）
  - [x] 6.2 新建 `pkg/threat/policy_config.go`，定义策略配置结构和 YAML 解析
  - [x] 6.3 在 `gateway.yaml` 中增加 `ingress_policy.rules` 默认配置
  - [x] 6.4 将 IngressPolicy 集成到 ThreatResponder，替代现有硬编码的威胁等级 → 动作映射
  - [x] 6.5 为每次处置动作记录结构化安全日志
  - [x] 6.6 编写 IngressPolicy 单元测试：验证多条件优先级、各动作类型

- [x] 7. 蜜罐-指纹-黑名单联动（Gateway 侧）
  - [x] 7.1 新建 `pkg/cortex/risk_scorer.go`，实现 IP 风险评分器（评分累加 + 时间衰减 + 自动封禁）
  - [x] 7.2 在 Phantom 模块增加蜜罐命中事件上报到 Cortex ThreatBus
  - [x] 7.3 在 Cortex ThreatBus 增加 honeypot/fingerprint 事件类型订阅
  - [x] 7.4 在 B-DNA 模块增加高危指纹检测事件上报到 Cortex ThreatBus
  - [x] 7.5 实现断路器保护：Cortex 处理队列满时丢弃低优先级事件
  - [x] 7.6 编写联动链路集成测试：蜜罐命中 → 评分累加 → 超阈值自动封禁

- [x] 8. Gateway 本地安全状态机（Gateway 侧）
  - [x] 8.1 新建 `pkg/threat/security_fsm.go`，实现 5 状态安全状态机（Normal/Alert/HighPressure/Isolated/Silent）
  - [x] 8.2 实现状态迁移逻辑：升级立即执行，降级 300 秒冷却期
  - [x] 8.3 实现每种状态对应的入口策略覆盖（通过 IngressPolicy.ApplyStateOverride）
  - [x] 8.4 将 SecurityFSM 集成到 Responder，由威胁等级变化和安全指标驱动状态迁移
  - [x] 8.5 实现 OS 强制切换状态（通过 PushStrategy 指令中的 security_state 字段）
  - [x] 8.6 将当前安全状态通过心跳上报 OS
  - [x] 8.7 编写状态机单元测试：验证迁移条件、冷却期、强制切换

- [x] 9. Prometheus 观测指标（Gateway 侧）
  - [x] 9.1 新建 `pkg/threat/metrics.go`，定义并注册所有 Prometheus 指标
  - [x] 9.2 在 main.go 启动 metrics HTTP server（127.0.0.1:9090）
  - [x] 9.3 在 IngressPolicy 处置动作执行时递增 `mirage_ingress_reject_total`
  - [x] 9.4 在 BlacklistManager 命中时递增 `mirage_blacklist_hit_total`
  - [x] 9.5 在 SecurityFSM 状态变化时更新 `mirage_security_state` gauge
  - [x] 9.6 在 Phantom 蜜罐命中时递增 `mirage_honeypot_hit_total`
  - [x] 9.7 在 CommandAuth 鉴权失败时递增 `mirage_auth_failure_total`

- [x] 10. 安全回归测试
  - [x] 10.1 编写 Gateway 侧安全回归测试：mTLS 拒绝、黑名单生效、入口 Drop、状态机迁移、联动链路
  - [x] 10.2 编写 OS 侧安全回归测试：RBAC 越权、审计日志完整性
  - [x] 10.3 在 Makefile 增加 `test-security` target，一键执行所有安全回归测试
