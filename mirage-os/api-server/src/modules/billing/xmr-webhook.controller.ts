import { Controller, Post, Body, Get, Param, Query, HttpCode, NotFoundException, GoneException } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';

/**
 * XMR 异步确认 Webhook + 阅后即焚配置交付
 * 
 * 流程:
 * 1. Go 端 MoneroManager 确认到账后，调用 POST /webhook/xmr/confirmed
 * 2. NestJS 触发自动化配置流水线
 * 3. 用户通过一次性链接 GET /delivery/:token?key=xxx 获取配置
 */
@Controller('webhook')
export class XMRWebhookController {
  constructor(private prisma: PrismaService) {}

  /**
   * XMR 到账确认回调（由 Go 端 MoneroManager 调用）
   * 内部接口，不对外暴露
   */
  @Post('xmr/confirmed')
  @HttpCode(200)
  async onXMRConfirmed(
    @Body() body: {
      userId: string;
      txHash: string;
      amountXmr: number;
      amountUsd: number;
      confirmations: number;
    },
  ) {
    // 1. 记录充值确认
    await this.prisma.deposit.updateMany({
      where: { txHash: body.txHash, userId: body.userId },
      data: { status: 'CONFIRMED' },
    });

    // 2. 增加用户余额
    await this.prisma.user.update({
      where: { id: body.userId },
      data: {
        remainingQuota: { increment: body.amountUsd },
        totalDeposit: { increment: body.amountUsd },
      },
    });

    // 3. 自动分配蜂窝（如果用户尚未分配）
    const user = await this.prisma.user.findUnique({
      where: { id: body.userId },
      select: { cellId: true },
    });

    if (!user?.cellId) {
      await this.autoAssignCell(body.userId);
    }

    // 4. 触发 Go Provisioner 生成阅后即焚配置链接
    try {
      const provisionerUrl = process.env.PROVISIONER_URL || 'http://localhost:18443';
      await fetch(`${provisionerUrl}/internal/provision`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          uid: body.userId,
          amount_piconero: Math.round(body.amountXmr * 1e12),
        }),
      });
    } catch (err) {
      // Provisioner 调用失败不阻塞充值确认，用户可手动触发
      console.error('[XMR Webhook] Provisioner 调用失败:', err);
    }

    return { status: 'ok', message: 'provisioning triggered' };
  }

  /**
   * 自动分配蜂窝 — 选择负载最低的可用蜂窝
   */
  private async autoAssignCell(userId: string) {
    // 查找有空位的蜂窝
    const cells = await this.prisma.cell.findMany({
      include: { _count: { select: { users: true } } },
      orderBy: { createdAt: 'asc' },
    });

    const available = cells.find((c) => c._count.users < c.maxUsers);
    if (!available) return;

    await this.prisma.user.update({
      where: { id: userId },
      data: { cellId: available.id },
    });
  }
}
