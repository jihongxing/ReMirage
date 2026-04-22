import { Controller, Get, Post, Body, Query, Req, UseGuards, ForbiddenException } from '@nestjs/common';
import { BillingService, RechargeDto } from './billing.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { PaginationDto } from '../../common/pagination.dto';

@Controller('billing')
@UseGuards(JwtAuthGuard)
export class BillingController {
  constructor(private billingService: BillingService) {}

  @Get('logs')
  async getLogs(
    @Req() req: any,
    @Query() pagination: PaginationDto,
    @Query('startDate') startDate?: string,
    @Query('endDate') endDate?: string,
  ) {
    const result = await this.billingService.getLogs(req.user.userId, {
      page: pagination.page,
      limit: pagination.limit,
      startDate: startDate ? new Date(startDate) : undefined,
      endDate: endDate ? new Date(endDate) : undefined,
    });
    return result.data.map((log: any) => ({
      created_at: log.createdAt,
      user_id: log.userId,
      business_bytes: Number(log.businessBytes),
      defense_bytes: Number(log.defenseBytes),
      business_cost: Number(log.businessCost),
      defense_cost: Number(log.defenseCost),
      total_cost: Number(log.totalCost),
    }));
  }

  @Get('quota')
  async getQuota(@Req() req: any) {
    const user = await this.billingService.getQuota(req.user.userId);
    return {
      remaining_quota: user?.remainingQuota ?? 0,
      total_recharged: user?.totalDeposit ?? 0,
      total_consumed: user?.totalConsumed ?? 0,
    };
  }

  @Post('recharge')
  recharge(@Req() req: any, @Body() body: { amount: number }) {
    // 仅 admin 角色可调用
    if (req.user?.role !== 'admin') {
      throw new ForbiddenException('仅 admin 角色可调用 recharge 端点');
    }
    const dto: RechargeDto = {
      quotaGb: body.amount,
      price: body.amount,
    };
    return this.billingService.recharge(req.user.userId, dto);
  }

  @Get('purchases')
  getPurchases(@Req() req: any) {
    return this.billingService.getPurchases(req.user.userId);
  }
}
