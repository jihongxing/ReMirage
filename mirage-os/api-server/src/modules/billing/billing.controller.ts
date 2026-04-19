import { Controller, Get, Post, Body, Query, Req, UseGuards } from '@nestjs/common';
import { BillingService, RechargeDto } from './billing.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';

@Controller('billing')
@UseGuards(JwtAuthGuard)
export class BillingController {
  constructor(private billingService: BillingService) {}

  @Get('logs')
  async getLogs(
    @Req() req: any,
    @Query('page') page?: string,
    @Query('limit') limit?: string,
    @Query('startDate') startDate?: string,
    @Query('endDate') endDate?: string,
  ) {
    const result = await this.billingService.getLogs(req.user.userId, {
      page: page ? parseInt(page) : 1,
      limit: limit ? parseInt(limit) : 100,
      startDate: startDate ? new Date(startDate) : undefined,
      endDate: endDate ? new Date(endDate) : undefined,
    });
    // 前端按数组消费，字段名转 snake_case
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
    // 统一前端契约字段名
    return {
      remaining_quota: user?.remainingQuota ?? 0,
      total_recharged: user?.totalDeposit ?? 0,
      total_consumed: user?.totalConsumed ?? 0,
    };
  }

  @Post('recharge')
  recharge(@Req() req: any, @Body() body: { amount: number }) {
    // 前端发 { amount }，转换为后端 RechargeDto
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
