import { Injectable, Logger, OnModuleInit } from '@nestjs/common';

/**
 * BridgeClientService — API Server 到 gateway-bridge 的统一 HTTP 客户端。
 * 所有请求自动携带 X-Internal-Secret Header。
 */
@Injectable()
export class BridgeClientService implements OnModuleInit {
  private readonly logger = new Logger(BridgeClientService.name);
  private baseUrl: string;
  private internalSecret: string;

  onModuleInit() {
    this.baseUrl = process.env.BRIDGE_URL || 'http://127.0.0.1:7000';
    this.internalSecret = process.env.BRIDGE_INTERNAL_SECRET || '';
    if (!this.internalSecret) {
      throw new Error(
        'BRIDGE_INTERNAL_SECRET 环境变量为空，拒绝启动。生产环境必须配置内部鉴权密钥。',
      );
    }
  }

  private buildHeaders(extra?: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...extra,
    };
    if (this.internalSecret) {
      headers['X-Internal-Secret'] = this.internalSecret;
    }
    return headers;
  }

  async get<T = any>(path: string): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const res = await fetch(url, {
      method: 'GET',
      headers: this.buildHeaders(),
      redirect: 'error',
    });
    if (!res.ok) {
      const body = await res.text();
      throw new Error(`bridge GET ${path} failed: ${res.status} ${body}`);
    }
    return res.json() as Promise<T>;
  }

  async post<T = any>(path: string, body?: unknown): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const res = await fetch(url, {
      method: 'POST',
      headers: this.buildHeaders(),
      redirect: 'error',
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`bridge POST ${path} failed: ${res.status} ${text}`);
    }
    return res.json() as Promise<T>;
  }
}
