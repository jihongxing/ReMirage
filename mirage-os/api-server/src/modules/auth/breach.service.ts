import { Injectable, UnauthorizedException } from '@nestjs/common';
import { JwtService } from '@nestjs/jwt';
import { PrismaService } from '../../prisma/prisma.service';
import * as crypto from 'crypto';

/**
 * Ed25519 挑战-响应认证服务（方案 B：NestJS 原生实现）
 *
 * 流程：
 * 1. 前端调用 GET /auth/challenge 获取挑战字符串
 * 2. 前端用 Ed25519 私钥签名挑战
 * 3. 前端调用 POST /auth/breach 提交签名
 * 4. 后端验签，签发 JWT
 */
@Injectable()
export class BreachService {
  // 内存中的挑战缓存（生产环境应用 Redis）
  private challenges = new Map<string, { raw: string; createdAt: number }>();
  private readonly CHALLENGE_TTL_MS = 300_000; // 5 分钟

  constructor(
    private prisma: PrismaService,
    private jwtService: JwtService,
  ) {
    // 定期清理过期挑战
    setInterval(() => this.cleanExpired(), 60_000);
  }

  /** 生成挑战字符串 */
  generateChallenge(): { nonce: string; raw: string; timestamp: number } {
    const nonce = crypto.randomBytes(32).toString('hex');
    const timestamp = Date.now();
    const raw = `mirage-auth:${nonce}:${timestamp}`;

    this.challenges.set(nonce, { raw, createdAt: timestamp });

    return { nonce, raw, timestamp };
  }

  /** 验证 Ed25519 签名并签发 JWT */
  async verifyAndSign(dto: {
    signature: string;
    challenge: string;
    publicKey?: string;
  }): Promise<{ accessToken: string; role: string; userId: string; expiresAt: number }> {
    const { signature, challenge } = dto;

    // 1. 解析挑战中的 nonce
    const parts = challenge.match(/^mirage-auth:([a-f0-9]{64}):(\d+)$/);
    if (!parts) {
      throw new UnauthorizedException('认证失败');
    }
    const [, nonce, tsStr] = parts;
    const ts = parseInt(tsStr, 10);

    // 2. 检查挑战是否存在且未过期
    const cached = this.challenges.get(nonce);
    if (!cached || cached.raw !== challenge) {
      throw new UnauthorizedException('认证失败');
    }
    if (Date.now() - ts > this.CHALLENGE_TTL_MS) {
      this.challenges.delete(nonce);
      throw new UnauthorizedException('挑战已过期');
    }

    // 3. 消费 nonce（防重放）
    this.challenges.delete(nonce);

    // 4. 查找拥有 Ed25519 公钥的 operator/admin 用户
    const users = await this.prisma.user.findMany({
      where: {
        ed25519Pubkey: { not: null },
        isActive: true,
        isOperator: true,
      },
    });

    // 5. 逐一验签
    const sigBuffer = Buffer.from(signature, 'hex');
    const msgBuffer = Buffer.from(challenge);

    for (const user of users) {
      if (!user.ed25519Pubkey) continue;

      try {
        const pubKeyBuffer = Buffer.from(user.ed25519Pubkey, 'hex');
        const keyObject = crypto.createPublicKey({
          key: Buffer.concat([
            // Ed25519 DER prefix
            Buffer.from('302a300506032b6570032100', 'hex'),
            pubKeyBuffer,
          ]),
          format: 'der',
          type: 'spki',
        });

        const valid = crypto.verify(null, msgBuffer, keyObject, sigBuffer);
        if (valid) {
          // 签发 JWT
          const expiresAt = Date.now() + 86400_000; // 24h
          const payload = { sub: user.id, cell_id: user.cellId, role: 'admin' };
          const accessToken = this.jwtService.sign(payload, { expiresIn: '24h' });

          return {
            accessToken,
            role: 'admin',
            userId: user.id,
            expiresAt,
          };
        }
      } catch {
        // 验签失败，继续尝试下一个用户
        continue;
      }
    }

    throw new UnauthorizedException('认证失败');
  }

  /** 验证已有 JWT token */
  async validateToken(token: string): Promise<{
    success: boolean;
    token?: string;
    role?: string;
    userId?: string;
    expiresAt?: number;
  }> {
    try {
      const payload = this.jwtService.verify(token);
      const user = await this.prisma.user.findUnique({
        where: { id: payload.sub },
      });

      if (!user || !user.isActive) {
        return { success: false };
      }

      // 续签 token — 保持原 role，不升级
      const originalRole = payload.role || 'user';
      const expiresAt = Date.now() + 86400_000;
      const newToken = this.jwtService.sign(
        { sub: user.id, cell_id: user.cellId, role: originalRole },
        { expiresIn: '24h' },
      );

      return {
        success: true,
        token: newToken,
        role: originalRole,
        userId: user.id,
        expiresAt,
      };
    } catch {
      return { success: false };
    }
  }

  private cleanExpired() {
    const now = Date.now();
    for (const [nonce, entry] of this.challenges) {
      if (now - entry.createdAt > this.CHALLENGE_TTL_MS) {
        this.challenges.delete(nonce);
      }
    }
  }
}
