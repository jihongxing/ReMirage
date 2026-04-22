# 设计文档：Release Readiness P1 OS Release Closure

## 概述

这一组问题集中在“系统声称自己可上线，但交付面和运维面并不支撑这个结论”。因此设计目标是收口三条真相：

1. 证书真相
2. 查询与部署真相
3. 文档与发布真相

---

## 模块一：证书真相

### 方案

1. `/internal/cert/sign` 若保留，就必须真正持有签发能力或调用安全签发后端
2. `cert-rotate.sh` 必须与该路径一致，不允许脚本表面支持 OS API，实际仍强依赖本地 CA key
3. 统一脚本、compose、manifest 的证书目录与文件命名

---

## 模块二：Query 真相

### 方案

1. Query surface 的调用者认证应与 client runtime 的鉴权材料一致
2. entitlement 这类用户级接口不能仅依赖伪造 header
3. 修复 `persona-query` 的路由挂载缺失，保证测试即真实反映接口可用性

---

## 模块三：部署真相

### 方案

1. 对 `docker-compose.os.yml` 做一次“从 deploy 目录出发”的真实路径校验
2. 对 Gateway manifest 中的 `gateway.yaml` 与 `GatewayConfig` 字段做逐项对齐
3. 补齐真实启动必需配置，否则交付资产不能宣称 ready

---

## 模块四：发布真相

### 方案

1. 新建 release readiness index 文档，明确本轮 P0/P1 spec 是唯一有效的修复拆解
2. 不修改旧 spec 的完成勾选，而是在索引文档中声明其“不可作为本轮上线依据”
3. 所有发布结论绑定到可执行验证命令

