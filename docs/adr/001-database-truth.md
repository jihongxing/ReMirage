---
Status: authoritative
Target Truth: 本文件为数据库真相归属的 ADR，运行时主源为 mirage-os/pkg/models/*.go
Migration: 无需迁移，已在真相源地图中登记
---

# ADR-001: GORM Models 为运行时数据库真相

## 状态

已采纳 (Accepted)

## 背景

Mirage OS 当前存在三套数据库模型定义：
1. **GORM Models** (`mirage-os/pkg/models/db.go`) — Go 运行时使用
2. **Prisma Schema** (`mirage-os/api-server/src/prisma/schema.prisma`) — NestJS API 层使用
3. **Raw SQL** (`mirage-os/gateway-bridge/pkg/topology/registry.go`) — Gateway Bridge 直接 SQL

三套定义的主键命名、字段集合、类型语义存在不一致，导致跨服务数据操作出现隐性 bug。

## 决策

**声明 `mirage-os/pkg/models/db.go` 中的 GORM Models 为运行时数据库真相（Single Source of Truth）。**

具体规则：
1. 所有表结构变更必须先修改 GORM Models，再同步到其他层
2. Prisma Schema 降级为**适配层/只读层**，仅用于 NestJS API 的查询映射
3. Gateway Bridge 的 raw SQL 必须使用与 GORM Models 一致的列名
4. 新增服务必须通过 GORM 或直接引用 `models` 包访问数据库

## 影响范围

| 表名 | GORM 主键 | Prisma 主键 | 对齐方案 |
|------|-----------|-------------|---------|
| users | `user_id` (业务ID) + `id` (自增) | `id` (UUID) | Prisma `id` 映射到 GORM `user_id` |
| gateways | `gateway_id` (业务ID) + `id` (自增) | `id` (字符串) | Prisma `id` 映射到 GORM `gateway_id` |
| billing_logs | `id` (自增) + `log_id` (UUID) | `id` (UUID) | Prisma `id` 映射到 GORM `log_id` |

## 后果

- Gateway Bridge SQL 需要重写以匹配 GORM 列名
- Prisma Schema 头部需标记为适配层
- 需要提供迁移脚本处理已有数据
