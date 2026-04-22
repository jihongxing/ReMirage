import {
  Controller,
  Get,
  Param,
  Query,
  NotFoundException,
  GoneException,
  Header,
  UseGuards,
} from '@nestjs/common';
import { InternalHMACGuard, signInternalRequest } from '../../common/internal-hmac.guard';

/**
 * 阅后即焚配置交付端点
 * 挂载 InternalHMACGuard 鉴权
 */
@Controller('delivery')
@UseGuards(InternalHMACGuard)
export class DeliveryController {
  @Get(':token')
  @Header('Cache-Control', 'no-store, no-cache, must-revalidate, private')
  @Header('Pragma', 'no-cache')
  @Header('X-Robots-Tag', 'noindex, nofollow, noarchive')
  async redeemConfig(
    @Param('token') token: string,
    @Query('key') key: string,
  ) {
    if (!token || !key) {
      throw new NotFoundException();
    }

    try {
      const provisionerUrl = process.env.PROVISIONER_URL || 'http://localhost:18443';
      const bodyStr = JSON.stringify({ token, decrypt_key: key });
      const hmacSecret = process.env.INTERNAL_HMAC_SECRET || '';
      const hmacHeaders = hmacSecret ? signInternalRequest(bodyStr, hmacSecret) : {};

      const response = await fetch(
        `${provisionerUrl}/internal/delivery/redeem`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', ...hmacHeaders },
          redirect: 'error',
          body: bodyStr,
        },
      );

      if (response.status === 404) {
        throw new NotFoundException('链接不存在或已销毁');
      }
      if (response.status === 410) {
        throw new GoneException('链接已被使用');
      }
      if (!response.ok) {
        throw new NotFoundException();
      }

      return await response.json();
    } catch (err) {
      if (err instanceof NotFoundException || err instanceof GoneException) {
        throw err;
      }
      throw new NotFoundException();
    }
  }
}
