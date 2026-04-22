import { Controller, Post, Body, Get, Param, Query, HttpCode, NotFoundException, GoneException, UseGuards } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';
import { InternalHMACGuard, signInternalRequest } from '../../common/internal-hmac.guard';

/**
 * XMR 异步确认 Webhook + 阅后即焚配置交付
 */
@Controller('webhook')
@UseGuards(InternalHMACGuard)
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
    // 仅保留 WebSocket 通知功能，不再直接修改用户余额
    // 余额落账由 Go 端 monero_manager.confirmDeposit() 单一真相源处理

    // 1. 更新充值状态（仅状态标记，不涉及余额）
    await this.prisma.deposit.updateMany({
      where: { txHash: body.txHash, userId: body.userId, status: 'PENDING' },
      data: { status: 'CONFIRMED' },
    });

    // 2. 自动分配蜂窝（如果用户尚未分配）
    const user = await this.prisma.user.findUnique({
      where: { id: body.userId },
      select: { cellId: true },
    });

    if (!user?.cellId) {
      await this.autoAssignCell(body.userId);
    }

    // 3. 触发 Go Provisioner 生成阅后即焚配置链接（带 HMAC 签名）
    try {
      const provisionerUrl = process.env.PROVISIONER_URL || 'http://localhost:18443';
      const bodyStr = JSON.stringify({
        uid: body.userId,
        amount_piconero: Math.round(body.amountXmr * 1e12),
      });
      const hmacSecret = process.env.INTERNAL_HMAC_SECRET || '';
      const hmacHeaders = hmacSecret ? signInternalRequest(bodyStr, hmacSecret) : {};
      await fetch(`${provisionerUrl}/internal/provision`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...hmacHeaders },
        redirect: 'error',
        body: bodyStr,
      });
    } catch (err) {
      console.error('[XMR Webhook] Provisioner 调用失败:', err);
    }

    return { status: 'ok', message: 'notification sent' };
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
