---
Status: authoritative
Target Truth: 本文件为 entitlement 接口契约主源
Migration: 无需迁移，持续按接口变更更新
---

# Entitlement API 契约

## GET /api/v2/entitlement

### 鉴权
- `Authorization: Bearer <base64(authKey)>`
- `X-Client-ID: <userID>`

### 响应结构 (Entitlement)
```json
{
  "service_class": "standard",
  "expires_at": "2024-12-31T23:59:59Z",
  "quota_remaining": 10737418240,
  "banned": false,
  "fetched_at": "2024-01-01T00:00:00Z"
}
```

### 缓存策略
- Client 缓存到 `PersistConfig.LastEntitlement`
- 离线宽限窗口：24 小时
- 刷新间隔：由 ServiceClass 决定（Standard=5min, Platinum=2min, Diamond=1min）

### 错误码
- 401: 鉴权失败
- 403: 账户被封禁
- 500: 服务端错误
