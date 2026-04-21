import { Controller, Get, Query, UseGuards } from '@nestjs/common';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { RolesGuard } from '../auth/roles.guard';
import { Permissions } from '../auth/permissions.decorator';
import { Permission } from '../auth/rbac-matrix';
import { SessionService } from './session.service';

@Controller('sessions')
@UseGuards(JwtAuthGuard, RolesGuard)
export class SessionController {
  constructor(private sessionService: SessionService) {}

  @Get('by-gateway')
  @Permissions(Permission.GATEWAY_READ)
  async getByGateway(@Query('gateway_id') gatewayId: string) {
    return this.sessionService.getSessionsByGateway(gatewayId);
  }

  @Get('by-user')
  @Permissions(Permission.GATEWAY_READ)
  async getByUser(@Query('user_id') userId: string) {
    return this.sessionService.getSessionsByUser(userId);
  }

  @Get('by-client')
  @Permissions(Permission.GATEWAY_READ)
  async getByClient(@Query('client_id') clientId: string) {
    return this.sessionService.getSessionByClient(clientId);
  }
}
