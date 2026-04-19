import { Injectable, BadRequestException, UnauthorizedException } from '@nestjs/common';
import { JwtService } from '@nestjs/jwt';
import { PrismaService } from '../../prisma/prisma.service';
import * as bcrypt from 'bcrypt';
import * as speakeasy from 'speakeasy';

export interface RegisterDto {
  username: string;
  password: string;
  inviteCode: string;
}

export interface LoginDto {
  username: string;
  password: string;
  totpCode: string;
}

@Injectable()
export class AuthService {
  constructor(
    private prisma: PrismaService,
    private jwtService: JwtService,
  ) {}

  async register(dto: RegisterDto) {
    // 验证邀请码
    const invite = await this.prisma.inviteCode.findUnique({
      where: { code: dto.inviteCode },
    });
    if (!invite || invite.isUsed) {
      throw new BadRequestException('邀请码无效');
    }

    const passwordHash = await bcrypt.hash(dto.password, 12);
    const secret = speakeasy.generateSecret({
      name: `MirageOS:${dto.username}`,
      issuer: 'MirageOS',
    });

    // 事务：创建用户 + 标记邀请码
    const user = await this.prisma.$transaction(async (tx) => {
      const newUser = await tx.user.create({
        data: {
          username: dto.username,
          passwordHash,
          totpSecret: secret.base32,
          inviteCodeUsed: dto.inviteCode,
        },
      });

      await tx.inviteCode.update({
        where: { id: invite.id },
        data: {
          isUsed: true,
          usedBy: newUser.id,
          usedAt: new Date(),
        },
      });

      return newUser;
    });

    return {
      user: { id: user.id, username: user.username },
      totpUri: secret.otpauth_url,
    };
  }

  async login(dto: LoginDto) {
    const user = await this.prisma.user.findUnique({
      where: { username: dto.username },
    });
    if (!user || !user.isActive) {
      throw new UnauthorizedException('认证失败');
    }

    const passwordValid = await bcrypt.compare(dto.password, user.passwordHash);
    if (!passwordValid) {
      throw new UnauthorizedException('认证失败');
    }

    if (user.totpSecret) {
      const totpValid = speakeasy.totp.verify({
        secret: user.totpSecret,
        encoding: 'base32',
        token: dto.totpCode,
        window: 1,
      });
      if (!totpValid) {
        throw new UnauthorizedException('认证失败');
      }
    }

    const payload = { sub: user.id, cell_id: user.cellId };
    return {
      accessToken: this.jwtService.sign(payload, { expiresIn: '24h' }),
    };
  }

  async validateUser(userId: string) {
    return this.prisma.user.findUnique({ where: { id: userId } });
  }
}
