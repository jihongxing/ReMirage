---
Status: input
Target Truth: mirage-gateway/pkg/phantom/ (运行时实现)
Migration: Phantom 策略审计，结论应回写到 Phantom 实现
---

# Phantom / 蜜罐策略审计与收敛建议

## 一、文档目标

本文档用于承接本次对 Mirage `Phantom / 蜜罐策略` 相关实现的静态审计结论，并给出一套更适合 Mirage 当前路线的收敛建议。

这里的“收敛”不是指削弱能力，而是指：

- 降低系统自我暴露
- 降低误判和误伤
- 提高策略可控性和可回退性
- 让 Phantom 更符合 Mirage “影子运营商”路线，而不是一个过度显眼的欺骗系统

本文不会把 Phantom 写成“更激进的对抗模块”，而是把它重新定位为：

**一个服务于低暴露运营、异常来源隔离、有限追踪和资源克制的辅助能力。**

---

## 二、审计范围

本次重点阅读的代码包括：

- [phantom.c](D:/codeSpace/ReMirage/mirage-gateway/bpf/phantom.c)
- [honeypot.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/honeypot.go)
- [manager.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/manager.go)
- [dispatcher.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/dispatcher.go)
- [labyrinth.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/labyrinth.go)
- [fingerprinter.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/fingerprinter.go)
- [governance.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/governance.go)

---

## 三、先说结论

当前 Phantom 已经具备一套完整的基础骨架：

- eBPF 数据面的可疑源重定向
- 用户态的蜜罐服务
- 多模板调度
- 迷宫式深链响应
- 金丝雀与指纹采集
- 运维克制策略

这说明 Mirage 在“发现异常来源后，不只是记录，而是进入诱导与隔离阶段”这件事上，已经有了成体系的实现雏形。

但从当前代码看，Phantom 最大的问题不是“能力不够多”，而是：

**太静态、太统一、太容易留下系统自身的模式。**

更直白一点说：

- 数据面重定向规则过于简单
- 调度规则过于模板化
- 返回内容过于像“蜜罐产品”，不像普通业务系统
- 追踪手法带有明显自我标记
- 运营治理的目标已经写对了，但没有把前面几层真正约束住

因此，Phantom 的下一步重点不应该是“加更多模板和更多花样”，而应该是：

**把它从一个显眼的欺骗系统，收敛成一个低可见度、低误伤、可控退出的影子隔离层。**

---

## 四、收敛原则

在进入五层审计前，先明确 Phantom 后续设计的 5 条原则。

### 4.1 低误判优先于高命中

宁可少进一点 Phantom，也不要把正常访问长期送进错误的欺骗链路。

### 4.2 一致性优先于复杂度

一个稳定、普通、前后一致的假目标，比很多花样更多、但互相不一致的模板更不容易暴露。

### 4.3 有限诱导优先于无限迷宫

Phantom 的目标不是陪异常来源玩到底，而是在有限成本内完成：

- 隔离
- 观察
- 归因
- 退出

### 4.4 默认可回退

每一个被拉进 Phantom 的对象，都应该有：

- 重新评估机会
- 时间衰减
- 手动解除路径
- 自动退出机制

### 4.5 Phantom 必须服从 Mirage 的低暴露路线

Phantom 再强，也不能反过来把 Mirage 做成一个更显眼的系统。

---

## 五、数据面审计与建议

这一层重点看 `mirage-gateway/bpf/phantom.c`。

### 5.1 当前实现特点

当前数据面的核心逻辑非常直接：

- 用 `phishing_list_map` 判断来源 IP 是否已进入名单
- 命中后直接将目标地址改写为单个 `honeypot_ip`
- 通过 ring buffer 上报事件
- 维护若干计数器

它的优点是简单、快、容易接通。

它的缺点也很明显：

- 命中条件过粗
- 目标蜜罐过于单一
- 缺少衰减与二次确认
- 链路层面的完整性处理不够严谨

### 5.2 当前主要问题

#### 问题 1：全局单蜜罐 IP 过于统一

