import { Injectable, NotFoundException } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';

@Injectable()
export class UsersService {
  constructor(private prisma: PrismaService) {}

  async findAll(page = 1, limit = 20) {
    const skip = (page - 1) * limit;
    const [users, total] = await Promise.all([
      this.prisma.user.findMany({
        skip,
        take: limit,
        select: {
          id: true,
          username: true,
          remainingQuota: true,
          totalDeposit: true,
          totalConsumed: true,
          cellId: true,
          isActive: true,
          createdAt: true,
        },
      }),
      this.prisma.user.count(),
    ]);
    return { data: users, total, page, limit };
  }

  async findOne(id: string) {
    const user = await this.prisma.user.findUnique({
      where: { id },
      select: {
        id: true,
        username: true,
        remainingQuota: true,
        totalDeposit: true,
        totalConsumed: true,
        cellId: true,
        cell: true,
        isActive: true,
        isOperator: true,
        inviteCodeUsed: true,
        invitedBy: true,
        inviteRoot: true,
        inviteDepth: true,
        observationEndsAt: true,
        createdAt: true,
        updatedAt: true,
        // 排除: passwordHash, totpSecret, ed25519Pubkey
      },
    });
    if (!user) throw new NotFoundException('用户不存在');
    return user;
  }

  async bindPubkey(id: string, pubkey: string) {
    const user = await this.prisma.user.findUnique({ where: { id } });
    if (!user) throw new NotFoundException('用户不存在');
    return this.prisma.user.update({
      where: { id },
      data: { ed25519Pubkey: pubkey },
    });
  }

  async deactivate(id: string) {
    const user = await this.prisma.user.findUnique({ where: { id } });
    if (!user) throw new NotFoundException('用户不存在');
    return this.prisma.user.update({
      where: { id },
      data: { isActive: false },
    });
  }
}
