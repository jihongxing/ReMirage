import { Controller, Get, Query, UseGuards } from '@nestjs/common';
import { ThreatsService } from './threats.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { RolesGuard } from '../auth/roles.guard';
import { Permissions } from '../auth/permissions.decorator';
import { Permission } from '../auth/rbac-matrix';
import { PaginationDto } from '../../common/pagination.dto';

@Controller('threats')
@UseGuards(JwtAuthGuard, RolesGuard)
@Permissions(Permission.THREAT_READ)
export class ThreatsController {
  constructor(private threatsService: ThreatsService) {}

  @Get()
  async findAll(
    @Query() pagination: PaginationDto,
    @Query('threatType') threatType?: string,
    @Query('isBanned') isBanned?: string,
    @Query('severity') severity?: string,
  ) {
    const result = await this.threatsService.findAll({
      page: pagination.page,
      limit: pagination.limit,
      threatType,
      isBanned: isBanned !== undefined ? isBanned === 'true' : undefined,
      severity: severity ? parseInt(severity) : undefined,
    });
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
    return {
      banned_count: stats.totalBanned,
      active_users: stats.activeUsers,
    };
  }
}
