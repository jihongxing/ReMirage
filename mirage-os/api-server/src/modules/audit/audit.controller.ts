import { Controller, Get, Query, UseGuards } from '@nestjs/common';
import { AuditService } from './audit.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { RolesGuard } from '../auth/roles.guard';
import { Permissions } from '../auth/permissions.decorator';
import { Permission } from '../auth/rbac-matrix';
import { PaginationDto } from '../../common/pagination.dto';

@Controller('audit')
@UseGuards(JwtAuthGuard, RolesGuard)
@Permissions(Permission.AUDIT_READ)
export class AuditController {
  constructor(private readonly auditService: AuditService) {}

  @Get()
  async findAll(
    @Query() pagination: PaginationDto,
    @Query('startDate') startDate?: string,
    @Query('endDate') endDate?: string,
    @Query('operatorId') operatorId?: string,
    @Query('actionType') actionType?: string,
  ) {
    return this.auditService.query({
      startDate,
      endDate,
      operatorId,
      actionType,
      page: pagination.page,
      limit: pagination.limit,
    });
  }
}
