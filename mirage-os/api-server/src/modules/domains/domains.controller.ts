import { Controller, Get, Query, UseGuards } from '@nestjs/common';
import { DomainsService } from './domains.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';

@Controller('domains')
@UseGuards(JwtAuthGuard)
export class DomainsController {
  constructor(private domainsService: DomainsService) {}

  @Get()
  findAll(@Query('status') status?: string) {
    return this.domainsService.findAll(status);
  }

  @Get('stats')
  getStats() {
    return this.domainsService.getStats();
  }
}
