import { Controller, Get, Param, Query, UseGuards } from '@nestjs/common';
import { GatewaysService } from './gateways.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { RolesGuard } from '../auth/roles.guard';
import { Permissions } from '../auth/permissions.decorator';
import { Permission } from '../auth/rbac-matrix';
import { GatewayStatus } from '@prisma/client';
import { PaginationDto } from '../../common/pagination.dto';

@Controller('gateways')
@UseGuards(JwtAuthGuard, RolesGuard)
@Permissions(Permission.GATEWAY_READ)
export class GatewaysController {
  constructor(private gatewaysService: GatewaysService) {}

  @Get()
  findAll(
    @Query() pagination: PaginationDto,
    @Query('cellId') cellId?: string,
    @Query('status') status?: GatewayStatus,
  ) {
    return this.gatewaysService.findAll({
      cellId,
      status,
      page: pagination.page,
      limit: pagination.limit,
    });
  }

  @Get('topology/by-cell/:cellId')
  async getGatewaysByCell(@Param('cellId') cellId: string) {
    return this.gatewaysService.findByCellOnline(cellId);
  }

  @Get('topology/online')
  async getOnlineGateways() {
    return this.gatewaysService.findAllOnline();
  }

  @Get('push-logs')
  async getPushLogs(@Query('limit') limit?: string) {
    const parsedLimit = limit ? parseInt(limit, 10) : 50;
    return this.gatewaysService.getRecentPushLogs(parsedLimit);
  }

  @Get(':id')
  findOne(@Param('id') id: string) {
    return this.gatewaysService.findOne(id);
  }
}
