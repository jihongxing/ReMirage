import { Role, Permission, RBAC_MATRIX } from './rbac-matrix';
import { RolesGuard } from './roles.guard';
import { OwnerGuard } from './owner.guard';
import { Reflector } from '@nestjs/core';
import { ExecutionContext, ForbiddenException } from '@nestjs/common';
import { PERMISSIONS_KEY } from './permissions.decorator';

// Helper: 构造 mock ExecutionContext
function createMockContext(
  user: { userId: string; role: string },
  permissions: Permission[] | undefined,
  params: Record<string, string> = {},
  body: Record<string, any> = {},
): ExecutionContext {
  const request = { user, params, body };
  return {
    switchToHttp: () => ({
      getRequest: () => request,
      getResponse: () => ({}),
      getNext: () => ({}),
    }),
    getHandler: () => ({}),
    getClass: () => ({}),
    getArgs: () => [],
    getArgByIndex: () => ({}),
    switchToRpc: () => ({} as any),
    switchToWs: () => ({} as any),
    getType: () => 'http' as any,
    // store permissions for reflector mock
    _permissions: permissions,
  } as any;
}

// ============================================================
// RBAC_MATRIX 权限矩阵测试
// ============================================================
describe('RBAC_MATRIX', () => {
  it('admin 拥有全部权限', () => {
    const allPermissions = Object.values(Permission);
    expect(RBAC_MATRIX[Role.ADMIN]).toEqual(allPermissions);
  });

  it('operator 拥有 gateway/cell/threat 读写权限', () => {
    const opPerms = RBAC_MATRIX[Role.OPERATOR];
    expect(opPerms).toContain(Permission.GATEWAY_READ);
    expect(opPerms).toContain(Permission.GATEWAY_WRITE);
    expect(opPerms).toContain(Permission.CELL_READ);
    expect(opPerms).toContain(Permission.CELL_WRITE);
    expect(opPerms).toContain(Permission.THREAT_READ);
    expect(opPerms).toContain(Permission.THREAT_WRITE);
  });

  it('operator 不拥有 user/billing/audit/system 权限', () => {
    const opPerms = RBAC_MATRIX[Role.OPERATOR];
    expect(opPerms).not.toContain(Permission.USER_READ);
    expect(opPerms).not.toContain(Permission.USER_WRITE);
    expect(opPerms).not.toContain(Permission.BILLING_READ);
    expect(opPerms).not.toContain(Permission.BILLING_WRITE);
    expect(opPerms).not.toContain(Permission.AUDIT_READ);
    expect(opPerms).not.toContain(Permission.SYSTEM_ADMIN);
  });

  it('auditor 拥有所有只读权限 + audit_read', () => {
    const audPerms = RBAC_MATRIX[Role.AUDITOR];
    expect(audPerms).toContain(Permission.USER_READ);
    expect(audPerms).toContain(Permission.GATEWAY_READ);
    expect(audPerms).toContain(Permission.CELL_READ);
    expect(audPerms).toContain(Permission.THREAT_READ);
    expect(audPerms).toContain(Permission.BILLING_READ);
    expect(audPerms).toContain(Permission.AUDIT_READ);
  });

  it('auditor 不拥有任何写权限', () => {
    const audPerms = RBAC_MATRIX[Role.AUDITOR];
    expect(audPerms).not.toContain(Permission.USER_WRITE);
    expect(audPerms).not.toContain(Permission.GATEWAY_WRITE);
    expect(audPerms).not.toContain(Permission.CELL_WRITE);
    expect(audPerms).not.toContain(Permission.THREAT_WRITE);
    expect(audPerms).not.toContain(Permission.BILLING_WRITE);
    expect(audPerms).not.toContain(Permission.SYSTEM_ADMIN);
  });

  it('user 仅拥有 billing_read', () => {
    const userPerms = RBAC_MATRIX[Role.USER];
    expect(userPerms).toEqual([Permission.BILLING_READ]);
  });
});

