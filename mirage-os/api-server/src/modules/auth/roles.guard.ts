import {
  Injectable,
  CanActivate,
  ExecutionContext,
  ForbiddenException,
} from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { PERMISSIONS_KEY } from './permissions.decorator';
import { Permission, RBAC_MATRIX, Role } from './rbac-matrix';

@Injectable()
export class RolesGuard implements CanActivate {
  constructor(private reflector: Reflector) {}

  canActivate(context: ExecutionContext): boolean {
    const requiredPermissions = this.reflector.getAllAndOverride<Permission[]>(
      PERMISSIONS_KEY,
      [context.getHandler(), context.getClass()],
    );
    if (!requiredPermissions || requiredPermissions.length === 0) return true;

    const { user } = context.switchToHttp().getRequest();
    if (!user) throw new ForbiddenException('权限不足');

    const userRole = user.role as Role;
    const userPermissions = RBAC_MATRIX[userRole] || [];

    if (!requiredPermissions.every((p) => userPermissions.includes(p))) {
      throw new ForbiddenException('权限不足');
    }
    return true;
  }
}
