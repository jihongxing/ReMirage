# Mirage-OS 部署指南

## V1 核心版部署清单

### 1. 环境准备

**系统要求**：
- PostgreSQL 14+
- Redis 6+
- Go 1.21+
- Node.js 18+（前端）

**GeoIP 数据库**（可选）：
```bash
# 下载 MaxMind GeoLite2-City 数据库
# 注册账号：https://www.maxmind.com/en/geolite2/signup
# 下载后放置到：/opt/geoip/GeoLite2-City.mmdb
```

### 2. 数据库初始化

```bash
# 创建数据库
psql -U postgres -c "CREATE DATABASE mirage_os;"

# 执行初始化脚本
psql -U postgres -d mirage_os -f mirage-os/scripts/init.sql
```

### 3. 环境变量配置

创建 `.env` 文件：

```bash
# 数据库配置
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=mirage_os

# Redis 配置
REDIS_ADDR=localhost:6379

# GeoIP 配置（可选）
GEOIP_DB_PATH=/opt/geoip/GeoLite2-City.mmdb

# gRPC 端口
GRPC_PORT=50051

# WebSocket 端口
WS_PORT=8080
```

### 4. 启动服务

**API Gateway**：
```bash
cd mirage-os/services/api-gateway
go run main.go server.go
```

**WebSocket Gateway**：
```bash
cd mirage-os/services/ws-gateway
go run main.go hub.go geoip.go
```

**前端 Dashboard**：
```bash
cd mirage-os/web
npm install
npm run dev
```

### 5. 验证部署

**检查 API Gateway**：
```bash
# 使用 grpcurl 测试
grpcurl -plaintext localhost:50051 list
```

**检查 WebSocket**：
```bash
# 运行测试脚本
bash mirage-os/scripts/test-websocket.sh
```

**检查前端**：
```
浏览器访问：http://localhost:5173
```

### 6. 三大对齐验证

**生死裁决硬对齐**：
```bash
# 设置用户配额为 0
psql -U postgres -d mirage_os -c "UPDATE users SET remaining_quota = 0 WHERE user_id = 'test-user';"

# 触发心跳，观察熔断
bash mirage-os/scripts/test-heartbeat.sh
```

**协议栈名实对齐**：
```bash
# 检查 jitter.c 处理链条
cat mirage-gateway/bpf/jitter.c | grep "SEC("
# 应输出：
# SEC("tc") int vpc_ingress_detect
# SEC("tc") int jitter_egress
```

**标准化目录对齐**：
```bash
# 验证目录结构
tree mirage-gateway/pkg
tree mirage-gateway/bpf
```

### 7. 全链路实战演习

```bash
# 运行所有场景测试
cd mirage-os/scripts
bash run-all-scenarios.sh

# 运行 Gateway 实战测试
cd mirage-gateway/scripts
bash run-all-combat-tests.sh
```

---

## V2 生产版演进路径

### 1. 全内存运行（tmpfs）

```bash
# 创建内存挂载点
sudo mkdir -p /mnt/mirage-tmpfs
sudo mount -t tmpfs -o size=2G tmpfs /mnt/mirage-tmpfs

# 配置 PostgreSQL 使用内存表空间
psql -U postgres -d mirage_os -c "CREATE TABLESPACE mirage_mem LOCATION '/mnt/mirage-tmpfs';"
```

### 2. Shamir 碎片化存储

```bash
# TODO: 实现 Shamir 秘密共享
# 将敏感数据分片存储到多个地理位置
```

### 3. B-DNA 流量指纹建模

```bash
# TODO: 独立实现 B-DNA 协议
# JA4 指纹伪装、TCP Window Size 修改
```

---

## 故障排查

### GeoIP 定位失败

**症状**：前端 3D 地球无法显示节点位置

**解决**：
1. 检查 GeoIP 数据库路径是否正确
2. 查看日志：`GeoIP: 未提供数据库路径，使用占位实现`
3. 下载 MaxMind GeoLite2-City.mmdb 并配置环境变量

### Redis 连接失败

**症状**：WebSocket 无法推送实时数据

**解决**：
1. 检查 Redis 是否启动：`redis-cli ping`
2. 检查端口：`netstat -an | grep 6379`
3. 检查环境变量：`REDIS_ADDR`

### 数据库迁移失败

**症状**：启动时报错 `数据库迁移失败`

**解决**：
1. 检查数据库连接：`psql -U postgres -d mirage_os -c "SELECT 1;"`
2. 手动执行迁移：`psql -U postgres -d mirage_os -f mirage-os/scripts/init.sql`
3. 检查表是否存在：`\dt`

---

## 性能优化

### PostgreSQL 优化

```sql
-- 调整连接池
ALTER SYSTEM SET max_connections = 200;

-- 调整共享内存
ALTER SYSTEM SET shared_buffers = '2GB';

-- 调整工作内存
ALTER SYSTEM SET work_mem = '64MB';

-- 重启生效
SELECT pg_reload_conf();
```

### Redis 优化

```bash
# 调整最大内存
redis-cli CONFIG SET maxmemory 2gb

# 设置淘汰策略
redis-cli CONFIG SET maxmemory-policy allkeys-lru
```

---

## 监控指标

### 关键指标

| 指标 | 阈值 | 说明 |
|------|------|------|
| API 响应时间 | < 100ms | gRPC 心跳响应 |
| WebSocket 延迟 | < 50ms | 实时推送延迟 |
| 数据库连接数 | < 150 | PostgreSQL 连接池 |
| Redis 内存使用 | < 80% | 缓存命中率 |
| GeoIP 查询时间 | < 10ms | IP 定位性能 |

### 监控命令

```bash
# 查看 API Gateway 日志
tail -f /var/log/mirage-os/api-gateway.log

# 查看 WebSocket 连接数
redis-cli CLIENT LIST | wc -l

# 查看数据库连接数
psql -U postgres -d mirage_os -c "SELECT count(*) FROM pg_stat_activity;"
```

---

## 安全加固

### 1. 数据库安全

```sql
-- 创建只读用户
CREATE USER mirage_readonly WITH PASSWORD 'readonly_password';
GRANT SELECT ON ALL TABLES IN SCHEMA public TO mirage_readonly;

-- 限制连接来源
-- 编辑 pg_hba.conf
host    mirage_os    mirage_readonly    127.0.0.1/32    md5
```

### 2. Redis 安全

```bash
# 设置密码
redis-cli CONFIG SET requirepass "your_redis_password"

# 禁用危险命令
redis-cli CONFIG SET rename-command FLUSHDB ""
redis-cli CONFIG SET rename-command FLUSHALL ""
```

### 3. GeoIP 数据保护

```bash
# 限制文件权限
chmod 600 /opt/geoip/GeoLite2-City.mmdb
chown mirage:mirage /opt/geoip/GeoLite2-City.mmdb
```

---

## V1 核心版封版清单

- [x] 数据库 Schema（numeric(20,8) 精度）
- [x] gRPC 生死裁决（SyncHeartbeat）
- [x] 威胁检测与上报（Ring Buffer）
- [x] 计费系统（事务原子性）
- [x] WebSocket 实时推送（Redis Pub/Sub）
- [x] GeoIP 定位服务（全球视野坐标对齐）
- [x] 3D 看板（Three.js + React）
- [x] 协议栈名实对齐（VPC + Jitter-Lite + G-Tunnel）
- [x] 生死裁决硬对齐（内核态配额优先）
- [x] 标准化目录结构（defense/ + tunnel/ + billing/）

**V1 核心版已完成，可进入全链路实战演习阶段。**
