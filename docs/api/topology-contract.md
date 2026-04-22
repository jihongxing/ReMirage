---
Status: authoritative
Target Truth: 本文件为 topology 接口契约主源
Migration: 无需迁移，持续按接口变更更新
---

# Topology API 契约

## GET /api/v2/topology

### 鉴权
- `Authorization: Bearer <base64(authKey)>`
- `X-Client-ID: <userID>`

### 响应结构 (RouteTableResponse)
```json
{
  "version": 42,
  "published_at": "2024-01-01T00:00:00Z",
  "gateways": [
    {
      "gateway_id": "gw-abc123",
      "ip_address": "1.2.3.4",
      "cell_id": "cell-us-east",
      "status": "ONLINE"
    }
  ],
  "signature": "hex-encoded-hmac-sha256"
}
```

### 签名
HMAC-SHA256(PSK, version + published_at + gateways JSON)

### 缓存
- 支持 `If-None-Match` / `304 Not Modified`
- `version` 单调递增
- `published_at` 单调递增

### 错误码
- 401: 鉴权失败
- 304: 未修改
- 500: 服务端错误
