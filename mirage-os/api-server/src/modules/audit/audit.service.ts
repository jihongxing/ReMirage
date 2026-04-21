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

  async log(input: AuditLogInput): Promise<void> {
    try {
      await this.prisma.auditLog.create({
        data: {
          operatorId: input.operatorId ?? 'anonymous',
          operatorRole: input.operatorRole ?? 'unknown',
          sourceIp: input.sourceIp ?? '0.0.0.0',
          targetResource: input.targetResource,
          actionType: input.actionType,
          actionParams: input.actionParams ?? undefined,
          result: input.result,
        },
      });
    } catch (error) {
      // 审计日志写入失败不应阻断业务请求
      this.logger.error('审计日志写入失败', error);
    }
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
