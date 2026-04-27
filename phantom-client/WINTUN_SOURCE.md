# Wintun DLL 来源追溯

## 基本信息

| 属性 | 值 |
|------|-----|
| 名称 | Wintun |
| 版本 | 0.14.1 |
| 官方网站 | https://www.wintun.net/ |
| 下载地址 | https://www.wintun.net/builds/wintun-0.14.1.zip |
| 许可证 | Prebuilt binaries license (see https://www.wintun.net/) |
| 架构 | amd64 |

## SHA256 校验

| 文件 | SHA256 |
|------|--------|
| `cmd/phantom/wintun.dll` (authoritative) | `E5DA8447DC2C320EDC0FC52FA01885C103DE8C118481F683643CACC3220DAFCE` |

## Authoritative 副本

项目中 `wintun.dll` 的唯一 authoritative 副本位于：

```
phantom-client/cmd/phantom/wintun.dll
```

该文件通过 `//go:embed wintun.dll` 指令嵌入到 `phantom-client` 二进制中（见 `cmd/phantom/main.go`）。

## 用途

Wintun 是 WireGuard 项目提供的 Windows TUN 驱动适配层，用于在 Windows 上创建虚拟网络接口。
Phantom Client 在 Windows 平台运行时通过 `tun.SetWintunDLL()` 加载嵌入的 DLL。

## Linux 构建说明

Linux 构建不需要真实的 `wintun.dll`。`Dockerfile.chaos` 中使用 `RUN touch cmd/phantom/wintun.dll`
创建空占位文件以满足 `go:embed` 编译要求。

## 更新流程

1. 从 https://www.wintun.net/builds/ 下载新版本 zip
2. 解压 `bin/amd64/wintun.dll`
3. 替换 `cmd/phantom/wintun.dll`
4. 更新本文件中的版本号和 SHA256
5. 运行 `Get-FileHash -Path cmd/phantom/wintun.dll -Algorithm SHA256` 验证
