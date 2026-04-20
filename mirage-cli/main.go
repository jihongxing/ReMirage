// Package main - Mirage CLI 用户端管理工具
// 提供 Gateway 状态查询、隧道管理、认证签名、诊断等功能
package main

import (
	"fmt"
	"os"

	"mirage-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}
