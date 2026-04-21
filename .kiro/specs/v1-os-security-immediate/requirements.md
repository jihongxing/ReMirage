# 需求文档：OS 安全紧急封洞

## 简介

本 Spec 对应 `OS-Gateway 安全整改清单.md` 中 OS 立刻修部分（OS-Immediate-1 ~ 5），目标是封堵 Mirage-OS 侧最高风险的安全漏洞。

当前 OS 的核心安全问题：
1. **权限升级路径**：`validateToken` 续签时硬编码 `role: 'admin'`，任何用户 token 续签后都变成管理员
2. **无角色校验**：所有 API 只有 `JwtAuthGuard`，无 RBAC，普通用户可访问所有管理接口（users/gateways/cells/threats）
3. **内部接口裸暴露**：gateway-bridge REST API（:7000）无任何鉴权，`/internal/gateway/{id}/kill` 可被任意内网请求调用
4. **无速率限制**：login/register/challenge/breach/validate 接口无频控，可被暴力破解
5. **JWT 密钥硬编码**：`dev_jwt_secret_change_in_production` 作为默认密钥

## 术语表

- **API Server**：NestJS REST API 服务，监听 :3000，提供用户认证、计费、威胁等接口
- **Gateway Bridge**：Go gRPC + REST 服务，监听 :50051（gRPC）和 :7000（内部 REST）
- **JwtAuthGuard**：NestJS JWT 认证守卫，验证 Bearer token 有效性
- **BreachService**：Ed25519 挑战-响应认证服务，用于管理员入口
- **RBAC**：基于角色的访问控制（Role-Based Access Control）
- **RolesGuard**：角色校验守卫（待实现），检查 JWT payload 中的 role 字段

## 需求

### 需求 1：关闭普通用户到管理面的权限升级路径

**用户故事：** 作为安全工程师，我需要普通用户 token 无法通过续签变成管理员 token，以便权限边界不可被绕过。

#### 验收标准

1. THE `BreachService.validateToken` SHALL 在续签时保持原 token 中的 role 字段不变，而不是硬编码 `role: 'admin'`
2. THE `AuthService.login` SHALL 在 JWT payload 中增加 `role: 'user'` 字段（普通用户登录）
3. THE `BreachService.verifyAndSign` SHALL 保持 `role: 'admin'`（Ed25519 管理员认证）
4. IF 普通用户 token 被提交到 `POST /auth/validate`，THEN 续签后的 token 中 role SHALL 仍为 `'user'`
5. THE `JwtStrategy.validate` SHALL 从 payload 中提取 role 字段并附加到 request.user 对象

### 需求 2：为所有管理接口补上角色校验

**用户故事：** 作为安全工程师，我需要管理类接口只允许 admin 角色访问，以便普通用户无法查看或修改其他用户的资源。

#### 验收标准

1. THE API Server SHALL 实现 `RolesGuard`（NestJS CanActivate），从 request.user.role 检查是否匹配所需角色
2. THE API Server SHALL 实现 `@Roles('admin')` 装饰器，用于标记需要管理员权限的接口
3. THE `UsersController` 的 `findAll`、`findOne`、`bindPubkey`、`deactivate` SHALL 增加 `@Roles('admin')` 装饰器
4. THE `GatewaysModule`、`CellsModule`、`ThreatsModule`、`DomainsModule` 的所有写操作 SHALL 增加 `@Roles('admin')` 装饰器
5. THE `BillingController` SHALL 限制用户只能查看自己的计费数据（当前已通过 req.user.userId 实现，保持不变）
6. IF 普通用户（role='user'）访问带 `@Roles('admin')` 的接口，THEN THE API SHALL 返回 HTTP 403 Forbidden

### 需求 3：收口所有内部接口

**用户故事：** 作为安全工程师，我需要内部 REST API 不能被外部未授权直接调用，以便焦土指令等高危操作不会被滥用。

#### 验收标准

1. THE gateway-bridge REST API SHALL 绑定到 `127.0.0.1:7000` 而不是 `0.0.0.0:7000`
2. THE gateway-bridge REST API SHALL 为所有 `/internal/*` 路由增加共享密钥校验 — 请求必须携带 `X-Internal-Secret` Header，值与配置中 `rest.internal_secret` 匹配
3. IF `X-Internal-Secret` Header 缺失或不匹配，THEN THE REST API SHALL 返回 HTTP 401 Unauthorized
4. THE gateway-bridge gRPC 服务 SHALL 在生产模式下强制启用 TLS（当 `MIRAGE_ENV=production` 且 `grpc.tls_enabled=false` 时拒绝启动）
5. THE `mirage-os.yaml` SHALL 增加 `rest.internal_secret` 配置项（环境变量 `MIRAGE_INTERNAL_SECRET`）

### 需求 4：给登录/认证接口补速率限制

**用户故事：** 作为安全工程师，我需要认证接口有频控保护，以便暴力破解和撞库攻击无法高频连续尝试。

#### 验收标准

1. THE API Server SHALL 对 `POST /auth/register`、`POST /auth/login`、`GET /auth/challenge`、`POST /auth/breach`、`POST /auth/validate` 增加 IP 级速率限制
2. THE 速率限制 SHALL 为：同一 IP 每分钟最多 10 次认证请求，超出返回 HTTP 429 Too Many Requests
3. THE 速率限制 SHALL 对连续 5 次登录失败的 IP 增加 5 分钟冷却期（冷却期内所有认证请求返回 429）
4. THE API Server SHALL 使用 NestJS `@nestjs/throttler` 模块实现速率限制
5. THE 速率限制状态 SHALL 存储在内存中（第一期不依赖 Redis，后续可升级）

### 需求 5：清理生产环境默认密钥

**用户故事：** 作为安全工程师，我需要生产环境不存在默认密钥和弱配置，以便系统不会因为忘记修改默认值而被攻破。

#### 验收标准

1. THE `JwtStrategy` SHALL 在 `JWT_SECRET` 环境变量未设置或为默认值 `'dev_jwt_secret_change_in_production'` 时，当 `NODE_ENV=production` 拒绝启动并输出明确错误
2. THE API Server `main.ts` SHALL 在启动时校验 `JWT_SECRET` 长度 >= 32 字符（生产模式）
3. THE gateway-bridge `mirage-os.yaml` SHALL 将 `grpc.tls_enabled` 默认值改为 `true`
4. THE gateway-bridge SHALL 在 `MIRAGE_ENV=production` 且 `rest.internal_secret` 为空时拒绝启动
