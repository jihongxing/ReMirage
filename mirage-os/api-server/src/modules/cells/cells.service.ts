import { Injectable, ConflictException } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';
import { CellLevel, Prisma } from '@prisma/client';

export interface CreateCellDto {
  name: string;
  region: string;
  level: CellLevel;
  maxUsers?: number;
  maxDomains?: number;
}

const LEVEL_MULTIPLIER: Record<CellLevel, number> = {
  STANDARD: 1.0,
  PLATINUM: 1.5,
  DIAMOND: 2.0,
};

@Injectable()
export class CellsService {
  constructor(private prisma: PrismaService) {}

  async create(dto: CreateCellDto) {
    const multiplier = LEVEL_MULTIPLIER[dto.level] || 1.0;
    return this.prisma.cell.create({
      data: {
        name: dto.name,
        region: dto.region,
        level: dto.level,
        costMultiplier: new Prisma.Decimal(multiplier),
        maxUsers: dto.maxUsers ?? 50,
        maxDomains: dto.maxDomains ?? 15,
      },
    });
  }

  async findAll() {
    const cells = await this.prisma.cell.findMany({
      include: {
        _count: {
          select: { users: true, gateways: true },
        },
      },
    });
    return cells.map((c) => ({
      ...c,
      userCount: c._count.users,
      gatewayCount: c._count.gateways,
    }));
  }

  async assignUser(cellId: string, userId: string) {
    const cell = await this.prisma.cell.findUnique({
      where: { id: cellId },
      include: { _count: { select: { users: true } } },
    });
    if (!cell) throw new ConflictException('蜂窝不存在');
    if (cell._count.users >= cell.maxUsers) {
      throw new ConflictException('蜂窝已满');
    }

    await this.prisma.user.update({
      where: { id: userId },
      data: { cellId },
    });
  }
}
