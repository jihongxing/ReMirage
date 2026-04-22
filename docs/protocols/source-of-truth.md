# Protocol Source of Truth

本文件定义协议域内各协议或协议族的主真相源。

## 判定规则

对于协议问题，先判断它属于哪一类：

1. 共享消息、字段、命令结构。
2. 单协议运行时行为。
3. 协议目录、边界和拥有者说明。
4. 旧资料迁移去向。

不同类型的问题，不共享同一个真相源。

## 真相源总表

| 协议或问题域 | 主真相源 | 运行时锚点 | 当前状态 | 旧资料状态 |
|--------------|----------|------------|----------|------------|
| 协议域目录与边界 | `docs/protocols/README.md`、`docs/protocols/stack.md` | 无 | 已建立 | 已接管目录入口 |
| 共享控制消息与会话语义 | `mirage-proto/mirage.proto` | `mirage-proto/gen/*`、调用方实现 | 已采纳 | 旧文档降为解释材料 |
| 控制命令总线 | `mirage-proto/control_command.proto` | `mirage-proto/gen/control_command.pb.go`、消费方实现 | 已采纳 | 旧文档暂无独立主源 |
| G-Tunnel 传输承载 | `docs/protocols/gtunnel.md` | `mirage-gateway/pkg/gtunnel/*`、`phantom-client/pkg/gtclient/*` | 第二轮已收敛 | 旧文档降为输入材料 |
| G-Switch 存活与切换 | `docs/protocols/gswitch.md` | `mirage-gateway/pkg/gswitch/*`、`mirage-proto/mirage.proto` 中的转生消息 | 第二轮已收敛 | 旧文档降为输入材料 |
| NPM 包长与形态伪装 | `docs/protocols/npm.md` | `mirage-gateway/bpf/common.h`、`mirage-gateway/bpf/npm.c`、`mirage-gateway/pkg/ebpf/*`、`mirage-gateway/pkg/mcc/*`、`mirage-gateway/pkg/api/handlers.go` | 第六轮已收敛 | 旧文档降为解释性输入材料 |
| B-DNA 指纹伪装 | `docs/protocols/bdna.md` | `mirage-gateway/bpf/bdna.c`、`mirage-gateway/pkg/ebpf/bdna_profile_updater.go`、`mirage-gateway/configs/bdna/profile-registry.v1.json`、`mirage-gateway/pkg/ebpf/dna_updater.go`、`mirage-gateway/pkg/cortex/*`、`mirage-gateway/pkg/gswitch/*` | 第七轮已收敛 | 旧文档降为解释性输入材料 |
| Jitter-Lite 时域扰动 | `docs/protocols/jitter-lite.md` | `mirage-gateway/bpf/jitter.c`、`mirage-gateway/pkg/jitter/*`、`mirage-gateway/pkg/ebpf/*` | 第四轮已收敛 | 旧文档降为解释性输入材料 |
| VPC 背景噪声与威胁自适应 | `docs/protocols/vpc.md` | `mirage-gateway/pkg/ebpf/*`、`mirage-gateway/pkg/threat/*`、`mirage-gateway/pkg/cortex/*`、`mirage-gateway/bpf/jitter.c` | 第三轮已收敛 | 旧文档降为解释性输入材料 |
| WebRTC / DNS / ICMP 回退承载 | `mirage-gateway/pkg/gtunnel/*` 中对应 transport 实现 | `phantom-client/pkg/resonance/*` 与 `phantom-client/pkg/gtclient/*` | 已识别 | 分散在旧矩阵文档中 |

## 当前最重要的几条规则

### 共享字段去看 `.proto`

任何涉及消息字段、枚举、RPC 名称、命令结构的问题，都必须先看：

- `mirage-proto/mirage.proto`
- `mirage-proto/control_command.proto`

### 单协议行为去看运行时实现

例如：

- `G-Tunnel` 的多路径、FEC、回退承载要看 `mirage-gateway/pkg/gtunnel/*` 和 `phantom-client/pkg/gtclient/*`。
- `G-Switch` 的转生与 DNS-less 行为要看 `mirage-gateway/pkg/gswitch/*`。
- `NPM`、`B-DNA`、`Jitter-Lite` 的底层行为要看对应 `bpf/*.c`。

但从现在开始，若是要理解 `G-Tunnel` / `G-Switch` 的“当前有效协议语义”，应先看：

- `docs/protocols/gtunnel.md`
- `docs/protocols/gswitch.md`
- `docs/protocols/npm.md`
- `docs/protocols/bdna.md`
- `docs/protocols/vpc.md`
- `docs/protocols/jitter-lite.md`

### 协议目录问题去看这里

协议如何分层、某个协议属于哪一族、旧文档是否还能继续写，统一以本目录为准。

## 本轮收口结果

本轮已经收掉两处之前显式登记的缺口：

1. `NPM` 已统一为 `struct npm_config` + `npm_config_map`。
2. `B-DNA` 已明确拆成：
   - `active_profile_map` / `fingerprint_map` 的握手画像链路
   - `dna_template_map` 的跨协议协同模板链路
3. `B-DNA` 默认画像库已经外推成版本化 registry：
   - `mirage-gateway/configs/bdna/profile-registry.v1.json`
   - `pkg/ebpf/bdna_profile_updater.go` 负责解析、校验、装载

这意味着当前伪装扰动层里：

- `NPM` 的主配置面已经统一。
- `B-DNA` 的画像主线已经独立。
- `dna_template_map` 不再被继续误认为“唯一的 B-DNA 主配置面”。
- 默认画像库也不再以内嵌 Go 种子充当事实主源。

## 当前仍保留的次级缺口

若继续往下收，当前更值得处理的已经不是命名冲突，而是更细一层的运行时主源治理：

1. `dna_template_map` 虽然边界已经清晰，但它和 `Jitter-Lite` / `NPM` 的协同字段未来仍可继续拆细。
2. `B-DNA` registry 目前已经版本化，但还没有形成多版本并存、灰度切换与签名校验机制。

后续若继续迁移，优先目标不是重写概念文档，而是继续把这些运行时子域拆成更稳定的主源。
