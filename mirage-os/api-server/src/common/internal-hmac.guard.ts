import {
  Injectable,
  CanActivate,
  ExecutionContext,
  UnauthorizedException,
  ForbiddenException,
} from '@nestjs/common';
import * as crypto from 'crypto';

/** 敏感键集合 */
const TIMESTAMP_TOLERANCE_SECONDS = 300; // ±5 分钟

/**
 * InternalHMACGuard - 内部接口 HMAC-SHA256 签名校验中间件
 *
 * 请求头:
 * - X-Internal-Timestamp: Unix 秒
 * - X-Internal-Nonce: UUID v4
 * - X-Internal-Signature: HMAC-SHA256(secret, timestamp + nonce + SHA256(body))
 */
@Injectable()
export class InternalHMACGuard implements CanActivate {
  private readonly secret: string;
  private readonly nonceStore = new Map<string, number>();
  private readonly nonceTTL = TIMESTAMP_TOLERANCE_SECONDS * 1000;

  constructor() {
    const secret = process.env.INTERNAL_HMAC_SECRET;
    if (!secret) {
      throw new Error('INTERNAL_HMAC_SECRET 环境变量未设置');
    }
    this.secret = secret;

    // 定期清理过期 nonce
    setInterval(() => this.cleanExpiredNonces(), 60_000);
  }

  canActivate(context: ExecutionContext): boolean {
    const request = context.switchToHttp().getRequest();

    const timestamp = request.headers['x-internal-timestamp'];
    const nonce = request.headers['x-internal-nonce'];
    const signature = request.headers['x-internal-signature'];

    if (!timestamp || !nonce || !signature) {
      throw new UnauthorizedException('缺少 HMAC 签名头');
    }

    // 1. 校验时间窗口
    const ts = parseInt(timestamp, 10);
    if (isNaN(ts)) {
      throw new ForbiddenException('无效的时间戳格式');
    }
    const now = Math.floor(Date.now() / 1000);
    if (Math.abs(now - ts) > TIMESTAMP_TOLERANCE_SECONDS) {
      throw new ForbiddenException('时间戳超出允许窗口');
    }

    // 2. 校验 nonce 未被使用
    if (this.nonceStore.has(nonce)) {
      throw new ForbiddenException('Nonce 已被使用（重放攻击）');
    }
    this.nonceStore.set(nonce, Date.now());

    // 3. 计算 body hash
    const body = request.body ? JSON.stringify(request.body) : '';
    const bodyHash = crypto.createHash('sha256').update(body).digest('hex');

    // 4. 计算 HMAC-SHA256
    const mac = crypto.createHmac('sha256', this.secret);
    mac.update(timestamp + nonce + bodyHash);
    const expected = mac.digest('hex');

    // 5. 常量时间比较
    if (!crypto.timingSafeEqual(Buffer.from(signature, 'hex'), Buffer.from(expected, 'hex'))) {
      throw new ForbiddenException('HMAC 签名校验失败');
    }

    return true;
  }

  private cleanExpiredNonces(): void {
    const now = Date.now();
    for (const [key, ts] of this.nonceStore) {
      if (now - ts > this.nonceTTL) {
        this.nonceStore.delete(key);
      }
    }
  }
}

/**
 * signInternalRequest - 为内部 HTTP 调用生成 HMAC 签名头
 */
export function signInternalRequest(
  body: string,
  secret: string,
): { 'X-Internal-Timestamp': string; 'X-Internal-Nonce': string; 'X-Internal-Signature': string } {
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const nonce = crypto.randomUUID();
  const bodyHash = crypto.createHash('sha256').update(body).digest('hex');
  const mac = crypto.createHmac('sha256', secret);
  mac.update(timestamp + nonce + bodyHash);
  const signature = mac.digest('hex');

  return {
    'X-Internal-Timestamp': timestamp,
    'X-Internal-Nonce': nonce,
    'X-Internal-Signature': signature,
  };
}
