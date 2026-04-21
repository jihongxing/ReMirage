import { Injectable, CanActivate, ExecutionContext } from '@nestjs/common';
import { Role } from './rbac-matrix';

@Injectable()
export class OwnerGuard implements CanActivate {
  canActivate(context: ExecutionContext): boolean {
    const request = context.switchToHttp().getRequest();
    const user = request.user;
    if (!user) return false;

    // admin/operator/auditor 跳过 owner check
    if (
      [Role.ADMIN, Role.OPERATOR, Role.AUDITOR].includes(user.role as Role)
    ) {
      return true;
    }

    // user 角色：检查资源归属
    const resourceUserId =
      request.params.userId || request.params.id || request.body?.userId;
    if (resourceUserId && resourceUserId !== user.userId) return false;

    return true;
  }
}
