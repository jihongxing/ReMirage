import { Injectable } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';

@Injectable()
export class SessionService {
  constructor(private prisma: PrismaService) {}

  async onSessionConnected(data: {
    sessionId: string;
    gatewayId: string;
    userId: string;
    clientId: string;
  }) {
    await this.prisma.gatewaySession.upsert({
      where: { sessionId: data.sessionId },
      create: {
        sessionId: data.sessionId,
        gatewayId: data.gatewayId,
        userId: data.userId,
        clientId: data.clientId,
        status: 'active',
      },
      update: {
        gatewayId: data.gatewayId,
        status: 'active',
        disconnectedAt: null,
      },
    });
    await this.prisma.clientSession.upsert({
      where: { sessionId: data.sessionId },
      create: {
        sessionId: data.sessionId,
        clientId: data.clientId,
        userId: data.userId,
        currentGatewayId: data.gatewayId,
        status: 'active',
      },
      update: {
        currentGatewayId: data.gatewayId,
        status: 'active',
      },
    });
  }

  async onSessionDisconnected(sessionId: string) {
    await this.prisma.gatewaySession.update({
      where: { sessionId },
      data: { status: 'disconnected', disconnectedAt: new Date() },
    });
    await this.prisma.clientSession.update({
      where: { sessionId },
      data: { status: 'disconnected' },
    });
  }

  async onGatewayTimeout(gatewayId: string) {
    await this.prisma.gatewaySession.updateMany({
      where: { gatewayId, status: 'active' },
      data: { status: 'disconnected', disconnectedAt: new Date() },
    });
  }

  async getSessionsByGateway(gatewayId: string) {
    return this.prisma.gatewaySession.findMany({
      where: { gatewayId, status: 'active' },
    });
  }

  async getSessionsByUser(userId: string) {
    return this.prisma.clientSession.findMany({
      where: { userId, status: 'active' },
    });
  }

  async getSessionByClient(clientId: string) {
    return this.prisma.clientSession.findFirst({
      where: { clientId, status: 'active' },
    });
  }
}
