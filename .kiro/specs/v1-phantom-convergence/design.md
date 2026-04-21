# 设计文档：Phantom 蜜罐收敛（第一阶段）

## 概述

本设计覆盖 Phantom 数据面修复、名单升级、目标池分层、追踪去显式化、模板收敛、迷宫限深。改动集中在 `bpf/phantom.c`（C 数据面）和 `pkg/phantom/`（Go 控制面）。

## 设计原则

1. **C 做数据面，Go 做控制面**：名单查询/重定向在 eBPF TC 层，TTL 清理/目标池管理在 Go 层
2. **Go → C 通过 eBPF Map**：名单写入/目标池配置通过 Map 操作，不使用直接函数调用
3. **低暴露优先**：所有改动以降低系统指纹为首要目标
4. **向后兼容**：名单 Map value 结构变更需要重新加载 eBPF 程序，但 Go 侧 API 保持兼容

---

## 模块 1：数据面统计修复（需求 1）

### 改动范围

- `mirage-gateway/bpf/phantom.c`：修复 STAT_PASSED 计数逻辑

### 设计细节

当前错误代码：

```c
// ❌ 错误：先用 key=0 查询，再改 key 为 STAT_PASSED
__u32 key = 0;
__u64 *passed = bpf_map_lookup_elem(&phantom_stats, &key);
if (passed) __sync_fetch_and_add(passed, 1);
key = STAT_PASSED;  // 这行改了 key 但没有再次查询
return TC_ACT_OK;
```

修复为：

```c
// ✅ 正确：直接用 STAT_PASSED 作为 key
__u32 pass_key = STAT_PASSED;
__u64 *passed = bpf_map_lookup_elem(&phantom_stats, &pass_key);
if (passed) __sync_fetch_and_add(passed, 1);
return TC_ACT_OK;
```

---

## 模块 2：名单结构升级（需求 2）

### 改动范围

- `mirage-gateway/bpf/phantom.c`：升级 phishing_list_map value 结构
- `mirage-gateway/pkg/phantom/manager.go`：TTL 清理循环 + 新增 API

### 设计细节

#### eBPF 名单结构

```c
// bpf/phantom.c
struct phantom_entry {
    __u64 first_seen;
    __u64 last_seen;
    __u32 hit_count;
    __u8  risk_level;  // 0-4
    __u8  pad[3];
    __u32 ttl_seconds;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u32);   // src_ip
    __type(value, struct phantom_entry);
} phishing_list_map SEC(".maps");
```

数据面命中时更新 `last_seen` 和 `hit_count`：

```c
struct phantom_entry *entry = bpf_map_lookup_elem(&phishing_list_map, &src_ip);
if (!entry) {
    // 不在名单，放行
    ...
}
// 更新命中信息
entry->last_seen = bpf_ktime_get_ns();
__sync_fetch_and_add(&entry->hit_count, 1);
```

#### Go 侧 TTL 清理

```go
// pkg/phantom/manager.go
func (m *PhantomManager) StartTTLCleaner(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done(): return
            case <-ticker.C: m.cleanExpired()
            }
        }
    }()
}

func (m *PhantomManager) cleanExpired() {
    now := uint64(time.Now().UnixNano())
    // 遍历 phishing_list_map，删除 last_seen + ttl_seconds*1e9 < now 的条目
    var key uint32
    var entry PhantomEntry
    var toDelete []uint32
    iter := m.loader.GetMap("phishing_list_map").Iterate()
    for iter.Next(&key, &entry) {
        expireAt := entry.LastSeen + uint64(entry.TTLSeconds)*1e9
        if now > expireAt {
            toDelete = append(toDelete, key)
        }
    }
    for _, k := range toDelete {
        m.loader.GetMap("phishing_list_map").Delete(k)
    }
}

func (m *PhantomManager) AddToPhantom(ip string, riskLevel uint8, ttlSeconds uint32) error {
    ipAddr := net.ParseIP(ip).To4()
    key := binary.BigEndian.Uint32(ipAddr)
    entry := PhantomEntry{
        FirstSeen:  uint64(time.Now().UnixNano()),
        LastSeen:   uint64(time.Now().UnixNano()),
        HitCount:   0,
        RiskLevel:  riskLevel,
        TTLSeconds: ttlSeconds,
    }
    return m.loader.GetMap("phishing_list_map").Put(key, &entry)
}
```

