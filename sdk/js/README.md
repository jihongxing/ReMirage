# Mirage JavaScript/TypeScript SDK

## 安装

```bash
npm install @mirage/sdk
# 或
yarn add @mirage/sdk
```

## 快速开始

```typescript
import { MirageClient } from '@mirage/sdk';

// 初始化
const client = new MirageClient({
  endpoint: 'grpc.mirage.example:50847',
  token: 'your_jwt_token',
});

// 心跳同步
const resp = await client.gateway.syncHeartbeat({
  gatewayId: 'gw-001',
  version: '1.0.0',
  threatLevel: 0,
});
console.log(`剩余配额: ${resp.remainingQuota} bytes`);

// 查询余额
const balance = await client.billing.getBalance('acc-001');
console.log(`余额: $${(balance.balanceUsd / 100).toFixed(2)}`);

// 查询蜂窝
const cells = await client.cell.listCells({ onlineOnly: true });
cells.cells.forEach(cell => {
  console.log(`${cell.cellName}: ${cell.loadPercent}%`);
});
```

## WebSocket 实时推送

```typescript
import { MirageWebSocket } from '@mirage/sdk';

const ws = new MirageWebSocket('wss://ws.mirage.example:18443', {
  token: 'your_token',
});

ws.on('threat', (data) => {
  console.log('威胁告警:', data);
});

ws.on('quota_warning', (data) => {
  console.log('配额告警:', data);
});

await ws.connect();
```

## API 参考

### GatewayService

| 方法 | 说明 |
|------|------|
| `syncHeartbeat(req)` | 心跳同步 |
| `reportTraffic(req)` | 流量上报 |
| `reportThreat(req)` | 威胁上报 |
| `getQuota(gatewayId, userId)` | 配额查询 |

### BillingService

| 方法 | 说明 |
|------|------|
| `createAccount(userId, publicKey)` | 创建账户 |
| `deposit(req)` | 充值 |
| `getBalance(accountId)` | 查询余额 |
| `purchaseQuota(req)` | 购买流量包 |

### CellService

| 方法 | 说明 |
|------|------|
| `listCells(req)` | 查询蜂窝 |
| `allocateGateway(req)` | 分配 Gateway |
| `switchCell(req)` | 切换蜂窝 |
