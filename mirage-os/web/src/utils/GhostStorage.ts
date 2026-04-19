// GhostStorage - 取证对抗存储管理
// 所有敏感数据仅存内存，零留存策略

// 禁止写入的 localStorage 键
const FORBIDDEN_KEYS = [
  'user_balance',
  'user_quota',
  'node_config',
  'transaction_history',
  'protocol_settings',
  'threat_events',
];

// 允许写入的键（仅鉴权相关）
const ALLOWED_KEYS = [
  'mirage_vault',
  'mirage_token',
];

// 拦截 localStorage 写入
const originalSetItem = localStorage.setItem.bind(localStorage);
localStorage.setItem = (key: string, value: string) => {
  if (FORBIDDEN_KEYS.some(k => key.includes(k))) {
    console.warn(`[GhostStorage] Blocked write to localStorage: ${key}`);
    return;
  }
  originalSetItem(key, value);
};

// 内存态存储
class GhostStore {
  private store: Map<string, unknown> = new Map();
  private listeners: Map<string, Set<(value: unknown) => void>> = new Map();

  set<T>(key: string, value: T): void {
    this.store.set(key, value);
    this.notify(key, value);
  }

  get<T>(key: string): T | undefined {
    return this.store.get(key) as T | undefined;
  }

  delete(key: string): void {
    this.store.delete(key);
    this.notify(key, undefined);
  }

  subscribe(key: string, callback: (value: unknown) => void): () => void {
    if (!this.listeners.has(key)) {
      this.listeners.set(key, new Set());
    }
    this.listeners.get(key)!.add(callback);
    return () => this.listeners.get(key)?.delete(callback);
  }

  private notify(key: string, value: unknown): void {
    this.listeners.get(key)?.forEach(cb => cb(value));
  }

  // 自毁：清空所有内存数据
  wipe(): void {
    this.store.clear();
    this.listeners.clear();
  }
}

export const ghostStore = new GhostStore();

// 时间模糊化工具
export function fuzzyTime(timestamp: number): string {
  const now = Date.now();
  const diff = now - timestamp;
  const hours = Math.floor(diff / 3600000);
  const days = Math.floor(diff / 86400000);

  if (hours < 1) return '刚刚';
  if (hours < 3) return '不久前';
  if (hours < 6) return '今日早些时候';
  if (hours < 12) return '今日';
  if (hours < 24) return '约一天前';
  if (days < 3) return '近几日';
  if (days < 7) return '本周';
  if (days < 30) return '本月';
  return '较早';
}

// 哈希遮掩工具
export function maskHash(hash: string, showFull = false): string {
  if (showFull || !hash || hash.length < 12) return hash;
  return `${hash.slice(0, 6)}...${hash.slice(-4)}`;
}

// UID 遮掩
export function maskUID(uid: string, showFull = false): string {
  if (showFull || !uid || uid.length < 8) return uid;
  return `Mirage-${uid.slice(0, 4)}...${uid.slice(-4)}`;
}

// 自毁处理
export function handleSelfDestruct(): void {
  // 清空内存存储
  ghostStore.wipe();
  
  // 清空允许的 localStorage
  ALLOWED_KEYS.forEach(key => localStorage.removeItem(key));
  
  // 清空 sessionStorage
  sessionStorage.clear();
  
  // 跳转空白页
  window.location.href = 'about:blank';
}

// Ghost Mode 下禁止复制
export function setupGhostModeCopyProtection(ghostMode: boolean): void {
  if (ghostMode) {
    document.addEventListener('copy', (e) => {
      e.preventDefault();
      e.clipboardData?.setData('text/plain', '[REDACTED]');
    });
  }
}

export default ghostStore;
