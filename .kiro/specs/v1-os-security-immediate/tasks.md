# 任务清单：OS 安全紧急封洞

## 需求 1：关闭普通用户到管理面的权限升级路径

- [x] 1. 权限升级修复
  - [x] 1.1 修改 `mirage-os/api-server/src/modules/auth/auth.service.ts` login 方法：JWT payload 从 `{ sub: user.id, cell_id: user.cellId }` 改为 `{ sub: user.id, cell_id: user.cellId, role: 'user' }`
  - [x] 1.2 修改 `mirage-os/api-server/src/modules/auth/breach.service.ts` validateToken 方法：从原 token payload 中提取 `role` 字段（`const originalRole = payload.role || 'user'`），续签时使用 `originalRole` 而不是硬编码 `'admin'`
  - [x] 1.3 修改 `mirage-os/api-server/src/modules/auth/breach.service.ts` validateToken 返回值：`role` 字段使用 `originalRole`
  - [x] 1.4 修改 `mirage-os/api-server/src/modules/auth/jwt.strategy.ts` validate 方法：参数类型增加 `role?: string`，返回对象增加 `role: payload.role || 'user'`

## 需求 2：为所有管理接口补上角色校验

- [x] 2. RBAC 角色校验
  - [x] 2.1 创建 `mirage-os/api-server/src/modules/auth/roles.decorator.ts`：导出 `ROLES_KEY = 'roles'` 常量和 `Roles(...roles: string[])` 装饰器（使用 `SetMetadata`）
  - [x] 2.2 创建 `mirage-os/api-server/src/modules/auth/roles.guard.ts`：实现 `RolesGuard`（CanActivate），使用 Reflector 获取 handler/class 上的 ROLES_KEY metadata，从 request.user.role 检查是否匹配，不匹配时抛出 ForbiddenException
  - [x] 2.3 修改 `mirage-os/api-server/src/modules/auth/auth.module.ts`：导出 RolesGuard 和 Roles 装饰器
  - [x] 2.4 修改 `mirage-os/api-server/src/modules/users/users.controller.ts`：增加 `@UseGuards(RolesGuard)` 和 `@Roles('admin')` 到 class 级别（所有用户管理接口需要 admin）
  - [x] 2.5 修改 `mirage-os/api-server/src/modules/gateways/` 相关 Controller：增加 `@UseGuards(RolesGuard)` 和 `@Roles('admin')` 到写操作
  - [x] 2.6 修改 `mirage-os/api-server/src/modules/cells/` 相关 Controller：增加 `@UseGuards(RolesGuard)` 和 `@Roles('admin')` 到写操作
  - [x] 2.7 修改 `mirage-os/api-server/src/modules/threats/` 相关 Controller：增加 `@UseGuards(RolesGuard)` 和 `@Roles('admin')` 到所有接口
  - [x] 2.8 修改 `mirage-os/api-server/src/modules/domains/` 相关 Controller：增加 `@UseGuards(RolesGuard)` 和 `@Roles('admin')` 到写操作

## 需求 3：收口所有内部接口

- [x] 3. 内部接口收口
  - [x] 3.1 修改 `mirage-os/gateway-bridge/pkg/config/config.go`：在 RESTConfig 结构体中增加 `InternalSecret string \`yaml:"internal_secret"\`` 字段
  - [x] 3.2 创建 `mirage-os/gateway-bridge/pkg/rest/middleware.go`：实现 `InternalAuthMiddleware(secret string) func(http.Handler) http.Handler` — 检查 `r.Header.Get("X-Internal-Secret") != secret` 时返回 401
  - [x] 3.3 修改 `mirage-os/gateway-bridge/cmd/bridge/main.go` REST 服务启动段：将 `restAddr` 默认值从 `":7000"` 改为 `"127.0.0.1:7000"`；在 `restServer` 创建时用 `InternalAuthMiddleware(cfg.REST.InternalSecret)` 包装 mux
  - [x] 3.4 修改 `mirage-os/gateway-bridge/cmd/bridge/main.go`：在 gRPC 服务启动前增加生产模式校验 — `os.Getenv("MIRAGE_ENV") == "production"` 且 `!cfg.GRPC.TLSEnabled` 时 `log.Fatalf`
  - [x] 3.5 修改 `mirage-os/gateway-bridge/configs/mirage-os.yaml`：在 `rest` 段增加 `internal_secret: "${MIRAGE_INTERNAL_SECRET}"`，将 `grpc.tls_enabled` 默认值改为 `true`

## 需求 4：给登录/认证接口补速率限制

- [x] 4. 速率限制
  - [x] 4.1 修改 `mirage-os/api-server/package.json`：增加 `@nestjs/throttler` 依赖
  - [x] 4.2 修改 `mirage-os/api-server/src/app.module.ts`：导入 `ThrottlerModule.forRoot([{ ttl: 60000, limit: 60 }])`（全局默认：60s 内 60 次）
  - [x] 4.3 修改 `mirage-os/api-server/src/app.module.ts`：在 providers 中增加 `{ provide: APP_GUARD, useClass: ThrottlerGuard }`（全局启用）
  - [x] 4.4 修改 `mirage-os/api-server/src/modules/auth/auth.controller.ts`：在 class 级别增加 `@Throttle({ default: { ttl: 60000, limit: 10 } })`（认证接口更严格：60s 内 10 次）
  - [x] 4.5 安装依赖：运行 `npm install @nestjs/throttler`（在 mirage-os/api-server/ 目录）

## 需求 5：清理生产环境默认密钥

- [x] 5. 默认密钥清理
  - [x] 5.1 修改 `mirage-os/api-server/src/modules/auth/jwt.strategy.ts`：在 constructor 中增加生产模式校验 — 当 `process.env.NODE_ENV === 'production'` 时，检查 secretOrKey 是否为默认值或长度 < 32，是则 `throw new Error('生产模式必须设置 JWT_SECRET（>= 32 字符）')`
  - [x] 5.2 修改 `mirage-os/api-server/src/main.ts`：在 `bootstrap()` 函数开头增加 — 当 `process.env.NODE_ENV === 'production'` 且 `!process.env.JWT_SECRET` 时 `throw new Error('生产模式必须设置 JWT_SECRET 环境变量')`
  - [x] 5.3 修改 `mirage-os/gateway-bridge/cmd/bridge/main.go`：在配置加载后增加 — 当 `os.Getenv("MIRAGE_ENV") == "production"` 且 `cfg.REST.InternalSecret == ""` 时 `log.Fatalf("[FATAL] 生产模式必须设置 rest.internal_secret")`
