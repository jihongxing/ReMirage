import { Injectable, Logger } from '@nestjs/common';
import { Cron, CronExpression } from '@nestjs/schedule';
import { PrismaService } from '../../prisma/prisma.service';

export interface AddThreatIntelDto {
  sourceIp: string;
  threatType: string;
  severity?: number;
  source?: string; // manual / auto / gateway_report
  reportedByGateway?: string;
  ttlSeconds?: number;
}

@Injectable()
export class ThreatIntelService {
  private readonly logger = new Logger(ThreatIntelService.name);
  private readonly GLOBAL_BAN_THRESHOLD = 3;

  constructor(private prisma: PrismaService) {}

  /**
   * 添加/更新威胁情报记录（UPSERT）
   */
  async addThreatIntel(dto: AddThreatIntelDto) {
    const ttl = dto.ttlSeconds ?? 3600;
    const expiresAt = new Date(Date.now() + ttl * 1000);

    const record = await this.prisma.threatIntel.upsert({
      where: {
        sourceIp_threatType: {
          sourceIp: dto.sourceIp,
          threatType: dto.threatType,
        },
      },
      create: {
        sourceIp: dto.sourceIp,
        threatType: dto.threatType,
        severity: dto.severity ?? 0,
        source: dto.source ?? 'auto',
        reportedByGateway: dto.reportedByGateway,
        ttlSeconds: ttl,
        expiresAt,
        hitCount: 1,
      },
      update: {
        hitCount: { increment: 1 },
        severity: dto.severity !== undefined
          ? { set: Math.max(dto.severity, 0) }
          : undefined,
        lastSeen: new Date(),
        reportedByGateway: dto.reportedByGateway,
        expiresAt,
      },
    });

    return record;
  }

  /**
   * TTL 过期清理（每分钟执行）
   */
  @Cron(CronExpression.EVERY_MINUTE)
  async cleanExpired() {
    const result = await this.prisma.threatIntel.deleteMany({
      where: {
        expiresAt: { not: null, lte: new Date() },
        isBanned: false,
      },
    });

    if (result.count > 0) {
      this.logger.log(`Cleaned ${result.count} expired threat intel records`);
    }
  }

  /**
   * 评估全局封禁：同一 sourceIp 被 >= GLOBAL_BAN_THRESHOLD 个不同 Gateway 上报 → 全局封禁
   */
  async evaluateGlobalBan(sourceIp: string): Promise<boolean> {
    const distinctGateways = await this.prisma.threatIntel.findMany({
      where: {
        sourceIp,
        reportedByGateway: { not: null },
      },
      select: { reportedByGateway: true },
      distinct: ['reportedByGateway'],
    });

    if (distinctGateways.length >= this.GLOBAL_BAN_THRESHOLD) {
      await this.prisma.threatIntel.updateMany({
        where: { sourceIp },
        data: { isBanned: true },
      });
      this.logger.warn(
        `Global ban triggered for ${sourceIp} (reported by ${distinctGateways.length} gateways)`,
      );
      return true;
    }

    return false;
  }

  /**
   * 获取所有已封禁条目（用于黑名单同步）
   */
  async getBannedEntries() {
    return this.prisma.threatIntel.findMany({
      where: { isBanned: true },
      select: {
        sourceIp: true,
        threatType: true,
        severity: true,
        lastSeen: true,
      },
    });
  }

  /**
   * 获取封禁摘要（条目数 + 最新更新时间戳）
   */
  async getBannedSummary() {
    const [count, latest] = await Promise.all([
      this.prisma.threatIntel.count({ where: { isBanned: true } }),
      this.prisma.threatIntel.findFirst({
        where: { isBanned: true },
        orderBy: { lastSeen: 'desc' },
        select: { lastSeen: true },
      }),
    ]);

    return {
      count,
      latestUpdatedAt: latest?.lastSeen ?? null,
    };
  }

  /**
   * 手动解封
   */
  async unban(sourceIp: string) {
    return this.prisma.threatIntel.updateMany({
      where: { sourceIp },
      data: { isBanned: false },
    });
  }
}
