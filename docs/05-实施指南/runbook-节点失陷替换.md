---
Status: replaced
Target Truth: deploy/runbooks/compromised-node-replacement.md
Migration: 已迁移到 deploy/runbooks/compromised-node-replacement.md，本文件不再维护
---

# 节点失陷后替换流程 Runbook

## 目标
在限定时间内（< 30 分钟）替换单个失陷 Gateway 节点

## 前提
- 失陷节点不持有 Root CA 私钥（证书 72h 自然过期）
- 其他节点不受影响

## 步骤

### 1. 隔离（< 5 分钟）
```bash
# 从负载均衡器移除
kubectl cordon <node>
kubectl drain <node> --force --ignore-daemonsets
```

### 2. 证书失效确认
- 失陷节点证书最长 72h 后自然过期
- 无需手动吊销

### 3. 部署替换节点（< 15 分钟）
```bash
# 新节点加入集群
kubectl uncordon <new-node>
kubectl label node <new-node> mirage.io/role=gateway
```

### 4. 验证（< 10 分钟）
```bash
# 检查新节点健康
curl http://<new-node>:8081/healthz
# 检查 eBPF 加载状态
curl http://<new-node>:8081/status
```

### 5. 清理
```bash
# 安全擦除失陷节点磁盘
# 重装操作系统
```
