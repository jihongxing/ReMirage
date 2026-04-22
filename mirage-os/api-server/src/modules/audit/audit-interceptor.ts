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

/** 额外需要审计的 GET 路径模式 */
const SENSITIVE_PATH_PATTERNS = [
  /\/threats/,
  /\/users\/[^/]+\/deactivate/,
  /\/gateways/,
  /\/audit/,
];

/** 敏感键黑名单 */
const SENSITIVE_KEYS = new Set([
  'password', 'passwordHash', 'password_hash',
  'totpCode', 'totp_code', 'totpSecret', 'totp_secret',
  'token', 'secret', 'signature', 'key',
  'ed25519Pubkey', 'ed25519_pubkey',
]);

/** 需要脱敏的路径前缀 */
const SENSITIVE_PATHS = ['/auth/', '/billing/'];

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

    const sanitizedParams = method !== 'GET'
      ? this.sanitizeBody(request.body, url)
      : undefined;

    const auditBase = {
      operatorId: request.user?.userId ?? request.user?.sub ?? 'anonymous',
      operatorRole: request.user?.role ?? 'unknown',
      sourceIp: request.ip ?? request.connection?.remoteAddress ?? '0.0.0.0',
      targetResource: url,
      actionType: `${method} ${request.route?.path ?? url}`,
      actionParams: sanitizedParams,
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

  /** 路径级脱敏：对敏感路径移除敏感键 */
  private sanitizeBody(body: any, path: string): any {
    if (!body || typeof body !== 'object') return body;

    const needsSanitize = SENSITIVE_PATHS.some((p) => path.includes(p));
    if (!needsSanitize) return this.stripSensitiveKeys(body);

    return this.stripSensitiveKeys(body);
  }

  /** 递归移除敏感键 */
  private stripSensitiveKeys(obj: any): any {
    if (!obj || typeof obj !== 'object') return obj;
    if (Array.isArray(obj)) return obj.map((item) => this.stripSensitiveKeys(item));

    const result: Record<string, any> = {};
    for (const [key, value] of Object.entries(obj)) {
      if (SENSITIVE_KEYS.has(key)) continue;
      result[key] = typeof value === 'object' ? this.stripSensitiveKeys(value) : value;
    }
    return result;
  }
}
