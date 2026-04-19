import { Controller, Post, Get, Body, Headers } from '@nestjs/common';
import { AuthService, RegisterDto, LoginDto } from './auth.service';
import { BreachService } from './breach.service';

@Controller('auth')
export class AuthController {
  constructor(
    private authService: AuthService,
    private breachService: BreachService,
  ) {}

  /** 用户注册（邀请码 + 密码 + TOTP） */
  @Post('register')
  register(@Body() dto: RegisterDto) {
    return this.authService.register(dto);
  }

  /** 用户登录（密码 + TOTP → JWT） */
  @Post('login')
  login(@Body() dto: LoginDto) {
    return this.authService.login(dto);
  }

  /** 获取 Ed25519 挑战字符串 */
  @Get('challenge')
  getChallenge() {
    return this.breachService.generateChallenge();
  }

  /** Ed25519 签名认证（管理员入口） */
  @Post('breach')
  async breach(
    @Body() dto: { signature: string; challenge: string; timestamp?: number },
  ) {
    const result = await this.breachService.verifyAndSign(dto);
    return {
      success: true,
      token: result.accessToken,
      role: result.role,
      userId: result.userId,
      expiresAt: result.expiresAt,
    };
  }

  /** 验证已有 JWT token（续签） */
  @Post('validate')
  async validate(
    @Body() dto: { token: string },
    @Headers('x-mirage-token') headerToken?: string,
  ) {
    const token = dto.token || headerToken;
    if (!token) {
      return { success: false };
    }
    return this.breachService.validateToken(token);
  }
}
