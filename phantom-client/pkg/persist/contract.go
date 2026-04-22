// Package persist — 配置契约文档
//
// # PersistConfig 运行时配置契约
//
// 字段来源分类：
//
//	来自 token（provisioning 时从 BootstrapConfig 提取）：
//	  - BootstrapPool:   []GatewayEndpoint — 初始 Gateway 节点列表
//	  - CertFingerprint: string            — 服务端证书 SHA-256 指纹（hex）
//	  - UserID:          string            — 用户唯一标识
//
//	来自 provisioning 推导：
//	  - OSEndpoint:      string            — Mirage OS 控制面 API 地址
//	    推导规则：
//	    1. 如果 token 包含 os_endpoint 字段，直接使用
//	    2. 否则从 bootstrap pool 第一个 gateway 地址推导：https://<ip>:<port>
//	    3. 如果 delivery response 包含 os_endpoint，优先使用
//
//	运行时缓存（daemon 运行期间更新）：
//	  - LastEntitlement: *LastEntitlement  — 最近一次权限查询结果缓存
//	    用于离线宽限窗口（grace window）期间的降级运行
//
// 必填字段校验：
//
//	daemon 模式启动时必须满足：
//	  - BootstrapPool 非空
//	  - UserID 非空
//	  - OSEndpoint 非空（缺失时输出明确错误并建议重新 provision）
//
// 敏感材料不在此文件中：
//
//	PSK 和 AuthKey 存储在系统 Keyring 中，通过 persist.Keyring 接口访问。
package persist
