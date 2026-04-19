# Mirage SDK

多语言 SDK 工具包，用于接入 Mirage 系统。

Multi-language SDK toolkit for integrating with the Mirage system.

## 支持语言 / Supported Languages

| 语言 / Language | 目录 / Directory | 状态 / Status |
|-----------------|------------------|---------------|
| Python | `python/` | ✅ |
| Go | `go/` | ✅ |
| JavaScript/TypeScript | `js/` | ✅ |
| Java | `java/` | ✅ |
| Rust | `rust/` | ✅ |
| C# | `csharp/` | ✅ |
| PHP | `php/` | ✅ |
| Swift | `swift/` | ✅ |
| Kotlin | `kotlin/` | ✅ |

## 文档 / Documentation

| 语言 | Language | 目录 |
|------|----------|------|
| 中文 | Chinese | [docs-zh/](./docs-zh/) |
| English | English | [docs-en/](./docs-en/) |
| Español | Spanish | [docs-es/](./docs-es/) |
| हिन्दी | Hindi | [docs-hi/](./docs-hi/) |
| Русский | Russian | [docs-ru/](./docs-ru/) |
| 日本語 | Japanese | [docs-ja/](./docs-ja/) |

## 快速开始 / Quick Start

### 1. 获取凭证 / Get Credentials

```bash
# 生成密钥对 / Generate key pair
openssl ecparam -genkey -name secp256k1 -out private.pem
openssl ec -in private.pem -pubout -out public.pem
```

### 2. 选择语言 / Choose Language

查看对应语言目录下的 `README.md`。

See `README.md` in the corresponding language directory.

## API 概览 / API Overview

| 服务 / Service | 端口 / Port | 协议 / Protocol | 用途 / Purpose |
|----------------|-------------|-----------------|----------------|
| Gateway | 50847 | gRPC | 心跳/流量/威胁上报 Heartbeat/Traffic/Threat |
| Cell | 50847 | gRPC | 蜂窝管理 Cell Management |
| Billing | 50847 | gRPC | 计费/充值 Billing/Deposit |
| WebSocket | 18443 | WSS | 实时推送 Real-time Push |

## 认证方式 / Authentication

所有请求需携带 JWT Token / All requests require JWT Token:

```
Authorization: Bearer <token>
```

## 错误码 / Error Codes

| 代码 / Code | 含义 / Meaning |
|-------------|----------------|
| 0 | 成功 / Success |
| 1 | 认证失败 / Authentication Failed |
| 2 | 配额不足 / Quota Exceeded |
| 3 | 蜂窝不可用 / Cell Unavailable |
| 4 | 参数错误 / Invalid Argument |
| 5 | 内部错误 / Internal Error |
