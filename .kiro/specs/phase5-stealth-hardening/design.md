# 设计文档：Phase 5 流量隐匿加固

## 概述

本设计围绕四个已识别的隐匿缺陷展开，按"采集基线 → 修复缺陷 → 验证效果"的顺序组织。

核心设计原则：
- 拟态而非伪装：目标不是"变成 Chrome"，而是"让分类器无法区分"
- 画像族而非单一常量：按 OS + Browser 族群生成一致指纹，容纳版本间自然差异
- 数据驱动而非参数猜测：所有校准参数从真实采集数据推导

## 一、架构变更

### 1.1 B-DNA per-connection 画像选择

当前架构：
```
active_profile_map[0] → profile_id → fingerprint_map[profile_id] → stack_fingerprint
                         (全局唯一)
```

目标架构：
```
conn_key(saddr,daddr,sport,dport) → conn_profile_map[conn_key] → profile_id
                                                                      ↓
                                              fingerprint_map[profile_id] → stack_fingerprint
```

C 侧变更（`bdna.c`）：
- `bdna_tcp_rewrite` 中，先用 `conn_key` 查 `conn_profile_map`
- 命中 → 用返回的 `profile_id` 查 `fingerprint_map`
- **未命中（SYN 首包）→ C 侧自选**：用 `bpf_get_prng_u32()` 按 `profile_select_map` 权重选择 profile_id，写入 `conn_profile_map`，再查 `fingerprint_map`。这确保第一个可观测指纹（TCP SYN）就已经是动态画像，不会回退到全局唯一画像
- 仅当 `conn_profile_map` 和 `profile_select_map` 都查不到时，才回退到 `active_profile_map[0]`（兼容降级）
- `bdna_tls_rewrite` 和 `bdna_quic_rewrite` 使用相同的 per-connection 查询路径，确保三条重写路径画像一致
- 新增 `conn_profile_map`：`BPF_MAP_TYPE_LRU_HASH`，`max_entries=65536`
- 新增 `profile_select_map`：`BPF_MAP_TYPE_ARRAY`，`max_entries=64`，value=`struct { __u32 cumulative_weight; __u32 profile_id; }`。Go 侧只将已启用且已采集基线的画像写入此 Map，禁用/待采集的画像不写入即可排除。画像 ID 不要求连续
- 新增 `profile_count_map`：`BPF_MAP_TYPE_ARRAY`，`max_entries=1`，value=`__u32 count`（`profile_select_map` 中有效条目数）

Go 侧变更（`bdna_profile_updater.go`）：
- 启动时将已启用画像族的权重写入 `profile_select_map`（CDF 格式，每条包含 cumulative_weight + 真实 profile_id）和 `profile_count_map`
- 新增 `OverrideConnectionProfile(connKey ConnKey, profileID uint32) error` 方法（策略调整用，非首包路径）
- 权重从 `gateway.yaml` 的 `bdna.profile_weights` 读取
- registry 中禁用或 OS 节点不可用的画像不写入 `profile_select_map`

### 1.2 NPM MIMIC 分布模式

当前 `calculate_padding` 逻辑：
```c
switch (config->padding_mode) {
    case 0: // FIXED_MTU
    case 1: // RANDOM_RANGE
    case 2: // GAUSSIAN
}
```

新增 mode=3 `MIMIC`：
```c
case 3: // MIMIC — 从目标分布 CDF 采样
    target_len = sample_from_cdf(npm_target_distribution_map);
    if (target_len > current_size)
        padding = target_len - current_size;
    else
        padding = 0; // 目标比当前小，不截断
```

`npm_target_distribution_map`：
- 类型：`BPF_MAP_TYPE_ARRAY`
- key：`__u32`（bin index，0-255）
- value：`struct { __u32 cumulative_prob; __u16 pkt_len_low; __u16 pkt_len_high; }`
- 256 个 bin 覆盖 0-1500 字节范围
- Go 控制面从真实对照基线的包长直方图生成 CDF 写入

采样算法（C 侧）：
```c
static __always_inline __u16 sample_from_cdf(void *map) {
    __u32 rand = bpf_get_prng_u32() % 10000; // 0-9999
    // 二分查找 CDF 中 cumulative_prob >= rand 的最小 bin
    // 返回该 bin 的 [pkt_len_low, pkt_len_high] 区间内随机值
}
```

### 1.3 Jitter IAT 校准链路

