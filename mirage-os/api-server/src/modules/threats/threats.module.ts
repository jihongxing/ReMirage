import { Module } from '@nestjs/common';
import { ThreatsController } from './threats.controller';
import { ThreatsService } from './threats.service';
import { ThreatIntelService } from './threat-intel.service';

@Module({
  controllers: [ThreatsController],
  providers: [ThreatsService, ThreatIntelService],
  exports: [ThreatsService, ThreatIntelService],
})
export class ThreatsModule {}
