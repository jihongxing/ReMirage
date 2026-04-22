# Legacy Mapping

本文件记录 `docs/03-自研协议` 首轮迁移去向。

## 状态说明

| 状态 | 含义 |
|------|------|
| `input` | 保留为迁移输入材料，不再继续扩写 |
| `derived` | 可保留为解释性材料，但不再拥有主权 |
| `replaced` | 已被新目录中的正式入口接管 |

## 旧文档映射表

| 旧文档 | 当前状态 | 新落点 | 说明 |
|--------|----------|--------|------|
| `docs/03-自研协议/协议协同矩阵.md` | `replaced` | `docs/protocols/stack.md` | 协同关系保留，配置模板和性能假设不再作为主源 |
| `docs/03-自研协议/语言分工架构.md` | `replaced` | `docs/protocols/stack.md`、`docs/protocols/source-of-truth.md` | 语言分工降为实现说明，不再单独定义协议域 |
| `docs/03-自研协议/G-Tunnel-多路径传输协议.md` | `derived` | `docs/protocols/gtunnel.md` + `mirage-gateway/pkg/gtunnel/*` + `phantom-client/pkg/gtclient/*` | 当前有效协议语义已抽取；剩余未迁部分默认不再视为正式语义 |
| `docs/03-自研协议/G-Switch-域名转生协议.md` | `derived` | `docs/protocols/gswitch.md` + `mirage-gateway/pkg/gswitch/*` + `mirage-proto/mirage.proto` | 当前有效切换语义已抽取；愿景化扩散内容未被接管 |
| `docs/03-自研协议/NPM-流量伪装协议.md` | `derived` | `docs/protocols/npm.md` + `mirage-gateway/bpf/npm.c` + `mirage-gateway/pkg/ebpf/*` + `mirage-gateway/pkg/mcc/*` | 当前有效包长与形态语义已抽取；旧文档中的协议画像库与性能表不再作为正式语义 |
| `docs/03-自研协议/B-DNA-行为识别协议.md` | `derived` | `docs/protocols/bdna.md` + `mirage-gateway/bpf/bdna.c` + `mirage-gateway/pkg/ebpf/*` + `mirage-gateway/pkg/cortex/*` + `mirage-gateway/pkg/gswitch/*` | 当前有效指纹、JA4 与联动语义已抽取；旧文档中的完整浏览器库与理想化动态栈不再作为正式语义 |
| `docs/03-自研协议/Jitter-Lite-时域扰动协议.md` | `derived` | `docs/protocols/jitter-lite.md` + `mirage-gateway/bpf/jitter.c` + `mirage-gateway/pkg/jitter/*` + `mirage-gateway/pkg/ebpf/*` | 当前有效时域语义已抽取；旧文档中的理想化模板矩阵不再作为正式语义 |
| `docs/03-自研协议/VPC-噪声注入协议.md` | `derived` | `docs/protocols/vpc.md` + `mirage-gateway/pkg/ebpf/*` + `mirage-gateway/pkg/threat/*` + `mirage-gateway/pkg/cortex/*` + `mirage-gateway/bpf/jitter.c` | 当前有效语义已抽取；旧文档中未落地的检测细节不再作为正式语义 |

## 本轮迁移完成标准

本轮不是“把旧文档全部改写完”，而是完成以下三件事：

1. 新目录已接管协议域入口。
2. 各协议主真相源已经登记。
3. 旧协议文档默认停止承担长期主源职责。

## 下一轮建议

下一轮最值得继续做的是：

1. 将旧协议文档逐步补上文档状态标记。
2. 继续收敛 `NPM` 的 `npm_global_map` / `npm_config_map` 命名与结构。
3. 继续收敛 `B-DNA` 的 `active_profile_map` / `fingerprint_map` / `dna_template_map` 边界。
