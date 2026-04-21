import { Injectable } from '@nestjs/common';
import { PassportStrategy } from '@nestjs/passport';
import { ExtractJwt, Strategy } from 'passport-jwt';

@Injectable()
export class JwtStrategy extends PassportStrategy(Strategy) {
  constructor() {
    const secret = process.env.JWT_SECRET || 'dev_jwt_secret_change_in_production';
    if (process.env.NODE_ENV === 'production') {
      if (secret === 'dev_jwt_secret_change_in_production' || secret.length < 32) {
        throw new Error('生产模式必须设置 JWT_SECRET（>= 32 字符）');
      }
    }
    super({
      jwtFromRequest: ExtractJwt.fromAuthHeaderAsBearerToken(),
      ignoreExpiration: false,
      secretOrKey: secret,
    });
  }

  async validate(payload: { sub: string; cell_id: string; role?: string }) {
    return { userId: payload.sub, cellId: payload.cell_id, role: payload.role || 'user' };
  }
}
