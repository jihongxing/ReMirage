---
Status: authoritative
---

# 生产节点密钥注入 Runbook

## 原则
- 密钥不通过镜像固化
- 生产 K8s 部署：密钥不通过普通环境变量长期保存，使用 Kubernetes Secret 或 HashiCorp Vault 注入
- Compose 过渡部署：密钥通过宿主机环境变量注入（参见"过渡方案"章节）

## 流程（生产 K8s 部署）
1. 在安全工作站生成密钥材料
2. 通过 `kubectl create secret` 或 Vault API 注入到集群
3. Pod 通过 Volume Mount 读取密钥（tmpfs）
4. 密钥仅存在于内存中，Pod 销毁后自动清除

## 过渡方案（Compose 部署）

> **适用边界**: 仅限受控网络内的 Docker Compose 部署（如内网开发/测试/预发布环境）。
> 生产 Kubernetes 部署必须使用上述 K8s Secret / Vault 流程。

当前发布版本的 Compose 部署（`deploy/docker-compose.os.yml`）采用宿主机环境变量注入密钥：

- 密钥通过 `${MIRAGE_DB_PASSWORD}`、`${MIRAGE_JWT_SECRET}`、`${MIRAGE_QUERY_HMAC_SECRET}`、`${MIRAGE_REDIS_PASSWORD}` 等环境变量从宿主机传入容器
- 部署前须在宿主机通过 `.env` 文件或 shell export 设置这些变量，禁止硬编码
- 此方式依赖宿主机操作系统的进程隔离和文件权限保护环境变量安全
- `.env` 文件权限应设为 `600`，仅限部署操作用户读取

**过渡方案限制**:
- 环境变量可能通过 `/proc/<pid>/environ` 泄露，仅适用于受控网络
- 不适用于多租户或公网可达的部署场景
- 后续版本将迁移至 Docker Secrets 或 Vault Agent Sidecar

**迁移路径**:
1. 当前版本：Compose 环境变量注入（受控网络）
2. 下一版本：Docker Swarm Secrets 或 Compose Secrets（`secrets:` 顶级配置）
3. 生产目标：K8s Secret / Vault Volume Mount（本 Runbook "流程"章节）

## 密钥清单
- `MIRAGE_DB_PASSWORD`: PostgreSQL 数据库密码
- `MIRAGE_JWT_SECRET` (`JWT_SECRET`): API Server JWT 签名密钥
- `MIRAGE_QUERY_HMAC_SECRET` (`QUERY_HMAC_SECRET`): 内部查询 HMAC 密钥
- `MIRAGE_REDIS_PASSWORD`: Redis 鉴权密码，用于 `requirepass` 配置及消费方连接串
- `INTERNAL_HMAC_SECRET`: 内部接口 HMAC 密钥
- `BRIDGE_INTERNAL_SECRET`: Gateway Bridge 内部鉴权密钥
- `COMMAND_SECRET`: Gateway 命令签名密钥
- CA 证书私钥: 仅存在于 OS 节点，Gateway 不持有

## Redis 鉴权说明
- Redis 服务通过 `--requirepass ${MIRAGE_REDIS_PASSWORD}` 启用密码鉴权
- 消费方（`gateway-bridge`、`api-server`）通过 `redis://:${MIRAGE_REDIS_PASSWORD}@redis:6379` 连接
- 未提供密码的连接将被 Redis 拒绝（`NOAUTH` 错误）
- `MIRAGE_REDIS_PASSWORD` 必须在部署前通过安全渠道注入，禁止硬编码