---

## 模块 3：分层目标池（需求 3）

### 改动范围

- `mirage-gateway/bpf/phantom.c`：honeypot_config 扩展为 8 条目
- `mirage-gateway/pkg/phantom/manager.go`：目标池管理

### 设计细节

#### eBPF 目标池

```c
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 8);  // 从 1 扩展到 8，按 risk_level 索引
    __type(key, __u32);
    __type(value, __u32);    // honeypot_ip (network order)
} honeypot_config SEC(".maps");
```

重定向时按 risk_level 查找：

```c
__u32 level_key = (__u32)entry->risk_level;
__u32 *target_ip = bpf_map_lookup_elem(&honeypot_config, &level_key);
if (!target_ip || *target_ip == 0) {
    // 回退到 level=0 默认蜜罐
    __u32 default_key = 0;
    target_ip = bpf_map_lookup_elem(&honeypot_config, &default_key);
    if (!target_ip || *target_ip == 0) return TC_ACT_OK;
}
ip->daddr = *target_ip;
```

#### Go 侧配置

```go
func (m *PhantomManager) SetHoneypotPool(level int, ip string) error {
    ipAddr := net.ParseIP(ip).To4()
    key := uint32(level)
    value := binary.BigEndian.Uint32(ipAddr)
    return m.loader.GetMap("honeypot_config").Put(key, &value)
}
```

配置文件：

```yaml
# gateway.yaml
phantom:
  honeypot_pool:
    0: "10.99.1.100"  # 默认
    1: "10.99.1.101"  # 低风险
    2: "10.99.1.102"  # 中风险
    3: "10.99.1.103"  # 高风险
```

---

## 模块 4：追踪去显式化（需求 4）

### 改动范围

- `mirage-gateway/pkg/phantom/honeypot.go`：重构追踪标记和回调路径

### 设计细节

#### 追踪 ID 嵌入自然字段

```go
// 之前：
content := map[string]interface{}{
    "classification": "CONFIDENTIAL",
    "data":           h.generateRandomData(),
    "_tracking":      token.ID,
}

// 之后：
content := map[string]interface{}{
    "data":   h.generateRandomData(),
    "ref":    token.ID,  // 嵌入自然字段名
    "format": "json",
}
```

#### 回调路径自然化

```go
// 之前：
mux.HandleFunc("/canary/", h.handleCanaryCallback)

// 之后：
mux.HandleFunc("/static/img/", h.handleCanaryCallback)  // 伪装为静态资源
mux.HandleFunc("/collect", h.handleCanaryCallback)       // 伪装为 analytics
```

---

## 模块 5：调度规则清理（需求 5）

### 改动范围

- `mirage-gateway/pkg/phantom/dispatcher.go`：移除 Header 顺序依赖

### 设计细节

- 删除 `IsSuspiciousHeaderOrder` 函数或标记 deprecated
- 从 `RequestContext` 中移除 `HeaderOrder` 字段
- 调度规则仅保留可信信号：UA 匹配、路径匹配、TLS 版本、Accept-Language

---

## 模块 6：Persona 业务画像（需求 6、8）

### 改动范围

- `mirage-gateway/pkg/phantom/persona.go`（新建）：业务画像定义
- `mirage-gateway/pkg/phantom/dispatcher.go`：模板使用 persona
- `mirage-gateway/pkg/phantom/honeypot.go`：响应使用 persona

### 设计细节

