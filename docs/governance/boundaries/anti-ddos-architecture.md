# Anti-DDoS Architecture Boundary

本文件是 ReMirage 抗 DDoS 架构的权威入口，从整改清单中提炼的分层结论固化于此。

## 核心定位

Mirage 的抗 DDoS 目标不是让每个公网节点成为永远打不死的堡垒，而是让受击节点可以被快速识别、快速放弃、快速替换，并让整套系统在 OS 编排与 Client 自愈能力支撑下持续存活。

## 四层防御模型

### Layer 1：Upstream / 物理层

超过单节点物理带宽的体积型攻击，不应寄希望于单节点软件防御。此层依赖上游链路清洗或云厂商能力。

### Layer 2：Gateway / 入口准入层

负责处理非法协议画像、握手洪泛、连接爆发、小到中等规模资源耗尽型攻击。目标是把无效流量尽早丢弃在 XDP / TC / 无状态准入验证，不把压力带进完整协议栈。

运行时锚点：
- `mirage-gateway/bpf/l1_defense.c`（XDP ingress guard）
- `mirage-gateway/pkg/threat/blacklist.go`（黑名单管理）
- `mirage-gateway/pkg/ebpf/loader.go`（eBPF 挂载）

### Layer 3：OS / 编排裁决层

负责判定节点是否还应继续接客、是否应进入 UnderAttack、是否已死亡并需要替补。OS 的任务不是替节点抗攻击，而是在节点失去战斗力时快速重组系统。

运行时锚点：
- `mirage-os/` 中的 Gateway 状态管理
- `mirage-proto/*.proto` 中的心跳与状态上报

### Layer 4：Client / 生存恢复层

负责受击 Gateway 失联后的被动恢复、拓扑学习、备用节点发现、会话重建。如果 Client 不具备持续学习与自愈能力，任何"可耗材化网关"都只是纸面概念。

运行时锚点：
- `phantom-client/pkg/gtclient/client.go`（三级降级恢复）
- `phantom-client/pkg/resonance/`（信令共振发现）

## 两类攻击的严格区分

### 软防场景：资源耗尽型攻击
- 物理带宽未被打满，压力在握手洪泛/连接状态/CPU
- Gateway 继续存活，OS 停止分配新用户，老会话保留

### 硬断场景：体积型攻击
- 上游链路被塞满，节点心跳丧失
- OS 按死亡节点处理，启动备用节点，Client 独立恢复

## 架构原则

1. 不做单协议假设 — 每个入口绑定允许的协议画像
2. 不做单 IP 粗暴限流 — 考虑 CGNAT/企业共享出口
3. 尽可能前移到 XDP — 能在 XDP 做的不拖到 TC
4. 体积型攻击下优先保全系统而不是保全节点

## 节点状态语义

| 状态 | 含义 |
|------|------|
| ONLINE | 正常服务 |
| UNDER_ATTACK | 受压但仍可服务，停止接纳新用户 |
| DRAINING | 正在排空会话 |
| DEAD | 物理不可用，不再假设能完成排空 |

## 变更规则

- 修改抗 DDoS 分层策略前，先更新本文件。
- 新增节点状态语义前，先在本文件中定义。
- 整改清单中的具体任务完成后，结论回写到本文件对应层级。
