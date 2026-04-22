import { Injectable, Logger } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';

export interface AuditLogInput {
  operatorId: string;
  operatorRole: string;
  sourceIp: string;
  targetResource: string;
  actionType: string;
  actionParams?: Record<string, any>;
  result: 'success' | 'failure' | 'denied';
}

export interface AuditQueryParams {
  startDate?: string;
  endDate?: string;
  operatorId?: string;
  actionType?: string;
  page?: number;
  limit?: number;
}

@Injectable()
export class AuditService {
  private readonly logger = new Logger(AuditService.name);

  constructor(private prisma: PrismaService) {}

  /** 敏感键黑名单 */
  private static readonly SENSITIVE_KEYS = new Set([
    'password', 'passwordHash', 'password_hash',
    'totpCode', 'totp_code', 'totpSecret', 'totp_secret',
    'token', 'secret', 'signature', 'key',
    'ed25519Pubkey', 'ed25519_pubkey',
  ]);

  async log(input: AuditLogInput): Promise<void> {
    try {
      // 字段级红线过滤：写入前扫描并移除敏感键
      const sanitizedParams = input.actionParams
        ? this.stripSensitiveKeys(input.actionParams)
        : undefined;

      await this.prisma.auditLog.create({
        data: {
          operatorId: input.operatorId ?? 'anonymous',
          operatorRole: input.operatorRole ?? 'unknown',
          sourceIp: input.sourceIp ?? '0.0.0.0',
          targetResource: input.targetResource,
          actionType: input.actionType,
          actionParams: sanitizedParams ?? undefined,
          result: input.result,
        },
      });
    } catch (error) {
      this.logger.error('审计日志写入失败', error);
    }
  }

  private stripSensitiveKeys(obj: Record<string, any>): Record<string, any> {
    if (!obj || typeof obj !== 'object') return obj;
    const result: Record<string, any> = {};
    for (const [key, value] of Object.entries(obj)) {
      if (AuditService.SENSITIVE_KEYS.has(key)) continue;
      result[key] = typeof value === 'object' && value !== null
        ? this.stripSensitiveKeys(value)
        : value;
    }
    return result;
  }

  async query(params: AuditQueryParams) {
    const page = params.page ?? 1;
    const limit = Math.min(params.limit ?? 20, 100);
    const skip = (page - 1) * limit;

    const where: Record<string, any> = {};

    if (params.startDate || params.endDate) {
      where.createdAt = {};
      if (params.startDate) where.createdAt.gte = new Date(params.startDate);
      if (params.endDate) where.createdAt.lte = new Date(params.endDate);
    }
    if (params.operatorId) where.operatorId = params.operatorId;
    if (params.actionType) where.actionType = params.actionType;

    const [data, total] = await Promise.all([
      this.prisma.auditLog.findMany({
        where,
        orderBy: { createdAt: 'desc' },
        skip,
        take: limit,
      }),
      this.prisma.auditLog.count({ where }),
    ]);

    return { data, total, page, limit };
  }
}
