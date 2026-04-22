# 需求文档：Release Readiness P1 OS Release Closure

## 简介

本文档处理不直接阻断编译、但会让发布资产、证书流程、查询面和文档发布口径失真的问题。目标是让“可上线”不只体现在代码包能编译，也体现在交付模型、鉴权边界和文档真相上。

---

## 需求

### 需求 1：证书签发与轮转必须存在唯一真实路径

**用户故事：** 作为运维方，我需要 Gateway 和 OS 的证书签发/轮转流程真实可执行，以便 mTLS 不会在首轮轮换时失效。

#### 验收标准

1. WHEN Gateway 调用 `/internal/cert/sign` 时，THE OS SHALL 返回真实签发的证书，而不是占位 PEM
2. THE 轮转脚本 SHALL 选择并实现唯一的签发路径：OS API 或本地 CA，不得口径并存
3. THE 生成脚本、轮转脚本、compose/manifest 中的证书目录 SHALL 保持一致
4. THE 证书流程文档 SHALL 反映真实实现，而不是理想方案

---

### 需求 2：V2 查询面必须带真实认证并修复残缺路由

**用户故事：** 作为控制面调用方，我需要 query surface 既可用又可信，以便客户端和运维查询不会靠伪造 header 获取数据。

#### 验收标准

1. THE `/api/v2/entitlement` SHALL 校验真实身份，而不是仅信任 `X-Client-ID`
2. THE topology/query surface SHALL 定义公网暴露范围与内部暴露范围
3. THE `persona-query` SHALL 正式挂载 `/api/v2/sessions/{id}/persona`，并通过现有测试
4. THE 相关 query 测试 SHALL 保持绿色

---

### 需求 3：发布资产与运行时配置模型必须一致

**用户故事：** 作为交付方，我需要 compose、manifest、配置文件和实际程序解析逻辑一致，以便部署清单不是伪资产。

#### 验收标准

1. THE `deploy/docker-compose.os.yml` build context SHALL 指向真实存在的目录
2. THE Gateway production manifest 中的配置模型 SHALL 与 `cmd/gateway/main.go` 的 `GatewayConfig` 解析结构一致
3. THE release 资产 SHALL 提供启动所需的关键配置项，如 `security.command_secret`、`mcc.tls`
4. THE 部署清单 SHALL 可以被实际构建和启动验证，而不是只作为示意文档

---

### 需求 4：文档、Spec 与发布状态必须收敛到单一真相

**用户故事：** 作为项目负责人，我需要 docs 与 `.kiro/specs` 对“已完成/未完成/阻断项”给出一致结论，以便发布 gate 可被审计和复盘。

#### 验收标准

1. THE 仓库 SHALL 提供一份 release-readiness traceability index，映射 findings -> spec -> task -> 验收命令
2. THE 新建的 P0/P1 spec SHALL 与旧 spec 明确区分，不得复用已失真的完成状态
3. THE 旧文档若与当前实现冲突，THEN THE 文档 SHALL 被标记为过期、废弃或待整改
4. THE 发布结论 SHALL 以可验证检查点为准，而不是以勾选状态为准

