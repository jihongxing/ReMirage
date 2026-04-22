# 设计文档：Release Readiness P1 Gateway Control Hardening

## 概述

Gateway 当前的主要问题不是“代码不存在”，而是：

1. 防护组件停留在初始化层
2. 数据面校验过于乐观
3. 控制面安全策略有例外口子
4. 观测与测试不能真实代表运行态

本 Spec 用“运行主路径接线”作为首要原则。

---

## 模块一：L2 防护接线

### 方案

1. 找到 Gateway 的实际连接入口，将 `HandshakeGuard.WrapListener()` 包裹到真实 listener
2. 让协议探测与 nonce 校验进入命令/会话入口，而不是只保留库代码
3. 所有已启用防护组件必须在启动日志中输出“已接线”而不是“已初始化”

---

## 模块二：L1 SYN 验证重构

### 方案

1. 保留 XDP 前移方向，但修复“只要 ACK 就过”的逻辑
2. 验证条件至少需要绑定 challenge 状态与 ACK 可推导字段
3. 若当前 eBPF 约束下无法做到完整 SYN cookie 语义，则应降级声明，不得继续宣称“无状态验证已完成”

---

## 模块三：控制面鉴权与限流收口

### 方案

1. 将高危命令定义为必须带完整 metadata 的命令集合
2. `CommandAuthenticator` 对高危命令强制 `nonce + payload-hash`
3. `peerAddr()` 输出在进入 rate limiter 前规整成源 IP
4. 回归测试不再接受旧的弱签名样例

---

## 模块四：V2 运行时接线

### 方案

1. Gateway 启动时注册 V2 handler，而不是只 new registry/dispatcher
2. 将关键 legacy 命令的 V2 处理结果接入 commit/ack/回执路径
3. `GRPCClient` 在连接恢复后自动 flush `eventBuffer`

---

## 模块五：观测与测试可信化

### 方案

1. 审计日志改为“单次请求单次结论”
2. L1 指标改为基于上次快照计算 delta
3. 修正 `events/types_test.go` 与现有 `AllEventTypes` 的失配