```go
// pkg/phantom/persona.go
type Persona struct {
    CompanyName   string `yaml:"company_name"`
    Domain        string `yaml:"domain"`
    TagLine       string `yaml:"tag_line"`
    PrimaryColor  string `yaml:"primary_color"`
    ErrorPrefix   string `yaml:"error_prefix"`
    APIVersion    string `yaml:"api_version"`
    CopyrightYear int    `yaml:"copyright_year"`
}

var DefaultPersona = Persona{
    CompanyName:   "CloudBridge Systems",
    Domain:        "cloudbridge.io",
    TagLine:       "Enterprise Cloud Infrastructure",
    PrimaryColor:  "#2563eb",
    ErrorPrefix:   "CB",
    APIVersion:    "v2",
    CopyrightYear: 2026,
}
```

所有模板使用 persona 渲染：

```go
func (d *Dispatcher) serveCorporateWeb(w http.ResponseWriter, r *http.Request) {
    p := d.persona
    html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>%s</title>
<style>body{font-family:sans-serif;margin:0;padding:0;background:#f5f5f5}
.header{background:%s;color:white;padding:60px 20px;text-align:center}
</style></head><body>
<div class="header"><h1>%s</h1><p>%s</p></div>
<div style="max-width:1200px;margin:40px auto;padding:0 20px">
<div style="background:white;border-radius:8px;padding:30px;box-shadow:0 2px 10px rgba(0,0,0,0.1)">
<h2>Welcome</h2><p>Please authenticate to continue.</p></div></div>
<div style="text-align:center;padding:40px;color:#666">&copy; %d %s</div>
</body></html>`, p.CompanyName, p.PrimaryColor, p.CompanyName, p.TagLine, p.CopyrightYear, p.CompanyName)
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Write([]byte(html))
}
```

配置：

```yaml
# gateway.yaml
phantom:
  persona:
    company_name: "CloudBridge Systems"
    domain: "cloudbridge.io"
    tag_line: "Enterprise Cloud Infrastructure"
    primary_color: "#2563eb"
    error_prefix: "CB"
    api_version: "v2"
```

---

## 模块 7：迷宫限深（需求 7）

### 改动范围

- `mirage-gateway/pkg/phantom/labyrinth.go`：限制深度 + 移除 HATEOAS 字段

### 设计细节

```go
const MaxLabyrinthDepth = 5

func (l *LabyrinthEngine) generateResponse(w http.ResponseWriter, r *http.Request, depth int) {
    if depth > MaxLabyrinthDepth {
        // 自然死路：返回 404
        w.WriteHeader(http.StatusNotFound)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "not_found",
            "message": "The requested resource does not exist.",
        })
        return
    }

    w.Header().Set("Content-Type", "application/json")
    data := l.generateFakeData(depth)

    // 不再包含 _links 和 _meta
    response := map[string]interface{}{
        "status": "success",
        "data":   data,
        "page":   1,
        "total":  len(data.([]map[string]interface{})),
    }

    // 仅在未达最大深度时包含 next 链接（自然分页风格）
    if depth < MaxLabyrinthDepth {
        response["next"] = fmt.Sprintf("%s?page=2", r.URL.Path)
    }

    json.NewEncoder(w).Encode(response)
}
```

maxDelay 从 30s 降为 3s：

```go
func NewLabyrinthEngine() *LabyrinthEngine {
    return &LabyrinthEngine{
        // ...
        maxDelay: 3 * time.Second, // 从 30s 降为 3s
    }
}
```

---

## 配置变更

### gateway.yaml 新增

```yaml
phantom:
  enabled: true
  persona:
    company_name: "CloudBridge Systems"
    domain: "cloudbridge.io"
    tag_line: "Enterprise Cloud Infrastructure"
    primary_color: "#2563eb"
    error_prefix: "CB"
    api_version: "v2"
  honeypot_pool:
    0: "10.99.1.100"
    1: "10.99.1.101"
    2: "10.99.1.102"
    3: "10.99.1.103"
  default_ttl_seconds: 3600
  high_risk_ttl_seconds: 86400
  labyrinth_max_depth: 5
  labyrinth_max_delay_ms: 3000
```

## 不在本次范围内

- 治理层上位约束（第三阶段）
- Phantom 生命周期状态机（第三阶段）
- 指纹采集范围压缩（第三阶段，当前保持现状）
- 多信号评分调度（第三阶段，当前保持规则匹配但移除不可信信号）
