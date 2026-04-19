import { Injectable } from '@nestjs/common';
import { PrismaService } from '../../prisma/prisma.service';

// 域名模块：域名温储备池状态查看
// 注意：域名表由 G-Switch 协议管理，此处仅提供查询接口
// 如果 domains 表尚未在 Prisma Schema 中定义，使用原始查询
@Injectable()
export class DomainsService {
  constructor(private prisma: PrismaService) {}

  async findAll(status?: string) {
    const where = status ? `WHERE status = '${status}'` : '';
    try {
      const domains = await this.prisma.$queryRawUnsafe(
        `SELECT * FROM domains ${where} ORDER BY created_at DESC`,
      );
      return domains;
    } catch {
      // domains 表可能尚未创建
      return [];
    }
  }

  async getStats() {
    try {
      const stats = await this.prisma.$queryRaw`
        SELECT status, COUNT(*)::int as count
        FROM domains
        GROUP BY status
      `;
      return stats;
    } catch {
      return [];
    }
  }
}
