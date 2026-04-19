import {
  Controller,
  Get,
  Param,
  Query,
  NotFoundException,
  GoneException,
  Header,
} from '@nestjs/common';

/**
 * 阅后即焚配置交付端点
 * 
 * 用户通过一次性加密链接获取客户端配置
 * 链接被访问一次后立即销毁，不留痕迹
 * 
 * 注意：实际的加密/解密和链接管理由 Go 端 Provisioner 处理
 * 此 Controller 仅作为 HTTP 代理层，转发到 Go 端 API
 */
@Controller('delivery')
export class DeliveryController {
  /**
   * 兑换配置链接
   * GET /delivery/:token?key=<base64url_aes_key>
   * 
   * 成功返回解密后的 JSON 配置
   * 链接立即销毁
   */
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

    // 转发到 Go 端 Provisioner API
    // 实际部署中通过内部 gRPC 或 HTTP 调用
    try {
      const response = await fetch(
        `${process.env.PROVISIONER_URL || 'http://localhost:18443'}/internal/delivery/redeem`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ token, decrypt_key: key }),
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

      const config = await response.json();
      return config;
    } catch (err) {
      if (err instanceof NotFoundException || err instanceof GoneException) {
        throw err;
      }
      throw new NotFoundException();
    }
  }
}
