---
Status: authoritative
---

# 生产节点密钥注入 Runbook

## 原则
- 密钥不通过镜像固化
- 密钥不通过普通环境变量长期保存
- 使用 Kubernetes Secret 或 HashiCorp Vault 注入

## 流程
1. 在安全工作站生成密钥材料
2. 通过 `kubectl create secret` 或 Vault API 注入到集群
3. Pod 通过 Volume Mount 读取密钥（tmpfs）
4. 密钥仅存在于内存中，Pod 销毁后自动清除

## 密钥清单
- `JWT_SECRET`: API Server JWT 签名密钥
- `INTERNAL_HMAC_SECRET`: 内部接口 HMAC 密钥
- `BRIDGE_INTERNAL_SECRET`: Gateway Bridge 内部鉴权密钥
- `COMMAND_SECRET`: Gateway 命令签名密钥
- CA 证书私钥: 仅存在于 OS 节点，Gateway 不持有
