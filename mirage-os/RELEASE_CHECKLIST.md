# Release Checklist — Mirage-OS 上线前必过 10 项

> 每次部署/发版前逐项确认。全部 ✓ 才可放行。

## 基础设施

- [ ] **1. PostgreSQL 连通 + Prisma Migrate**
  ```bash
  docker compose exec api-server npx prisma migrate deploy
  # 期望：无 pending migration
  ```

- [ ] **2. Redis 连通**
  ```bash
  docker compose exec redis redis-cli ping
  # 期望：PONG
  ```

## 服务健康

- [ ] **3. api-server /health → 200**
  ```bash
  curl -f http://localhost:3000/health
  ```

- [ ] **4. gateway-bridge /internal/health → 200**
  ```bash
  curl -f http://localhost:7000/internal/health
  ```

- [ ] **5. web 首页加载 → 200 + 包含 id="root"**
  ```bash
  curl -s http://localhost:8080/ | grep 'id="root"'
  ```

## 认证链路

- [ ] **6. JWT 登录完整链路**
  ```bash
  # 获取 challenge
  curl http://localhost:3000/api/auth/challenge
  # 登录获取 token
  curl -X POST http://localhost:3000/api/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"...","password":"...","totp":"..."}'
  # 期望：返回 token
  ```

## 核心业务接口

- [ ] **7. GET /api/gateways → 200 + JSON 数组**
  ```bash
  curl -H "Authorization: Bearer $TOKEN" http://localhost:3000/api/gateways
  ```

- [ ] **8. GET /api/threats/stats → 200 + {banned_count, active_users}**
  ```bash
  curl -H "Authorization: Bearer $TOKEN" http://localhost:3000/api/threats/stats
  ```

- [ ] **9. GET /api/billing/quota → 200 + {remaining_quota, ...}**
  ```bash
  curl -H "Authorization: Bearer $TOKEN" http://localhost:3000/api/billing/quota
  ```

## 恢复能力

- [ ] **10. 服务重启恢复**
  ```bash
  docker compose restart api-server gateway-bridge
  sleep 5
  curl -f http://localhost:3000/health
  curl -f http://localhost:7000/internal/health
  # 期望：两个都 200，无数据丢失
  ```

---

## 自动化验证

上述 10 项可通过 smoke test 脚本一键验证：

```bash
# 启动全部服务
docker compose up -d

# 等待就绪（约 10s）
sleep 10

# 跑 smoke test
bash scripts/smoke-test.sh
```

## 补充检查（非阻塞，建议做）

- [ ] Nginx 代理：`curl http://localhost:8080/api/auth/challenge` 通过 web 容器代理到 api-server
- [ ] Gateway 心跳入库：启动 mirage-gateway 后检查 `gateways` 表 `last_heartbeat` 更新
- [ ] Threat 上报：通过 gRPC 发送 threat event，检查 `threat_intel` 表新增记录
- [ ] Quota 变化：recharge 后 `remaining_quota` 增加
- [ ] 日志无 FATAL/panic：`docker compose logs | grep -i "fatal\|panic"` 为空
