# M4 业务连续性样板受控演练报告

## 概述

本报告记录 Phase 1 M4 里程碑的业务连续性样板验证结果。

- 证据强度：代码级模拟（mock transport），非真实网络演练
- 验证范围：switchWithTransaction 事务式切换 + 业务数据流连续性

## 传输层切换结果

| 场景 | 结果 | 说明 |
|------|------|------|
| 同 IP 切换（adoptConnection） | ✅ 通过 | 不触发 PreAdd/Commit，直接接管 |
| PreAdd 失败 | ✅ 通过 | 关闭新 engine，不修改当前活跃连接 |
| PreAdd→adoptConnection→Commit 正常流程 | ✅ 通过 | currentGW 正确更新 |
| 有 ClientOrchestrator 时 adoptConnection | ✅ 通过 | Orchestrator 内部 active 替换，transport 本身不变 |
| 同 IP 幂等性 PBT（100 次） | ✅ 通过 | 随机 IP/Port/Region，同 IP 不触发 PreAdd/Commit |
| 状态一致性 PBT（100 次） | ✅ 通过 | 切换后 currentGW 与输入一致 |

## 业务层影响量化

| 指标 | mock 环境数据 | 说明 |
|------|--------------|------|
| 切换耗时 | < 1μs | mock transport 无网络延迟 |
| 丢包数量 | 0 | mock 环境无真实丢包 |
| 业务恢复时间 | 0 | mock 环境切换即时生效 |

## 当前版本业务连续性边界

### 已验证场景

- 传输层切换（switchWithTransaction）在 mock 环境下正确执行
- 事务式切换的三阶段（PreAdd→adoptConnection→Commit）逻辑正确
- PreAdd 失败时不修改当前活跃连接，业务数据流不中断
- 有 ClientOrchestrator 时，adoptConnection 正确替换内部 active

### 未验证场景（需后续真实网络演练）

- 真实 QUIC/WSS 切换时的丢包数量和恢复时间
- 高并发请求流下的切换影响
- 跨地域切换的延迟影响
- Commit 失败场景（当前实现无 Rollback 调用路径）

### 边界说明

当前版本的业务连续性验证仅覆盖传输层切换逻辑正确性。mock 环境下丢包为 0、切换耗时为微秒级，但这不能直接推导为"业务层无感"。真实网络环境下的业务层影响（丢包数、恢复时间）需要单独的真实网络演练来量化。

## 复验命令

```bash
bash deploy/scripts/drill-m4-continuity.sh
```
