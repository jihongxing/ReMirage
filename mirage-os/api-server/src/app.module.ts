import { Module } from '@nestjs/common';
import { ScheduleModule } from '@nestjs/schedule';
import { ThrottlerModule, ThrottlerGuard } from '@nestjs/throttler';
import { APP_GUARD, APP_INTERCEPTOR } from '@nestjs/core';
import { PrismaModule } from './prisma/prisma.module';
import { AuthModule } from './modules/auth/auth.module';
import { UsersModule } from './modules/users/users.module';
import { CellsModule } from './modules/cells/cells.module';
import { BillingModule } from './modules/billing/billing.module';
import { DomainsModule } from './modules/domains/domains.module';
import { ThreatsModule } from './modules/threats/threats.module';
import { GatewaysModule } from './modules/gateways/gateways.module';
import { AuditModule } from './modules/audit/audit.module';
import { AuditInterceptor } from './modules/audit/audit-interceptor';
import { SessionModule } from './modules/sessions/session.module';

@Module({
  imports: [
    ScheduleModule.forRoot(),
    ThrottlerModule.forRoot([{ ttl: 60000, limit: 60 }]),
    PrismaModule,
    AuthModule,
    UsersModule,
    CellsModule,
    BillingModule,
    DomainsModule,
    ThreatsModule,
    GatewaysModule,
    AuditModule,
    SessionModule,
  ],
  providers: [
    { provide: APP_GUARD, useClass: ThrottlerGuard },
    { provide: APP_INTERCEPTOR, useClass: AuditInterceptor },
  ],
})
export class AppModule {}
