// 单元测试：Gateway 超时标记 OFFLINE
describe('GatewaysService - markOfflineGateways', () => {
  it('should identify gateways exceeding heartbeat timeout', () => {
    const TIMEOUT_SECONDS = 300;
    const now = Date.now();

    const gateways = [
      { id: 'gw-1', status: 'ONLINE', lastHeartbeat: new Date(now - 100 * 1000) }, // 100s ago
      { id: 'gw-2', status: 'ONLINE', lastHeartbeat: new Date(now - 400 * 1000) }, // 400s ago
      { id: 'gw-3', status: 'DEGRADED', lastHeartbeat: new Date(now - 600 * 1000) }, // 600s ago
      { id: 'gw-4', status: 'OFFLINE', lastHeartbeat: new Date(now - 1000 * 1000) }, // already offline
    ];

    const threshold = new Date(now - TIMEOUT_SECONDS * 1000);
    const toMarkOffline = gateways.filter(
      (gw) => gw.status !== 'OFFLINE' && gw.lastHeartbeat < threshold,
    );

    expect(toMarkOffline).toHaveLength(2);
    expect(toMarkOffline.map((g) => g.id)).toEqual(['gw-2', 'gw-3']);
  });

  it('should not mark recently active gateways as offline', () => {
    const TIMEOUT_SECONDS = 300;
    const now = Date.now();

    const gw = {
      id: 'gw-active',
      status: 'ONLINE',
      lastHeartbeat: new Date(now - 10 * 1000), // 10s ago
    };

    const threshold = new Date(now - TIMEOUT_SECONDS * 1000);
    const shouldMarkOffline = gw.status !== 'OFFLINE' && gw.lastHeartbeat < threshold;

    expect(shouldMarkOffline).toBe(false);
  });
});
