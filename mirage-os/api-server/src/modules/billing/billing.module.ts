import { Module } from '@nestjs/common';
import { BillingController } from './billing.controller';
import { BillingService } from './billing.service';
import { XMRWebhookController } from './xmr-webhook.controller';
import { DeliveryController } from './delivery.controller';

@Module({
  controllers: [BillingController, XMRWebhookController, DeliveryController],
  providers: [BillingService],
  exports: [BillingService],
})
export class BillingModule {}
