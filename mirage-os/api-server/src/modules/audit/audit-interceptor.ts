import {
  Injectable,
  NestInterceptor,
  ExecutionContext,
  CallHandler,
} from '@nestjs/common';
import { Observable } from 'rxjs';
import { tap, catchError } from 'rxjs/operators';
import { AuditService } from './audit.service';

/** HTTP 方法被视为敏感操作 */
const SENSITIVE_METHODS = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

/** 额外需要审计的 GET 路径模式（正则） */
const SENSITIVE_PATH_PATTERNS = [
  /\/threats/,
  /\/users\/[^/]+\/deactivate/,
  /\/gateways/,
  /\/audit/,
];

@Injectable()
export class AuditInterceptor implements NestInterceptor {
  constructor(private readonly auditService: AuditService) {}

  intercept(context: ExecutionContext, next: CallHandler): Observable<any> {
    const request = context.switchToHttp().getRequest();
    if (!request) return next.handle();

    const method: string = request.method;
    const url: string = request.url ?? '';

    if (!this.isSensitiveAction(method, url)) {
      return next.handle();
    }

    const auditBase = {
      operatorId: request.user?.userId ?? request.user?.sub ?? 'anonymous',
      operatorRole: request.user?.role ?? 'unknown',
      sourceIp: request.ip ?? request.connection?.remoteAddress ?? '0.0.0.0',
      targetResource: url,
      actionType: `${method} ${request.route?.path ?? url}`,
      actionParams: method !== 'GET' ? request.body : undefined,
    };

    return next.handle().pipe(
      tap(() => {
        this.auditService.log({ ...auditBase, result: 'success' });
      }),
      catchError((error) => {
        const result = error?.status === 403 ? 'denied' : 'failure';
        this.auditService.log({ ...auditBase, result });
        throw error;
      }),
    );
  }

  private isSensitiveAction(method: string, url: string): boolean {
    if (SENSITIVE_METHODS.has(method)) return true;
    return SENSITIVE_PATH_PATTERNS.some((pattern) => pattern.test(url));
  }
}
