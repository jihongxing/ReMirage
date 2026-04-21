import { Module } from '@nestjs/common';
import { GatewaysController } from './gateways.controller';
import { GatewaysService } from './gateways.service';
import { BridgeClientService } from './bridge-client.service';

@Module({
  controllers: [GatewaysController],
  providers: [GatewaysService, BridgeClientService],
  exports: [GatewaysService, BridgeClientService],
})
export class GatewaysModule {}
