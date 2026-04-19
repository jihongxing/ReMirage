import { Controller, Get, Post, Body, Param, UseGuards } from '@nestjs/common';
import { CellsService, CreateCellDto } from './cells.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';

@Controller('cells')
@UseGuards(JwtAuthGuard)
export class CellsController {
  constructor(private cellsService: CellsService) {}

  @Post()
  create(@Body() dto: CreateCellDto) {
    return this.cellsService.create(dto);
  }

  @Get()
  async findAll() {
    const cells = await this.cellsService.findAll();
    // 统一前端契约字段名: user_count, gateway_count, health
    return cells.map((c: any) => ({
      id: c.id,
      name: c.name,
      region: c.region,
      level: c.level,
      user_count: c.userCount ?? 0,
      max_users: c.maxUsers ?? 50,
      gateway_count: c.gatewayCount ?? 0,
      health: c.health ?? 'HEALTHY',
    }));
  }

  @Post(':id/assign')
  assign(@Param('id') id: string, @Body('userId') userId: string) {
    return this.cellsService.assignUser(id, userId);
  }
}
