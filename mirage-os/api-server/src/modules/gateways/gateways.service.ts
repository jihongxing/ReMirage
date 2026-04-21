import { Injectable, NotFoundException } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';
import { GatewayStatus } from '@prisma/client';

export interface GatewayQueryDto {
  cellId?: string;
  status?: GatewayStatus;
  page?: number;
  limit?: number;
}

@Injectable()
export class GatewaysService {
  constructor(private prisma: PrismaService) {}

  async findAll(query: GatewayQueryDto) {
    const where: any = {};
    if (query.cellId) where.cellId = query.cellId;
    if (query.status) where.status = query.status;

    const page = query.page ?? 1;
    const limit = query.limit ?? 20;

    return this.prisma.gateway.findMany({
      where,
      include: { cell: true },
      skip: (page - 1) * limit,
      take: limit,
    });
  }

  async findOne(id: string) {
    const gw = await this.prisma.gateway.findUnique({
      where: { id },
      include: { cell: true },
    });
    if (!gw) throw new NotFoundException('Gateway 不存在');
    return gw;
  }

  async markOfflineGateways(): Promise<number> {
    const threshold = new Date(Date.now() - 300 * 1000);
    const result = await this.prisma.gateway.updateMany({
      where: {
        status: { not: 'OFFLINE' },
        lastHeartbeat: { lt: threshold },
      },
      data: { status: 'OFFLINE' },
    });
    return result.count;
  }
}
