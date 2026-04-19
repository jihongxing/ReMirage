import { Controller, Get, Query, UseGuards } from '@nestjs/common';
import { ThreatsService } from './threats.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';

@Controller('threats')
@UseGuards(JwtAuthGuard)
export class ThreatsController {
  constructor(private threatsService: ThreatsService) {}

  @Get()
  async findAll(
    @Query('page') page?: string,
    @Query('limit') limit?: string,
    @Query('threatType') threatType?: string,
    @Query('isBanned') isBanned?: string,
    @Query('severity') severity?: string,
  ) {
    const result = await this.threatsService.findAll({
      page: page ? parseInt(page) : 1,
      limit: limit ? parseInt(limit) : 200,
      threatType,
      isBanned: isBanned !== undefined ? isBanned === 'true' : undefined,
      severity: severity ? parseInt(severity) : undefined,
    });
    // 前端按数组消费，字段名转 snake_case
    return result.data.map((t: any) => ({
      source_ip: t.sourceIp,
      threat_type: t.threatType,
      severity: t.severity,
      hit_count: t.hitCount,
      is_banned: t.isBanned,
      last_seen: t.lastSeen,
    }));
  }

  @Get('stats')
  async getStats() {
    const stats = await this.threatsService.getStats();
    // 统一前端契约: { banned_count, active_users }
    return {
      banned_count: stats.totalBanned,
      active_users: stats.activeUsers,
    };
  }
}