// ============================================================
// RolesGuard 测试
// ============================================================
describe('RolesGuard', () => {
  let guard: RolesGuard;
  let reflector: Reflector;

  beforeEach(() => {
    reflector = new Reflector();
    guard = new RolesGuard(reflector);
  });

  function runGuard(
    role: string,
    permissions: Permission[] | undefined,
  ): boolean {
    const ctx = createMockContext(
      { userId: 'u1', role },
      permissions,
    );
    jest
      .spyOn(reflector, 'getAllAndOverride')
      .mockReturnValue(permissions as any);
    return guard.canActivate(ctx);
  }

  it('无权限要求时放行所有角色', () => {
    expect(runGuard('user', undefined)).toBe(true);
    expect(runGuard('admin', undefined)).toBe(true);
  });

  it('admin 可访问任何受保护接口', () => {
    expect(runGuard('admin', [Permission.USER_READ])).toBe(true);
    expect(runGuard('admin', [Permission.SYSTEM_ADMIN])).toBe(true);
    expect(runGuard('admin', [Permission.GATEWAY_WRITE])).toBe(true);
  });

  it('operator 可访问 gateway/cell/threat 接口', () => {
    expect(runGuard('operator', [Permission.GATEWAY_READ])).toBe(true);
    expect(runGuard('operator', [Permission.CELL_WRITE])).toBe(true);
    expect(runGuard('operator', [Permission.THREAT_READ])).toBe(true);
  });

  it('operator 不可访问 user/billing/audit 接口', () => {
    expect(() => runGuard('operator', [Permission.USER_READ])).toThrow(
      ForbiddenException,
    );
    expect(() => runGuard('operator', [Permission.BILLING_READ])).toThrow(
      ForbiddenException,
    );
    expect(() => runGuard('operator', [Permission.AUDIT_READ])).toThrow(
      ForbiddenException,
    );
  });

  it('auditor 可访问只读接口', () => {
    expect(runGuard('auditor', [Permission.USER_READ])).toBe(true);
    expect(runGuard('auditor', [Permission.GATEWAY_READ])).toBe(true);
    expect(runGuard('auditor', [Permission.THREAT_READ])).toBe(true);
    expect(runGuard('auditor', [Permission.AUDIT_READ])).toBe(true);
  });

  it('auditor 不可访问写接口', () => {
    expect(() => runGuard('auditor', [Permission.USER_WRITE])).toThrow(
      ForbiddenException,
    );
    expect(() => runGuard('auditor', [Permission.GATEWAY_WRITE])).toThrow(
      ForbiddenException,
    );
    expect(() => runGuard('auditor', [Permission.SYSTEM_ADMIN])).toThrow(
      ForbiddenException,
    );
  });

  it('user 仅可访问 billing_read', () => {
    expect(runGuard('user', [Permission.BILLING_READ])).toBe(true);
  });

  it('user 不可访问其他接口', () => {
    expect(() => runGuard('user', [Permission.USER_READ])).toThrow(
      ForbiddenException,
    );
    expect(() => runGuard('user', [Permission.GATEWAY_READ])).toThrow(
      ForbiddenException,
    );
    expect(() => runGuard('user', [Permission.CELL_READ])).toThrow(
      ForbiddenException,
    );
    expect(() => runGuard('user', [Permission.THREAT_READ])).toThrow(
      ForbiddenException,
    );
  });

  it('未知角色被拒绝', () => {
    expect(() => runGuard('unknown', [Permission.USER_READ])).toThrow(
      ForbiddenException,
    );
  });

  it('无 user 对象时抛出 ForbiddenException', () => {
    const ctx = createMockContext(
      null as any,
      [Permission.USER_READ],
    );
    jest
      .spyOn(reflector, 'getAllAndOverride')
      .mockReturnValue([Permission.USER_READ]);
    expect(() => guard.canActivate(ctx)).toThrow(ForbiddenException);
  });
});

// ============================================================
// OwnerGuard 测试
// ============================================================
describe('OwnerGuard', () => {
  const guard = new OwnerGuard();

  function runOwnerGuard(
    user: { userId: string; role: string },
    params: Record<string, string> = {},
    body: Record<string, any> = {},
  ): boolean {
    const ctx = createMockContext(user, undefined, params, body);
    return guard.canActivate(ctx);
  }

  it('admin 跳过 owner check', () => {
    expect(runOwnerGuard({ userId: 'u1', role: 'admin' }, { id: 'u2' })).toBe(
      true,
    );
  });

  it('operator 跳过 owner check', () => {
    expect(
      runOwnerGuard({ userId: 'u1', role: 'operator' }, { id: 'u2' }),
    ).toBe(true);
  });

  it('auditor 跳过 owner check', () => {
    expect(
      runOwnerGuard({ userId: 'u1', role: 'auditor' }, { id: 'u2' }),
    ).toBe(true);
  });

  it('user 访问自己的资源 → 放行', () => {
    expect(runOwnerGuard({ userId: 'u1', role: 'user' }, { id: 'u1' })).toBe(
      true,
    );
  });

  it('user 访问他人资源 → 拒绝', () => {
    expect(runOwnerGuard({ userId: 'u1', role: 'user' }, { id: 'u2' })).toBe(
      false,
    );
  });

  it('user 通过 body.userId 访问他人资源 → 拒绝', () => {
    expect(
      runOwnerGuard({ userId: 'u1', role: 'user' }, {}, { userId: 'u2' }),
    ).toBe(false);
  });

  it('user 无资源标识时放行', () => {
    expect(runOwnerGuard({ userId: 'u1', role: 'user' })).toBe(true);
  });

  it('无 user 对象时拒绝', () => {
    expect(runOwnerGuard(null as any)).toBe(false);
  });
});
