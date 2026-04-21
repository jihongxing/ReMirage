import { Controller, Get, Query, UseGuards } from '@nestjs/common';
import { DomainsService } from './domains.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { RolesGuard } from '../auth/roles.guard';
import { Permissions } from '../auth/permissions.decorator';
import { Permission } from '../auth/rbac-matrix';
import { PaginationDto } from '../../common/pagination.dto';

@Controller('domains')
@UseGuards(JwtAuthGuard, RolesGuard)
@Permissions(Permission.GATEWAY_READ)
export class DomainsController {
  constructor(private domainsService: DomainsService) {}

  @Get()
  findAll(
    @Query() pagination: PaginationDto,
    @Query('status') status?: string,
  ) {
    return this.domainsService.findAll(status, pagination.page, pagination.limit);
  }

  @Get('stats')
  getStats() {
    return this.domainsService.getStats();
  }
}