在 [phantom.c](D:/codeSpace/ReMirage/mirage-gateway/bpf/phantom.c#L22) 到 [phantom.c](D:/codeSpace/ReMirage/mirage-gateway/bpf/phantom.c#L28) 中，当前只有一个全局 `honeypot_ip`。

这意味着：

- 所有命中 Phantom 的流量最终都会收敛到同一个目标
- 不同风险等级、不同蜂窝、不同场景没有隔离
- 运营上容易形成明显模式

#### 问题 2：名单模型过于粗糙

在 [phantom.c](D:/codeSpace/ReMirage/mirage-gateway/bpf/phantom.c#L95) 到 [phantom.c](D:/codeSpace/ReMirage/mirage-gateway/bpf/phantom.c#L103) 中，是否重定向只取决于 `src_ip` 是否存在于 map 中。

目前缺少：

- TTL
- 命中次数
- 风险等级
- 最近活跃时间
- 自动过期机制

这会带来两个问题：

- 误判对象会被长期困在 Phantom
- 名单越来越脏，最终降低整套策略的可信度

#### 问题 3：统计桶存在明显错误

在 [phantom.c](D:/codeSpace/ReMirage/mirage-gateway/bpf/phantom.c#L99) 到 [phantom.c](D:/codeSpace/ReMirage/mirage-gateway/bpf/phantom.c#L101) 中，正常放行路径先用 `key=0` 查询统计，再把 `key` 设成 `STAT_PASSED`。

这会导致：

- `STAT_PASSED` 统计不准确
- 重定向率和正常流量比例失真

对 Phantom 这种需要精细运营判断的模块来说，这是必须优先修掉的问题。

#### 问题 4：包改写后的完整性处理不够稳

在 [phantom.c](D:/codeSpace/ReMirage/mirage-gateway/bpf/phantom.c#L116) 到 [phantom.c](D:/codeSpace/ReMirage/mirage-gateway/bpf/phantom.c#L120) 中，只更新了 IP checksum。

如果改写目标地址后没有把后续相关校验同步处理严谨，容易在不同路径、不同协议栈下出现异常行为。

对于 Mirage 来说，这种“不稳定”本身就是一种额外暴露。

### 5.3 建议整改方向

#### 建议 1：把单一蜜罐目标改成分层陷阱池

建议至少按以下任一维度分层：

- `cell`
- 风险等级
- 场景类别

目标不是“做更多蜜罐”，而是避免所有流量被统一引导到同一个洞口。

#### 建议 2：把名单从“是否存在”升级为“轻量状态对象”

建议名单值不再只存首次时间戳，而至少包含：

- `first_seen`
- `last_seen`
- `hit_count`
- `risk_level`
- `ttl`

这样数据面才能配合控制面实现：

- 自动衰减
- 二次确认
- 误判释放

#### 建议 3：让重定向变成“分级动作”而不是唯一动作

建议把数据面动作拆成：

- 放行
- 观察
- 限制
- 引导
- 终止

不是所有可疑来源都立即重定向进蜜罐。这样既更稳，也更符合 Mirage 的低暴露路线。

#### 建议 4：优先修复统计准确性

Phantom 是一个运营敏感模块，所有误差都会放大决策偏差。

建议立即修复：

- `STAT_PASSED` 计数错误
- `STAT_TRAPPED` 实际未形成闭环统计的问题
- 用户态与数据面统计口径不一致的问题

---

## 六、调度层审计与建议

这一层重点看 [dispatcher.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/dispatcher.go)。

### 6.1 当前实现特点

当前调度层通过：

- UA
- 路径
- 敏感文件名
- 简单浏览器特征

把请求分发到：

- 企业官网
- 网络错误
- 老旧管理后台
- 标准 404

这个思路没有问题，但当前实现偏 demo 化。

### 6.2 当前主要问题

#### 问题 1：规则太静态

在 [dispatcher.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/dispatcher.go#L83) 到 [dispatcher.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/dispatcher.go#L183) 中，规则集完全写死。

这带来的问题是：

- 命中方式容易固定化
- 返回风格容易长期一致
- 不同节点之间的行为容易高度雷同

对于 Mirage 来说，太统一就意味着太容易留下系统指纹。

#### 问题 2：信号质量不够高

当前用到的多数信号都比较粗：

- 空 UA
- 短 UA
- 路径前缀
- 字符串包含

这些信号能工作，但不够稳。

如果直接基于这些信号切模板，误判成本不低。

#### 问题 3：Header 顺序判断不可靠

在 [dispatcher.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/dispatcher.go#L237) 到 [dispatcher.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/dispatcher.go#L240) 中，Header 顺序来自 Go 的 `map` 迭代。

这并不能真实反映客户端原始 Header 顺序。

因此，任何基于这个字段做的异常判断，都应视为不可信或低可信信号。

### 6.3 建议整改方向

#### 建议 1：把规则改成“多信号评分”而不是“单规则命中”

建议把调度逻辑调整为：

- 不是某条规则命中就立刻分模板
- 而是多个低风险信号累计成一个风险分
- 再由风险分决定进入哪一类响应

这样能明显降低误判和模板抖动。

#### 建议 2：把模板选择和“业务画像”绑定

建议不是让所有节点都返回同一批企业站、404、老后台，而是让每个 Gateway 或每个 Cell 拥有一个稳定的“对外画像”。

这样同一个节点上的 Phantom 返回会更一致、更像一个普通系统，而不是一个切来切去的陷阱框架。

#### 建议 3：降低规则的“显眼度”

当前规则名称和命中对象都偏安全圈常见词汇，例如：

- `scanner_detection`
- `admin_path_probe`
- `sensitive_file_probe`

从实现角度没问题，但产品思路上应当逐渐从“安全产品思维”切向“普通系统思维”。

Mirage 更需要的是“不显眼的普通性”，而不是“明确知道自己在对抗什么”的模板感。

---

## 七、模板层审计与建议

这一层重点看：

- [honeypot.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/honeypot.go)
- [labyrinth.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/labyrinth.go)
- [dispatcher.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/dispatcher.go)

### 7.1 当前实现特点

当前模板层已经有：

- 企业官网样式页
- 标准错误页
- 假 API
- 假配置
- 假日志
- 迷宫式 JSON 深链

从“能响应”角度看是够的。

但从“像不像一个真实且普通的系统”角度看，当前还不够克制。

### 7.2 当前主要问题

#### 问题 1：模板之间缺少统一业务世界观

当前的企业官网、错误页、后台 API、迷宫 JSON，并没有形成一致的系统画像。

结果是：

- 某一类模板像企业官网
- 某一类模板像通用 API 沙盘
- 某一类模板像刻意设置的深链迷宫

这类不一致本身就是一种暴露。

#### 问题 2：迷宫太“像迷宫”

在 [labyrinth.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/labyrinth.go#L95) 到 [labyrinth.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/labyrinth.go#L129) 中，深度越大延迟越高，响应始终带 `_links` 和 `_meta`，并不断引出更深路径。

这适合研究和演示，但不够像真实世界里一个普通无聊的系统。

更适合 Mirage 的做法，不是无限往深处带，而是：

- 有限深度
- 有限页面类型
- 一致的站点画像
- 最后自然死路

#### 问题 3：错误页和默认页过于固定

在 [dispatcher.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/dispatcher.go#L272) 到 [dispatcher.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/dispatcher.go#L320) 以及 [honeypot.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/honeypot.go#L247) 到 [honeypot.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/honeypot.go#L253) 中，返回内容非常固定。

固定内容的成本低，但长期来看更容易形成可识别模式。

### 7.3 建议整改方向

#### 建议 1：用“少量稳定画像”取代“大量花样模板”

建议 Phantom 只保留少数几类稳定画像，例如：

- 普通企业官网
- 正常 API 服务
- 常规错误站点

不要把系统做得像一个“专门用于诱导的模板集合”。

#### 建议 2：每个画像内部保持高度一致

如果某个 Gateway 对外看起来像一个普通企业系统，那么：

- 首页
- 错误页
- API 返回
- 下载文件

都应该属于同一个“世界观”，而不是互相割裂。

#### 建议 3：迷宫改成“有限深度的自然死路”

不建议继续强化无限迷宫。

更好的方向是：

- 深度有限
- 延迟有限
- 页面类型有限
- 最终回到普通 404 / 403 / 超时 / 空结果

这样更省资源，也更不显眼。

---

## 八、追踪层审计与建议

这一层重点看：

- [honeypot.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/honeypot.go)
- [fingerprinter.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/fingerprinter.go)

### 8.1 当前实现特点

当前追踪层包含：

- 金丝雀文件
- 透明像素回调
- 访问日志
- 浏览器指纹采集
- Beacon 持续追踪

这说明 Phantom 不只是引导，还希望建立后续画像。

### 8.2 当前主要问题

#### 问题 1：追踪标记过于直白

在 [honeypot.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/honeypot.go#L208) 到 [honeypot.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/honeypot.go#L213) 中，`_tracking` 字段直接出现在 JSON 里。

这类显式标记过于像“追踪器本身”，不够克制。

#### 问题 2：回调路径过于显眼

在 [honeypot.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/honeypot.go#L220) 到 [honeypot.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/honeypot.go#L244) 中，`/canary/` 回调路径语义太明确。

这类路径在真实系统里不够自然。

#### 问题 3：指纹采集边界过宽

在 [fingerprinter.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/fingerprinter.go) 中，采集内容已经扩展到 Canvas、WebGL、Audio、Fonts 等高维指纹。

从“功能完整性”看没有问题，但对于 Mirage 当前路线来说，这里要问的是：

**这些采集是否真的值得增加系统暴露面和复杂度？**

对影子运营商而言，很多时候“少而准”的观察比“大而全”的指纹库更合适。

### 8.3 建议整改方向

#### 建议 1：追踪手段尽量去显式化

如果继续保留金丝雀和回调能力，建议至少做到：

- 避免使用明显的 `_tracking` 字段名
- 避免使用过于显眼的专用回调路径
- 把追踪行为嵌入更自然的资源访问链中

#### 建议 2：压缩指纹采集范围

建议只保留对运营判断真正有用的最低集合：

- UA
- 语言
- 平台
- 若干基础环境特征

没有明确收益的高维浏览器指纹，不建议在第一阶段继续扩大。

#### 建议 3：追踪目标改成“有限归因”

Phantom 不应该演进成一个无限扩张的情报系统。

更适合 Mirage 的目标是：

- 知道这个来源是否值得继续隔离
- 知道是否需要人工关注
- 知道是否需要加入短期名单

够用就好，不必无限扩张。

---

## 九、运营治理层审计与建议

这一层重点看：

- [manager.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/manager.go)
- [governance.go](D:/codeSpace/ReMirage/mirage-gateway/pkg/phantom/governance.go)

### 9.1 当前实现特点

当前治理层的方向其实是对的：

- 有资源熔断
- 有会话时长控制
- 有保留周期
- 有 Cthulhu 模式节流
- 有重生次数控制

说明你已经意识到 Phantom 不能无限打下去。

### 9.2 当前主要问题

#### 问题 1：治理原则还没有真正约束前面几层

治理层已经定义了：

- `MaxSessionDuration`
- `MaxTotalDuration`
- `CanaryTraceDepth`
- `CircuitBreaker`

但这些治理约束还没有完全体现到：

- 数据面的重定向期限
- 调度层的模板切换策略
- 模板层的深链深度
- 追踪层的采集范围

结果就是：

治理思路是克制的，但执行面还比较放飞。

#### 问题 2：缺少“何时结束”的统一策略

当前 Phantom 更像一个进入后持续运行的系统，而不是一个“进入 -> 观察 -> 决策 -> 退出”的流程。

这不利于 Mirage 的低暴露运营。

### 9.3 建议整改方向

#### 建议 1：给 Phantom 明确生命周期

建议统一定义 Phantom 生命周期：

1. `observe`
2. `confirm`
3. `isolate`
4. `limited_trace`
5. `expire`

任何来源进入 Phantom 后，都不应该无限停留。

#### 建议 2：让治理层真正下沉到前四层

建议把治理策略作为上位约束，明确作用到：

- 数据面 TTL
- 模板最大深度
- 追踪最大时长
- 资源最大消耗
- 自动回收条件

#### 建议 3：增加“静默结束”能力

Mirage 当前更需要的不是“把每个可疑对象都玩透”，而是：

- 隔离一段时间
- 建立最小画像
- 达到目的后静默结束

这更符合影子运营商路线。

---

## 十、建议实施顺序

为了尽量少改代码、尽量快获得收益，建议按下面顺序收敛。

### 第一阶段：先把明显错误和高暴露点修掉

- 修复数据面统计桶错误
- 给名单增加 TTL 和过期机制
- 把单一 `honeypot_ip` 升级为分层目标池
- 去掉显眼的 `_tracking` 和明显回调路径
- 停止依赖不可信的 Header 顺序判断

### 第二阶段：把 Phantom 从“模板集合”收敛成“稳定画像”

- 减少模板种类
- 每个 Gateway / Cell 绑定稳定外部画像
- 统一官网、错误页、API、默认页的世界观
- 把无限迷宫改成有限深度自然死路

### 第三阶段：把治理层变成真正的上位约束

- 为 Phantom 定义生命周期
- 给重定向、追踪、迷宫、指纹采集加统一退出条件
- 让 Circuit Breaker 和资源上限真正影响前面四层行为

---

## 十一、最终建议

如果 Mirage 继续走“影子运营商”路线，那么 Phantom 不应该继续往“更大、更复杂、更显眼的诱导系统”发展。

更适合 Mirage 的 Phantom，应该具备以下特征：

- 不是所有可疑对象都立即重定向
- 不是所有命中都进入统一蜜罐
- 不是所有模板都无限延展
- 不是所有追踪都尽可能多采集
- 不是所有会话都长期保留

它应该更像：

**一个低可见度、分级触发、有限追踪、可自动退出的异常来源隔离层。**

这才更符合 Mirage 当前的总路线：

**不做显眼的大系统，而做一个低暴露、强控制、能长期活下去的小规模高价值服务。**
