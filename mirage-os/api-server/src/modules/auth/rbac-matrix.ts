export enum Role {
  USER = 'user',
  ADMIN = 'admin',
  OPERATOR = 'operator',
  AUDITOR = 'auditor',
}

export enum Permission {
  USER_READ = 'user:read',
  USER_WRITE = 'user:write',
  GATEWAY_READ = 'gateway:read',
  GATEWAY_WRITE = 'gateway:write',
  CELL_READ = 'cell:read',
  CELL_WRITE = 'cell:write',
  THREAT_READ = 'threat:read',
  THREAT_WRITE = 'threat:write',
  BILLING_READ = 'billing:read',
  BILLING_WRITE = 'billing:write',
  AUDIT_READ = 'audit:read',
  SYSTEM_ADMIN = 'system:admin',
}

export const RBAC_MATRIX: Record<Role, Permission[]> = {
  [Role.ADMIN]: Object.values(Permission),
  [Role.OPERATOR]: [
    Permission.GATEWAY_READ,
    Permission.GATEWAY_WRITE,
    Permission.CELL_READ,
    Permission.CELL_WRITE,
    Permission.THREAT_READ,
    Permission.THREAT_WRITE,
  ],
  [Role.AUDITOR]: [
    Permission.USER_READ,
    Permission.GATEWAY_READ,
    Permission.CELL_READ,
    Permission.THREAT_READ,
    Permission.BILLING_READ,
    Permission.AUDIT_READ,
  ],
  [Role.USER]: [Permission.BILLING_READ],
};
