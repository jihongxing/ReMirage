import { Injectable, BadRequestException } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';
import { Prisma } from '@prisma/client';

export interface LogQueryDto {
  page: number;
  limit: number;
  startDate?: Date;
  endDate?: Date;
}

export interface RechargeDto {
  quotaGb: number;
  price: number;
  cellLevel?: string;
}

@Injectable()
export class BillingService {
  constructor(private prisma: PrismaService) {}

  async getLogs(userId: string, query: LogQueryDto) {
    const where: any = { userId };
    if (query.startDate || query.endDate) {
      where.createdAt = {};
      if (query.startDate) where.createdAt.gte = query.startDate;
      if (query.endDate) where.createdAt.lte = query.endDate;
    }

    const skip = (query.page - 1) * query.limit;
    const [logs, total] = await Promise.all([
      this.prisma.billingLog.findMany({
        where,
        skip,
        take: query.limit,
        orderBy: { createdAt: 'desc' },
      }),
      this.prisma.billingLog.count({ where }),
    ]);

    return { data: logs, total, page: query.page, limit: query.limit };
  }

  async getQuota(userId: string) {
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
      select: {
        remainingQuota: true,
        totalDeposit: true,
        totalConsumed: true,
      },
    });
    return user;
  }

  async recharge(userId: string, dto: RechargeDto) {
    if (!dto.quotaGb || dto.quotaGb <= 0) {
      throw new BadRequestException('quotaGb must be > 0');
    }
    if (!dto.price || dto.price <= 0) {
      throw new BadRequestException('price must be > 0');
    }

    const cellLevel = dto.cellLevel || 'STANDARD';

    return this.prisma.$transaction(async (tx) => {
      const purchase = await tx.quotaPurchase.create({
        data: {
          userId,
          quotaGb: new Prisma.Decimal(dto.quotaGb),
          price: new Prisma.Decimal(dto.price),
          cellLevel,
        },
      });

      await tx.user.update({
        where: { id: userId },
        data: {
          remainingQuota: { increment: new Prisma.Decimal(dto.quotaGb) },
          totalDeposit: { increment: new Prisma.Decimal(dto.price) },
        },
      });

      return purchase;
    });
  }

  async getPurchases(userId: string) {
    return this.prisma.quotaPurchase.findMany({
      where: { userId },
      orderBy: { createdAt: 'desc' },
    });
  }
}
