# mirage-proto — 协议中枢 (Single Source of Truth)

所有 gRPC 服务定义和消息类型的唯一权威来源。

## 架构原则

- **单点真实源**：`.proto` 文件只在此目录维护，禁止在 OS/Gateway/Client 中各自维护副本
- **统一编译**：`make gen` 生成 Go 代码到 `gen/` 目录
- **两端引用**：OS 和 Gateway 通过 `go.mod replace` 指向此模块

## 使用方式

```bash
# 生成代码
make gen

# 清理
make clean
```

## 引用方式

在 `mirage-gateway/go.mod` 和 `mirage-os/gateway-bridge/go.mod` 中：

```
require mirage-proto v0.0.0

replace mirage-proto => ../mirage-proto
```

代码中：

```go
import pb "mirage-proto/gen"
```

## Schema 演进规则

1. **永远不要删除或重命名字段** — 使用 `reserved` 标记废弃字段
2. **新增字段必须使用新的 field number** — 旧客户端会忽略未知字段
3. **业务代码必须容忍缺失字段** — 旧版 Gateway 不会发送新字段
