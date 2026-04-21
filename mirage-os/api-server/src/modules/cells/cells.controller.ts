import {
  Controller,
  Get,
  Post,
  Body,
  Param,
  Query,
  UseGuards,
} from '@nestjs/common';
import { CellsService, CreateCellDto } from './cells.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { RolesGuard } from '../auth/roles.guard';
import { Permissions } from '../auth/permissions.decorator';
import { Permission } from '../auth/rbac-matrix';
import { PaginationDto } from '../../common/pagination.dto';

@Controller('cells')
@UseGuards(JwtAuthGuard, RolesGuard)
export class CellsController {
  constructor(private cellsService: CellsService) {}

  @Post()
  @Permissions(Permission.CELL_WRITE)
  create(@Body() dto: CreateCellDto) {
    return this.cellsService.create(dto);
  }

  @Get()
  @Permissions(Permission.CELL_READ)
  async findAll(@Query() pagination: PaginationDto) {
    const cells = await this.cellsService.findAll(
      pagination.page,
      pagination.limit,
    );
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
  @Permissions(Permission.CELL_WRITE)
  assign(@Param('id') id: string, @Body('userId') userId: string) {
    return this.cellsService.assignUser(id, userId);
  }
}