当前链路：
```
gateway.yaml → jitter_config_map → jitter_lite_egress → gaussian_sample(mean, stddev)
```

校准后链路：
```
真实对照基线 IAT 统计 → Go 控制面 → dna_template_map → jitter_lite_egress → get_mimic_delay()
                                                                                    ↓
                                                              (有模板时用模板参数，无模板时回退到 jitter_config)
```

Go 侧变更（`dna_updater.go`）：
- 新增 `CalibrateFromBaseline(baselinePath string) error` 方法
- 从 `artifacts/dpi-audit/timing/iat-stats.csv` 读取真实 IAT 统计
- 将 `iat_mean_us` / `iat_std_us` 写入 `dna_template_map` 的 `TargetIATMu` / `TargetIATSigma`

### 1.4 TLS/QUIC 指纹审计矩阵

三条路径分别审计：

| 路径 | 握手协议 | 指纹检测面 | 代码位置 | 审计项 |
|------|----------|-----------|----------|--------|
| H3/QUIC 主路径 | QUIC Initial | transport params、CID 长度、token | `quic_engine.go` | max_idle/max_data/max_streams 与画像族一致 |
| H2/WSS 路径 | TLS ClientHello | extension 列表、cipher suites、ALPN | `chameleon_client.go` `dialWithUTLS` | uTLS spec 与画像族 TLS extension 一致 |
| WS upgrade 路径 | TLS ClientHello + HTTP/1.1 | extension 列表 + Upgrade headers | `chameleon_client.go` `DialChameleon` | User-Agent/Accept/Sec-WebSocket-* 与浏览器一致 |

## 二、Property Tests

### Property 1: per-connection 画像隔离
- 生成 N 条随机 conn_key，分配画像后验证：不同 conn_key 可以有不同 profile_id；同一 conn_key 多次查询返回相同 profile_id
- 验证: 需求 2.1, 2.4

### Property 2: NPM MIMIC 分布拟合
- 生成随机目标 CDF（单调递增），随机 **padding-eligible** 包序列（current_size ∈ [min_packet_size, target_mtu]）经 MIMIC 处理后，输出包长分布与目标 CDF 的 JS 散度 < 阈值。同时验证：current_size < min_packet_size 的包不填充、current_size > target_mtu 的包 padding=0（单调不截断约束）。全局 JS 散度（含不可填充包）留给 M15 真实实验验证，不作为 PBT 断言
- 验证: 需求 3.2, 3.5

### Property 3: Jitter 校准后 IAT 分布
- 生成随机 baseline IAT 参数（mean ∈ [500,5000]μs, std ∈ [50,1000]μs），校准后 Jitter 输出的 IAT 序列均值偏差 < 20%、标准差偏差 < 30%。P95 偏差和 KS 检验 p-value 作为 M15 实验观测指标记录，不作为 PBT 断言（当前 `dna_template_map` 只有 mean/std 两个字段，无法精确控制 P95）
- 验证: 需求 4.1, 4.3

### Property 4: 画像族权重分布
- 生成随机权重配置，分配 N 条连接后，各画像族的实际占比与配置权重的 χ² 检验 p-value > 0.01
- 验证: 需求 2.3

## 三、里程碑

| 里程碑 | 内容 | 出关条件 |
|--------|------|----------|
| M13 | 真实对照基线采集 | **M13-full**：至少 3 个画像族各 100 条连接的 pcapng + 统计数据（在对应原生 OS 采集）。**M13-degraded**：部分画像族标注"待采集"，已采集画像族数据有效。M13-full 才能进入 Capability-Upgrade Gate，M13-degraded 只能进入 Implementation Exit |
| M14 | B-DNA 动态化 + NPM MIMIC + Jitter 校准 | 代码实现 + PBT 通过 + 编译回归通过 |
| M15 | TLS/QUIC 指纹审计 + 分类器实验迭代 | 三条路径审计报告 + 新 AUC 数据 |
| M16 | 治理回写 + 能力域状态评估 | capability-truth-source 回写 + claims-boundary 更新 |

## 四、不做的事

- 不实现完整浏览器栈克隆（TLS/HTTP2/HTTP3 行为级模拟）
- 不追求 AUC=0.5（理论不可区分），只追求简单分类器无法轻松区分
- 不在本 Spec 中处理双向流量整形（Client 侧填充），留作后续任务
- 不修改 Jitter 的 `jitter_lite_adaptive` / `jitter_lite_physical` / `jitter_lite_social` 三个未挂载函数
