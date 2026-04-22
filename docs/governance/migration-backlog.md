# Migration Backlog

## 迁移原则

1. 先迁移结论，再迁移背景材料。
2. 先建立权威入口，再处理旧资料的状态标记。
3. 对同类旧文档，优先提炼共识，不逐篇照搬。

## 已完成的迁移轮次

### 第一轮：治理骨架（已完成）
建立 governance 目录、边界定义、真相源地图、文档生命周期规则。

### 第二轮至第七轮：协议域迁移（已完成）
六大协议正式入口全部建立，NPM/B-DNA 收口，版本化 registry 落地。

### 第八轮：全量状态标记（已完成）
docs/ 下所有非 governance/protocols 的 46 份文件完成 front-matter 状态标记。

### 第九轮：缺口关闭（已完成）

| 缺口 | 关闭动作 | 结果 |
|------|----------|------|
| runbook 仍在 docs/ 承载操作流程 | 3 份 runbook 迁移到 `deploy/runbooks/`，旧文件标记 replaced | 部署资产主源收口 |
| 抗 DDoS 架构无独立主源 | 新建 `boundaries/anti-ddos-architecture.md`，从整改清单提炼四层模型 | 主源已建立 |
| 定价/计费商业模型无正式入口 | 结论回写到 `product-scope.md` 商业模型与服务分层章节 | 主源已建立 |
| 方向确认纪要仍是 input | 核心结论回写到 `product-scope.md`，纪要降级为 derived | 主源已建立 |

## 当前状态分布

| 状态 | 数量 | 说明 |
|------|------|------|
| authoritative | 28 | 各问题域的权威入口（含 governance/protocols/api/adr/runbooks/审计 spec） |
| derived | 12 | 派生物或解释性材料，主源在别处 |
| input | 14 | 迁移输入材料，结论已回写或待回写到对应主源 |
| temporary | 9 | 专项/整改/发布周期材料，完成后归档 |
| replaced | 5 | 已被新入口完全接管 |
| archived | 2 | 已被明确否决或废弃的方向 |

## 仍需代码侧收敛的问题域

以下不是文档迁移问题，而是代码实现问题。文档侧的主源归属已经明确：

| 问题域 | 文档主源 | 代码收敛方向 |
|--------|----------|-------------|
| 多协议编排 | ✅ Orchestrator 被动模式已接入 Gateway main.go | TransportManager 标记 deprecated |
| Client 数据面 | ✅ ClientOrchestrator 已接入生产入口 | QUIC 主路径走统一 Transport，WSS 降级待实现 |
| 隐蔽控制面 | ⏳ StealthCP ReceiveLoop 运行中（ChannelQueued） | 待 QUIC/H3 bearer 建立后重建实例 |
| Gateway 数据面注入 | ⏳ 编排主链连通，IP 包注入待 TUN/NFQUEUE | 独立基础设施组件 |
| Client WSS 降级 | ⏳ 接口已定义，协议实现待补 | 需 WebSocket Upgrade + mTLS |

## 剩余 input 文件的处理建议

当前 14 份 input 文件中，结论已回写到主源的可进一步降级为 derived：

| 文件 | 建议动作 |
|------|----------|
| V2 编排内核设计草案 / 升级纪要 | 待 V2 代码落地后降级为 archived |
| 隐蔽控制面架构 | 待 StealthCP 接入后降级为 derived |
| 客户接入说明 | 待正式接入文档建立后降级为 derived |
| 安全软件兼容指南 | 待正式接入文档建立后降级为 derived |
| 零信任架构 | 待安全架构独立入口建立后降级为 derived |
| 交付技术规范 | 待部署资产体系完善后降级为 derived |
| Phantom 蜜罐策略审计 | 待 Phantom 实现收口后降级为 derived |
| 总体规划 / 架构概述 / 二元架构 / 组件说明 | 结论已在 governance 边界文件中，可降级为 derived |

## 完成标准

docs/ 目录唯一真相源改造的完成标准：

1. ✅ 每个问题域都有明确的主真相源（已完成）
2. ✅ 所有旧文档头部有状态标记（已完成）
3. ✅ 后续新增内容不再写回旧位置（规则已建立）
4. ⏳ 代码侧编排/数据面收敛（非文档任务，待代码实现）
5. ⏳ 9 份 temporary 整改清单完成后归档（待任务完成）
