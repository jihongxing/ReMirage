import { Injectable } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';

export interface ThreatQueryDto {
  page: number;
  limit: number;
  threatType?: string;
  isBanned?: boolean;
  severity?: number;
}

@Injectable()
export class ThreatsService {
  constructor(private prisma: PrismaService) {}

  async findAll(query: ThreatQueryDto) {
    const where: any = {};
    if (query.threatType) where.threatType = query.threatType;
    if (query.isBanned !== undefined) where.isBanned = query.isBanned;
    if (query.severity !== undefined) where.severity = { gte: query.severity };

    const skip = (query.page - 1) * query.limit;
    const [threats, total] = await Promise.all([
      this.prisma.threatIntel.findMany({
        where,
        skip,
        take: query.limit,
        orderBy: { lastSeen: 'desc' },
      }),
      this.prisma.threatIntel.count({ where }),
    ]);

    return { data: threats, total, page: query.page, limit: query.limit };
  }

  async getStats() {
    const [byType, bannedCount, activeUsers] = await Promise.all([
      this.prisma.threatIntel.groupBy({
        by: ['threatType'],
        _count: { id: true },
      }),
      this.prisma.threatIntel.count({ where: { isBanned: true } }),
      this.prisma.user.count({ where: { isActive: true } }),
    ]);

    return {
      byType: byType.map((t) => ({
        threatType: t.threatType,
        count: t._count.id,
      })),
      totalBanned: bannedCount,
      activeUsers,
    };
  }
}
