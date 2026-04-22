# 任务清单：Release Readiness P1 Gateway Control Hardening

## P1-A：防护链路真正生效

- [x] 1. 将 `HandshakeGuard` 接入真实 listener
  - [x] 1.1 定位 Gateway 真实入站 listener
  - [x] 1.2 用 `WrapListener()` 包裹该 listener
  - [x] 1.3 补充握手超时命中测试

- [x] 2. 将 `NonceStore` 与 `ProtocolDetector` 接入真实入口
  - [x] 2.1 在命令或会话入口加入 nonce 校验
  - [x] 2.2 在协议准入路径加入 `ProtocolDetector`
  - [x] 2.3 删除主程序中 `_ = nonceStore`、`_ = protocolDetector` 之类的死代码抑制

- [x] 3. 修复 XDP SYN validation 绕过
  - [x] 3.1 重写 `l1_defense.c` 中 challenge/ack 校验逻辑
  - [x] 3.2 增加 ACK 伪造回归测试
  - [x] 3.3 若无法满足设计目标，明确降级文档与注释

## P1-B：控制面安全与 V2 运行时

- [x] 4. 收紧高危命令 HMAC
  - [x] 4.1 对高危命令强制 `nonce` 和 `payload-hash`
  - [x] 4.2 更新 `security_regression_test.go`
  - [x] 4.3 拒绝仅 `commandType + timestamp` 的旧签名样例

- [x] 5. 修复 rate limiter 的 `ip:port` 绕过
  - [x] 5.1 规整 `peerAddr()` 为源 IP
  - [x] 5.2 补充端口轮换绕过测试

- [x] 6. 让 V2 dispatcher 具备运行态 handler
  - [x] 6.1 在 Gateway 启动阶段注册生产 handler
  - [x] 6.2 让 V2 adapter 投递事件后不再因无 handler 失败
  - [x] 6.3 补充运行态 V2 闭环测试

- [x] 7. 让威胁事件缓存能在重连后补发
  - [x] 7.1 在 `GRPCClient.Connect` 或重连成功路径触发 flush
  - [x] 7.2 对满缓冲和断线恢复场景补测试

## P1-C：观测与门禁

- [x] 8. 修复审计日志双重结论
  - [x] 8.1 移除无条件 `defer success log`
  - [x] 8.2 保证每个请求只记录一个最终状态

- [x] 9. 修复 L1 指标重复累计
  - [x] 9.1 为 `syncStats()` 保存上次快照
  - [x] 9.2 按 delta 更新 Prometheus

- [x] 10. 修复 `pkg/orchestrator/events` 测试资产失配
  - [x] 10.1 更新 `types_test.go`
  - [x] 10.2 跑通 `go test ./pkg/orchestrator/events -v`

## 检查点

- [x] C1. L2 组件在真实主路径生效
- [x] C2. 高危命令重放/替换被拒绝
- [x] C3. V2 handler 注册完成
- [x] C4. threat buffer 可恢复补发
- [x] C5. 审计、指标、测试结果可信

