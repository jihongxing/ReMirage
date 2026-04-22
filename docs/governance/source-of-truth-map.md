# Source of Truth Map

本文件是 ReMirage 当前治理框架下的真相源地图，回答"某类问题去哪里看、去哪里改、哪些材料不能再当主源"。

## 读取规则

- 先按问题域找主真相源。
- 主真相源不存在时，先补主真相源，再继续设计或开发。
- 输入材料只能用于迁移，不得继续平行扩写。
- 所有旧文档头部已标注 `Status / Target Truth / Migration`，可直接查看归属。

## 真相源总表

| 问题域 | 主真相源 | 类型 | 当前状态 |
|--------|----------|------|----------|
| 产品定位、非目标、商业模型与服务分层 | `boundaries/product-scope.md` | 治理文档 | authoritative |
| 系统分层与仓库边界 | `boundaries/system-context.md` | 治理文档 | authoritative |
| 组件职责归属 | `boundaries/component-responsibilities.md` | 治理文档 | authoritative |
| 协议域边界 | `boundaries/protocol-domain.md` | 治理文档 | authoritative |
| 运行时真相归属规则 | `boundaries/runtime-truth-boundaries.md` | 治理文档 | authoritative |
| 抗 DDoS 架构分层 | `boundaries/anti-ddos-architecture.md` | 治理文档 | authoritative |
| 协议域目录与分层 | `docs/protocols/README.md` + `stack.md` | 协议文档 | authoritative |
| 单协议主权归属 | `docs/protocols/source-of-truth.md` | 协议文档 | authoritative |
| G-Tunnel 协议语义 | `docs/protocols/gtunnel.md` | 协议文档 | authoritative |
| G-Switch 协议语义 | `docs/protocols/gswitch.md` | 协议文档 | authoritative |
| NPM 协议语义 | `docs/protocols/npm.md` | 协议文档 | authoritative |
| B-DNA 协议语义 | `docs/protocols/bdna.md` | 协议文档 | authoritative |
| Jitter-Lite 协议语义 | `docs/protocols/jitter-lite.md` | 协议文档 | authoritative |
| VPC 协议语义 | `docs/protocols/vpc.md` | 协议文档 | authoritative |
| Mirage OS 数据库模型 | `mirage-os/pkg/models/*.go` | 运行时代码 | authoritative |
| 跨组件 protobuf/gRPC 协议 | `mirage-proto/*.proto` | 协议文件 | authoritative |
| HTTP 接口契约 — entitlement | `docs/api/entitlement-contract.md` | 契约文档 | authoritative |
| HTTP 接口契约 — topology | `docs/api/topology-contract.md` | 契约文档 | authoritative |
| 数据库真相归属 ADR | `docs/adr/001-database-truth.md` | ADR | authoritative |
| 外部零特征审计与整改 | `docs/外部零特征消除审计与整改清单.md` | 审计 spec | authoritative |
| 密钥注入 runbook | `deploy/runbooks/secret-injection.md` | 运维资产 | authoritative |
| 最小权限模型 runbook | `deploy/runbooks/least-privilege-model.md` | 运维资产 | authoritative |
| 节点失陷替换 runbook | `deploy/runbooks/compromised-node-replacement.md` | 运维资产 | authoritative |
| Gateway 配置字段语义 | `mirage-gateway/cmd/gateway/main.go` 中 `GatewayConfig` | 运行时代码 | authoritative |
| 部署执行资产 | `deploy/` 目录 | 可执行资产 | authoritative |
| 多协议编排运行时 | `mirage-gateway/pkg/gtunnel/orchestrator.go` | 运行时代码 | 待收敛（S-01） |
| Client 数据面运行时 | `phantom-client/pkg/gtclient/client.go` | 运行时代码 | 待接入 Orchestrator |
| 隐蔽控制面 | `mirage-gateway/pkg/gtunnel/stealth/` | 运行时代码 | 部分落地 |

## 已关闭的问题域

以下问题域在本轮迁移中已从"待建立/待收敛"变为"有正式主源"：

| 问题域 | 关闭动作 |
|--------|----------|
| 抗 DDoS 架构 | 新建 `boundaries/anti-ddos-architecture.md`，从整改清单提炼四层模型 |
| 定价与计费商业模型 | 结论回写到 `product-scope.md` 商业模型与服务分层章节 |
| 运维 runbook（3 份） | 迁移到 `deploy/runbooks/`，旧文件标记为 replaced |
| 运营定位与方向 | 影子运营商结论回写到 `product-scope.md` 运营原则章节 |

## 仍需代码侧收敛的问题域

以下问题域的主源已经明确，但需要代码实现跟进才能真正关闭：

| 问题域 | 当前状态 | 收敛方向 |
|--------|----------|----------|
| 多协议编排运行时 | Orchestrator vs TransportManager 并存 | Orchestrator 为唯一主链（见审计 spec S-01） |
| Client 数据面 | 单一 QUICEngine，未接入 Orchestrator | 接入 Orchestrator 调度（见审计 spec A-01） |
| 隐蔽控制面 | StealthCP 创建后丢弃 | 绑定到 bearer 并启动（见审计 spec B-01a） |

这三项不是文档迁移问题，而是代码实现问题。文档侧的主源归属已经明确。

## 临时有效材料

以下材料仅在对应专项/发布周期内有效，完成后应归档：

| 文件 | 有效范围 |
|------|----------|
| `docs/release-readiness-traceability-index.md` | 当前发布周期 |
| `docs/Mirage 运营前高频攻击面检查清单.md` | 首批客户接入前 |
| `docs/05-实施指南/` 中 7 份整改/Spec 清单 | 对应整改任务完成前 |

## 重点规则

1. 一个问题域只有一个入口。
2. 真相源可以是代码，但必须在本地图中被明确声明。
3. 旧文档头部已标注 Status 字段，除 authoritative 外不能作为决策依据。
4. 编排运行时必须收敛为唯一主链（Orchestrator），这是代码任务不是文档任务。
