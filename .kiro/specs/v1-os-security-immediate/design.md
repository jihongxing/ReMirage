# 设计文档：OS 安全紧急封洞

## 概述

本设计覆盖 Mirage-OS 侧 5 项紧急安全整改。改动分布在 NestJS API Server 和 Go Gateway Bridge 两个服务中。

## 模块 1：权限升级路径修复（需求 1）

### 改动范围

- `mirage-os/api-server/src/modules/auth/breach.service.ts`：validateToken 修复
- `mirage-os/api-server/src/modules/auth/auth.service.ts`：login 增加 role
- `mirage-os/api-server/src/modules/auth/jwt.strategy.ts`：validate 提取 role

### 设计细节

#### validateToken 修复

当前 `validateToken` 续签时硬编码 `role: 'admin'`：

```typescript
const newToken = this.jwtService.sign(
  { sub: user.id, cell_id: user.cellId, role: 'admin' }, // BUG: 硬编码 admin
  { expiresIn: '24h' },
);
```

改为保持原 token 的 role：

```typescript
async validateToken(token: string) {
  const payload = this.jwtService.verify(token);
  // 保持原 role，不升级
  const originalRole = payload.role || 'user';
  const newToken = this.jwtService.sign(
    { sub: user.id, cell_id: user.cellId, role: originalRole },
    { expiresIn: '24h' },
  );
  return { success: true, token: newToken, role: originalRole, ... };
}
```

#### login 增加 role

```typescript
const payload = { sub: user.id, cell_id: user.cellId, role: 'user' };
```

#### JwtStrategy 提取 role

```typescript
async validate(payload: { sub: string; cell_id: string; role?: string }) {
  return { userId: payload.sub, cellId: payload.cell_id, role: payload.role || 'user' };
}
```

---

## 模块 2：RBAC 角色校验（需求 2）

### 改动范围

- `mirage-os/api-server/src/modules/auth/roles.guard.ts`（新建）
- `mirage-os/api-server/src/modules/auth/roles.decorator.ts`（新建）
- `mirage-os/api-server/src/modules/users/users.controller.ts`：增加 @Roles
- 其他管理类 Controller

### 设计细节

#### Roles 装饰器

```typescript
// roles.decorator.ts
import { SetMetadata } from '@nestjs/common';
export const ROLES_KEY = 'roles';
export const Roles = (...roles: string[]) => SetMetadata(ROLES_KEY, roles);
```

#### RolesGuard

```typescript
// roles.guard.ts
@Injectable()
export class RolesGuard implements CanActivate {
  constructor(private reflector: Reflector) {}

  canActivate(context: ExecutionContext): boolean {
    const requiredRoles = this.reflector.getAllAndOverride<string[]>(ROLES_KEY, [
      context.getHandler(),
      context.getClass(),
    ]);
    if (!requiredRoles) return true; // 无角色要求则放行

    const { user } = context.switchToHttp().getRequest();
    return requiredRoles.includes(user.role);
  }
}
```

#### 应用到 Controller

```typescript
@Controller('users')
@UseGuards(JwtAuthGuard, RolesGuard)
@Roles('admin')
export class UsersController { ... }
```

---

## 模块 3：内部接口收口（需求 3）

### 改动范围

- `mirage-os/gateway-bridge/cmd/bridge/main.go`：REST 绑定地址改为 127.0.0.1
- `mirage-os/gateway-bridge/pkg/rest/middleware.go`（新建）：共享密钥校验中间件
- `mirage-os/gateway-bridge/pkg/config/config.go`：增加 InternalSecret 配置
- `mirage-os/gateway-bridge/configs/mirage-os.yaml`：增加配置项

### 设计细节

#### REST 绑定地址

```go
restAddr := "127.0.0.1:7000" // 从 ":7000" 改为 "127.0.0.1:7000"
```

#### 共享密钥中间件

```go
// pkg/rest/middleware.go
func InternalAuthMiddleware(secret string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if r.Header.Get("X-Internal-Secret") != secret {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

#### gRPC 生产模式强制 TLS

```go
if os.Getenv("MIRAGE_ENV") == "production" && !cfg.GRPC.TLSEnabled {
    log.Fatalf("[FATAL] 生产模式禁止禁用 gRPC TLS")
}
```

---

## 模块 4：速率限制（需求 4）

### 改动范围

- `mirage-os/api-server/src/app.module.ts`：引入 ThrottlerModule
- `mirage-os/api-server/src/modules/auth/auth.controller.ts`：应用 @Throttle
- `mirage-os/api-server/package.json`：增加 @nestjs/throttler 依赖

### 设计细节

```typescript
// app.module.ts
import { ThrottlerModule, ThrottlerGuard } from '@nestjs/throttler';
import { APP_GUARD } from '@nestjs/core';

@Module({
  imports: [
    ThrottlerModule.forRoot([{ ttl: 60000, limit: 10 }]), // 全局：60s 内 10 次
    // ...
  ],
  providers: [
    { provide: APP_GUARD, useClass: ThrottlerGuard }, // 全局启用
  ],
})
```

对认证接口单独设置更严格的限制：

```typescript
@Controller('auth')
@Throttle({ default: { ttl: 60000, limit: 10 } })
export class AuthController { ... }
```

---

## 模块 5：默认密钥清理（需求 5）

### 改动范围

- `mirage-os/api-server/src/modules/auth/jwt.strategy.ts`：启动校验
- `mirage-os/api-server/src/main.ts`：启动校验
- `mirage-os/gateway-bridge/cmd/bridge/main.go`：启动校验
- `mirage-os/gateway-bridge/configs/mirage-os.yaml`：默认值修改

### 设计细节

#### NestJS 启动校验

```typescript
// jwt.strategy.ts
const secret = process.env.JWT_SECRET || 'dev_jwt_secret_change_in_production';
if (process.env.NODE_ENV === 'production') {
  if (secret === 'dev_jwt_secret_change_in_production' || secret.length < 32) {
    throw new Error('❌ 生产模式必须设置 JWT_SECRET（>= 32 字符）');
  }
}
```

#### Gateway Bridge 启动校验

```go
if os.Getenv("MIRAGE_ENV") == "production" {
    if cfg.REST.InternalSecret == "" {
        log.Fatalf("[FATAL] 生产模式必须设置 rest.internal_secret")
    }
}
```
