/**
 * 内部 URL 白名单校验
 * 防止 SSRF 攻击利用内部服务做跳板
 */

const DEFAULT_WHITELIST = new Set([
  '127.0.0.1',
  'localhost',
  '::1',
]);

let configuredHosts: Set<string> = new Set();

/**
 * 初始化白名单（启动时调用）
 */
export function initInternalURLWhitelist(extraHosts?: string[]): void {
  configuredHosts = new Set(extraHosts || []);
}

/**
 * 校验 URL host 是否在白名单内
 */
export function validateInternalURL(url: string): boolean {
  try {
    const parsed = new URL(url);
    const host = parsed.hostname;
    return DEFAULT_WHITELIST.has(host) || configuredHosts.has(host);
  } catch {
    return false;
  }
}

/**
 * 校验所有 *_URL 环境变量
 * 不在白名单内直接抛异常
 */
export function validateURLEnvironmentVariables(): void {
  const envVars = Object.entries(process.env);
  for (const [key, value] of envVars) {
    if (key.endsWith('_URL') && value) {
      if (!validateInternalURL(value)) {
        throw new Error(
          `环境变量 ${key}=${value} 的 host 不在内部白名单内，拒绝启动`,
        );
      }
    }
  }
}
