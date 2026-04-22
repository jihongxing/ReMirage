# 任务清单：Release Readiness P1 OS Release Closure

## P1-A：证书链路收口

- [x] 1. 让 `/internal/cert/sign` 返回真实签发证书
  - [x] 1.1 实现 CSR 解析和签发逻辑
  - [x] 1.2 为有效期、主题、序列号增加最小验证
  - [x] 1.3 增加签发成功与失败测试

- [x] 2. 收口证书轮转路径
  - [x] 2.1 修复 `cert-rotate.sh` 中“宣称走 OS API，实际仍依赖本地 root-ca.key”的矛盾
  - [x] 2.2 统一证书目录与文件命名
  - [x] 2.3 更新部署文档与脚本说明

## P1-B：查询面与部署资产修复

- [x] 3. 修复 query surface 认证边界
  - [x] 3.1 为 `/api/v2/entitlement` 接入真实认证
  - [x] 3.2 明确 topology/query surface 的暴露边界
  - [x] 3.3 回归验证未经授权请求被拒绝

- [x] 4. 修复 `persona-query` 路由缺口
  - [x] 4.1 挂载 `/api/v2/sessions/{id}/persona`
  - [x] 4.2 跑通 `go test ./services/persona-query -v`

- [x] 5. 修复发布资产不一致
  - [x] 5.1 修正 `deploy/docker-compose.os.yml` 的 build context
  - [x] 5.2 对齐 Gateway manifest 与 `GatewayConfig`
  - [x] 5.3 补齐运行必需配置项

## P1-C：发布文档与 traceability

- [x] 6. 新增 release readiness traceability index
  - [x] 6.1 建立 findings -> spec -> task -> verification 映射
  - [x] 6.2 标注旧 spec/旧文档的适用范围或失效状态
  - [x] 6.3 明确本轮发布仅以 P0/P1 检查点为准

## 检查点

- [x] C1. 证书签发与轮转路径一致
- [x] C2. query surface 认证真实生效
- [x] C3. `persona-query` 测试通过
- [x] C4. compose/manifest 可被真实验证
- [x] C5. release readiness index 发布完成

