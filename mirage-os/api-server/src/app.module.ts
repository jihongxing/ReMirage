import { Module } from '@nestjs/common';
import { ScheduleModule } from '@nestjs/schedule';
import { PrismaModule } from './prisma/prisma.module';
import { AuthModule } from './modules/auth/auth.module';
import { UsersModule } from './modules/users/users.module';
import { CellsModule } from './modules/cells/cells.module';
import { BillingModule } from './modules/billing/billing.module';
import { DomainsModule } from './modules/domains/domains.module';
import { ThreatsModule } from './modules/threats/threats.module';
import { GatewaysModule } from './modules/gateways/gateways.module';

@Module({
  imports: [
    ScheduleModule.forRoot(),
    PrismaModule,
    AuthModule,
    UsersModule,
    CellsModule,
    BillingModule,
    DomainsModule,
    ThreatsModule,
    GatewaysModule,
  ],
})
export class AppModule {}
