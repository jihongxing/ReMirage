import { Controller, Get, Param, Query, UseGuards } from '@nestjs/common';
import { GatewaysService } from './gateways.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { GatewayStatus } from '@prisma/client';

@Controller('gateways')
@UseGuards(JwtAuthGuard)
export class GatewaysController {
  constructor(private gatewaysService: GatewaysService) {}

  @Get()
  findAll(
    @Query('cellId') cellId?: string,
    @Query('status') status?: GatewayStatus,
  ) {
    return this.gatewaysService.findAll({ cellId, status });
  }

  @Get(':id')
  findOne(@Param('id') id: string) {
    return this.gatewaysService.findOne(id);
  }
}
